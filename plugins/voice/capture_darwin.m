//go:build darwin && cgo

#import <AVFoundation/AVFoundation.h>
#import <CoreAudio/CoreAudio.h>
#import <CoreMedia/CoreMedia.h>
#import <Foundation/Foundation.h>
#include "capture_darwin.h"
#include <limits.h>
#include <stdio.h>
#include <string.h>

// Command-line AVFoundation clients still need a microphone purpose string.
static const char jt_capture_info[] __attribute__((section("__TEXT,__info_plist"), used)) =
    "<?xml version=\"1.0\" encoding=\"UTF-8\"?><plist version=\"1.0\"><dict>"
    "<key>CFBundleIdentifier</key><string>com.aono255.just-talk</string>"
    "<key>CFBundleName</key><string>Just Talk</string>"
    "<key>NSMicrophoneUsageDescription</key><string>Just Talk 使用麦克风进行语音输入。</string>"
    "</dict></plist>";

// 32 seconds of 16 kHz mono PCM. A stalled consumer fails explicitly instead
// of blocking the capture queue or silently dropping part of an utterance.
#define JT_CAPTURE_MAX_PENDING (1024 * 1024)

static int64_t jt_capture_ms(void) {
    return (int64_t)(NSProcessInfo.processInfo.systemUptime * 1000);
}

static void jt_capture_error(char *buffer, size_t size, NSString *message) {
    if (buffer && size) snprintf(buffer, size, "%s", (message ?: @"录音失败").UTF8String);
}

@interface JTCapture : NSObject <AVCaptureAudioDataOutputSampleBufferDelegate> {
    NSCondition *_condition;
    NSMutableData *_pending;
    NSUInteger _offset;
    BOOL _finished;
    BOOL _received;
    NSString *_error;
    AVCaptureSession *_session;
    AVCaptureAudioDataOutput *_output;
    dispatch_queue_t _samples;
    id _observer;
    void *_activity;
}
- (void)fail:(NSString *)reason;
- (NSString *)errorDescription;
- (void)appendPCM:(NSData *)data;
- (void)appendSample:(CMSampleBufferRef)sample;
- (BOOL)waitForFirstBuffer;
- (int)read:(void *)data size:(size_t)size error:(char *)error errorSize:(size_t)errorSize;
- (BOOL)configure:(jt_capture_start_report_t *)report;
- (BOOL)start:(jt_capture_start_report_t *)report;
- (void)stop;
@end

