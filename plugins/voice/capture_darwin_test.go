//go:build darwin && cgo

package voice

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Real native buffer and callback code, with synthetic PCM instead of a mic.
func TestDarwinCaptureBuffersAndStop(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "capture-test.m")
	binary := filepath.Join(dir, "capture-test")
	if err := os.WriteFile(source, []byte(darwinCaptureTest), 0600); err != nil {
		t.Fatal(err)
	}
	includeDir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("clang", "-I", includeDir, "-Wall", "-Wextra", "-Werror",
		"-framework", "AVFoundation", "-framework", "CoreAudio", "-framework", "CoreMedia",
		"-framework", "Foundation", source, filepath.Join(includeDir, "activity_darwin.m"), "-o", binary)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile native capture regression: %v\n%s", err, out)
	}
	if out, err := exec.Command(binary).CombinedOutput(); err != nil {
		t.Fatalf("native capture regression: %v\n%s", err, out)
	}
}

const darwinCaptureTest = `
#include "capture_darwin.m"
#include <unistd.h>
#define CHECK(x, message) do { if (!(x)) { fprintf(stderr, "%s\n", message); return 1; } } while (0)

static CMSampleBufferRef sample(UInt32 rate) {
    static unsigned char bytes[] = {1, 2, 3, 4, 5, 6, 7, 8};
    AudioStreamBasicDescription pcm = {0};
    pcm.mSampleRate = rate;
    pcm.mFormatID = kAudioFormatLinearPCM;
    pcm.mFormatFlags = kAudioFormatFlagIsSignedInteger | kAudioFormatFlagIsPacked;
    pcm.mBytesPerPacket = pcm.mBytesPerFrame = 2;
    pcm.mFramesPerPacket = pcm.mChannelsPerFrame = 1;
    pcm.mBitsPerChannel = 16;
    CMAudioFormatDescriptionRef format = NULL;
    CMBlockBufferRef block = NULL;
    CMSampleBufferRef result = NULL;
    if (CMAudioFormatDescriptionCreate(kCFAllocatorDefault, &pcm, 0, NULL, 0, NULL, NULL, &format)) return NULL;
    if (CMBlockBufferCreateWithMemoryBlock(kCFAllocatorDefault, bytes, sizeof(bytes), kCFAllocatorNull,
        NULL, 0, sizeof(bytes), 0, &block) == noErr)
        CMAudioSampleBufferCreateReadyWithPacketDescriptions(kCFAllocatorDefault, block, format,
            sizeof(bytes) / 2, kCMTimeZero, NULL, &result);
    if (block) CFRelease(block);
    CFRelease(format);
    return result;
}

int main(void) {
    @autoreleasepool {
        char error[256] = {0};
        unsigned char bytes[16] = {0};
        JTCapture *rec = [[JTCapture alloc] init];
        CMSampleBufferRef pcm = sample(16000);
        CHECK(pcm, "synthetic PCM creation failed");
        [rec appendSample:pcm];
        CHECK([rec waitForFirstBuffer], "captured PCM must make startup ready");
        CHECK([rec read:bytes size:3 error:error errorSize:sizeof(error)] == 3 && bytes[0] == 1 && bytes[2] == 3,
              "partial reads must preserve captured byte order");
        [rec stop];
        CHECK([rec read:bytes size:sizeof(bytes) error:error errorSize:sizeof(error)] == 5 && bytes[0] == 4 && bytes[4] == 8,
              "stop must retain the unread audio tail");
        [rec appendPCM:[NSData dataWithBytes:bytes length:2]];
        CHECK([rec read:bytes size:sizeof(bytes) error:error errorSize:sizeof(error)] == 0,
              "stopped capture must reject late callbacks and reach EOF");
        [rec release];
        CFRelease(pcm);

        rec = [[JTCapture alloc] init];
        pcm = sample(24000);
        [rec appendSample:pcm];
        CHECK(![rec waitForFirstBuffer] && [rec read:bytes size:sizeof(bytes) error:error errorSize:sizeof(error)] == -1,
              "unexpected sample rate must fail before being sent to ASR");
        CHECK(error[0], "capture errors must have an actionable message");
        [rec stop]; [rec release]; CFRelease(pcm);

        rec = [[JTCapture alloc] init];
        dispatch_group_t readers = dispatch_group_create();
        dispatch_semaphore_t entered = dispatch_semaphore_create(0);
        __block int bad = 0;
        dispatch_group_async(readers, dispatch_get_global_queue(QOS_CLASS_DEFAULT, 0), ^{
            char output[2], detail[256];
            dispatch_semaphore_signal(entered);
            bad = [rec read:output size:sizeof(output) error:detail errorSize:sizeof(detail)] != 0;
        });
        dispatch_semaphore_wait(entered, DISPATCH_TIME_FOREVER);
        usleep(10000);
        [rec stop];
        CHECK(dispatch_group_wait(readers, dispatch_time(DISPATCH_TIME_NOW, NSEC_PER_SEC)) == 0 && !bad,
              "stop must wake a blocked reader before capture destruction");
        dispatch_release(entered); dispatch_release(readers); [rec release];

        rec = [[JTCapture alloc] init];
        [rec appendPCM:[NSMutableData dataWithLength:JT_CAPTURE_MAX_PENDING]];
        [rec appendPCM:[NSData dataWithBytes:bytes length:2]];
        NSMutableData *all = [NSMutableData dataWithLength:JT_CAPTURE_MAX_PENDING];
        CHECK([rec read:all.mutableBytes size:all.length error:error errorSize:sizeof(error)] == JT_CAPTURE_MAX_PENDING,
              "consumer overflow must preserve audio captured before the error");
        CHECK([rec read:bytes size:sizeof(bytes) error:error errorSize:sizeof(error)] == -1,
              "consumer overflow must be reported instead of silently dropping audio");
        [rec stop]; [rec release];
    }
    return 0;
}
`