@implementation JTCapture
- (instancetype)init {
    if ((self = [super init])) {
        _condition = [[NSCondition alloc] init];
        _pending = [[NSMutableData alloc] init];
    }
    return self;
}
- (void)dealloc {
    [_condition release];
    [_pending release];
    [_error release];
    [_session release];
    [_output release];
    if (_samples) dispatch_release(_samples);
    [super dealloc];
}
- (void)fail:(NSString *)reason {
    [_condition lock];
    if (!_finished) {
        _error = [reason copy];
        _finished = YES;
        [_condition broadcast];
    }
    [_condition unlock];
}
- (NSString *)errorDescription {
    [_condition lock];
    NSString *reason = [[_error copy] autorelease];
    [_condition unlock];
    return reason;
}
- (void)appendPCM:(NSData *)data {
    [_condition lock];
    if (!_finished && data.length) {
        if (data.length > JT_CAPTURE_MAX_PENDING - (_pending.length - _offset)) {
            _error = [@"识别端未及时读取音频，录音缓冲已满" copy];
            _finished = YES;
        } else {
            if (_offset) {
                [_pending replaceBytesInRange:NSMakeRange(0, _offset) withBytes:NULL length:0];
                _offset = 0;
            }
            [_pending appendData:data];
            _received = YES;
        }
        [_condition broadcast];
    }
    [_condition unlock];
}
- (BOOL)waitForFirstBuffer {
    NSDate *deadline = [NSDate dateWithTimeIntervalSinceNow:3];
    [_condition lock];
    while (!_received && !_finished) {
        if (![_condition waitUntilDate:deadline]) break;
    }
    BOOL ready = _received && !_finished;
    [_condition unlock];
    return ready;
}
- (int)read:(void *)data size:(size_t)size error:(char *)error errorSize:(size_t)errorSize {
    if (!size) return 0;
    [_condition lock];
    while (_pending.length == _offset && !_finished) [_condition wait];
    NSUInteger available = _pending.length - _offset;
    int count = (int)MIN(available, MIN(size, (size_t)INT_MAX));
    if (count) {
        memcpy(data, (const char *)_pending.bytes + _offset, count);
        _offset += count;
        if (_offset == _pending.length) {
            [_pending setLength:0];
            _offset = 0;
        }
    } else if (_error) {
        jt_capture_error(error, errorSize, _error);
        count = -1;
    }
    [_condition unlock];
    return count;
}
- (void)captureOutput:(AVCaptureOutput *)output didOutputSampleBuffer:(CMSampleBufferRef)sample
       fromConnection:(AVCaptureConnection *)connection {
    (void)output; (void)connection;
    [self appendSample:sample];
}
- (void)appendSample:(CMSampleBufferRef)sample {
    @autoreleasepool {
        const AudioStreamBasicDescription *format = CMAudioFormatDescriptionGetStreamBasicDescription(
            CMSampleBufferGetFormatDescription(sample));
        if (!format || format->mFormatID != kAudioFormatLinearPCM || format->mSampleRate != 16000 ||
            format->mChannelsPerFrame != 1 || format->mBitsPerChannel != 16 || format->mBytesPerFrame != 2 ||
            !(format->mFormatFlags & kAudioFormatFlagIsSignedInteger) ||
            (format->mFormatFlags & (kAudioFormatFlagIsFloat | kAudioFormatFlagIsBigEndian))) {
            [self fail:@"麦克风输出不是 16 kHz、16 位单声道 PCM"];
            return;
        }
        CMBlockBufferRef block = CMSampleBufferGetDataBuffer(sample);
        size_t size = block ? CMBlockBufferGetDataLength(block) : 0;
        if (!size || size != (size_t)CMSampleBufferGetNumSamples(sample) * 2) {
            [self fail:@"麦克风音频缓冲长度无效"];
            return;
        }
        NSMutableData *data = [NSMutableData dataWithLength:size];
        OSStatus status = CMBlockBufferCopyDataBytes(block, 0, size, data.mutableBytes);
        if (status != noErr) {
            [self fail:[NSString stringWithFormat:@"读取麦克风音频失败: %d", (int)status]];
            return;
        }
        [self appendPCM:data];
    }
}
- (BOOL)configure:(jt_capture_start_report_t *)report {
    _activity = jt_audio_activity_begin();
    int64_t began = jt_capture_ms();
    AudioObjectPropertyAddress property = {kAudioHardwarePropertyDefaultInputDevice,
        kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
    AudioDeviceID deviceID = kAudioObjectUnknown;
    UInt32 size = sizeof(deviceID);
    OSStatus status = AudioObjectGetPropertyData(kAudioObjectSystemObject, &property, 0, NULL, &size, &deviceID);
    if (status != noErr || deviceID == kAudioObjectUnknown) {
        [self fail:[NSString stringWithFormat:@"读取默认麦克风失败: %d", (int)status]];
        return NO;
    }
    property.mSelector = kAudioDevicePropertyDeviceUID;
    CFStringRef uid = NULL;
    size = sizeof(uid);
    status = AudioObjectGetPropertyData(deviceID, &property, 0, NULL, &size, &uid);
    if (status != noErr || !uid) {
        [self fail:[NSString stringWithFormat:@"读取麦克风标识失败: %d", (int)status]];
        return NO;
    }
    AVCaptureDevice *device = [AVCaptureDevice deviceWithUniqueID:(NSString *)uid];
    CFRelease(uid);
    report->select_ms = jt_capture_ms() - began;
    if (!device || ![device hasMediaType:AVMediaTypeAudio]) {
        [self fail:@"当前默认麦克风不可用于 AVFoundation 采集"];
        return NO;
    }
    began = jt_capture_ms();
    NSError *error = nil;
    AVCaptureDeviceInput *input = [AVCaptureDeviceInput deviceInputWithDevice:device error:&error];
    if (!input) {
        [self fail:error ? [NSString stringWithFormat:@"%@ (%ld): %@", error.domain,
                           (long)error.code, error.localizedDescription] : @"创建麦克风输入失败"];
        return NO;
    }
    _session = [[AVCaptureSession alloc] init];
    _output = [[AVCaptureAudioDataOutput alloc] init];
    _output.audioSettings = @{AVFormatIDKey: @(kAudioFormatLinearPCM), AVSampleRateKey: @16000,
        AVNumberOfChannelsKey: @1, AVLinearPCMBitDepthKey: @16, AVLinearPCMIsFloatKey: @NO,
        AVLinearPCMIsBigEndianKey: @NO, AVLinearPCMIsNonInterleaved: @NO};
    _samples = dispatch_queue_create("just-talk.audio.samples", DISPATCH_QUEUE_SERIAL);
    [_output setSampleBufferDelegate:self queue:_samples];
    if (![_session canAddInput:input] || ![_session canAddOutput:_output]) {
        [self fail:@"当前麦克风无法建立采集会话"];
        return NO;
    }
    [_session beginConfiguration];
    [_session addInput:input];
    [_session addOutput:_output];
    [_session commitConfiguration];
    _observer = [[[NSNotificationCenter defaultCenter]
        addObserverForName:AVCaptureSessionRuntimeErrorNotification object:_session queue:nil
        usingBlock:^(NSNotification *notification) {
            NSError *failure = notification.userInfo[AVCaptureSessionErrorKey];
            [self fail:[NSString stringWithFormat:@"%@ (%ld): %@", failure.domain,
                        (long)failure.code, failure.localizedDescription]];
        }] retain];
    report->configure_ms = jt_capture_ms() - began;
    return YES;
}
- (BOOL)start:(jt_capture_start_report_t *)report {
    int64_t began = jt_capture_ms();
    [_session startRunning];
    report->start_ms = jt_capture_ms() - began;
    if (!_session.running) {
        [self fail:@"麦克风采集会话启动失败"];
        return NO;
    }
    began = jt_capture_ms();
    BOOL ready = [self waitForFirstBuffer];
    report->first_buffer_ms = jt_capture_ms() - began;
    if (!ready) [self fail:@"麦克风启动后 3 秒内没有收到音频"];
    return ready;
}
- (void)stop {
    [_condition lock];
    _finished = YES;
    [_condition broadcast];
    [_condition unlock];
    @try { [_session stopRunning]; }
    @catch (NSException *exception) {
        [_condition lock];
        if (!_error) _error = [[NSString stringWithFormat:@"停止麦克风失败: %@: %@",
                              exception.name, exception.reason] copy];
        [_condition unlock];
    }
    [_output setSampleBufferDelegate:nil queue:NULL];
    if (_samples) dispatch_sync(_samples, ^{});
    if (_observer) {
        [[NSNotificationCenter defaultCenter] removeObserver:_observer];
        [_observer release];
        _observer = nil;
    }
    jt_audio_activity_end(_activity);
    _activity = NULL;
}
@end

int jt_capture_start(jt_capture_t **capture, jt_capture_start_report_t *report,
                     char *error, size_t errorSize) {
    if (!capture || !report) return -1;
    *capture = NULL;
    memset(report, 0, sizeof(*report));
    @autoreleasepool {
        JTCapture *recorder = [[JTCapture alloc] init];
        BOOL ready = NO;
        @try { ready = [recorder configure:report] && [recorder start:report]; }
        @catch (NSException *exception) {
            [recorder fail:[NSString stringWithFormat:@"%@: %@", exception.name, exception.reason]];
        }
        if (!ready) {
            jt_capture_error(error, errorSize, [recorder errorDescription]);
            [recorder stop];
            [recorder release];
            return -1;
        }
        *capture = (jt_capture_t *)recorder;
        return 0;
    }
}

int jt_capture_read(jt_capture_t *capture, void *data, size_t size, char *error, size_t errorSize) {
    @autoreleasepool {
        return [(JTCapture *)capture read:data size:size error:error errorSize:errorSize];
    }
}
void jt_capture_stop(jt_capture_t *capture) {
    @autoreleasepool { [(JTCapture *)capture stop]; }
}
void jt_capture_destroy(jt_capture_t *capture) {
    @autoreleasepool { [(JTCapture *)capture release]; }
}
