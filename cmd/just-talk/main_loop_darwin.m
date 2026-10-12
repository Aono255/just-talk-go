//go:build darwin && cgo

#import <Foundation/Foundation.h>
#import <CoreFoundation/CoreFoundation.h>
#include "main_loop_darwin.h"

// Only accessed by the main thread, including the queued stop block.
static BOOL jt_main_loop_finished;

static void jt_main_loop_keep_alive(void *info) { (void)info; }

int jt_main_loop_run(void) {
    @autoreleasepool {
        if (![NSThread isMainThread]) return -1;
        CFRunLoopRef loop = CFRunLoopGetMain();
        CFRunLoopSourceContext context = {0};
        context.perform = jt_main_loop_keep_alive;
        CFRunLoopSourceRef source = CFRunLoopSourceCreate(kCFAllocatorDefault, 0, &context);
        if (!source) return -2;
        jt_main_loop_finished = NO;
        // Keep the loop alive even before AVFoundation installs its sources.
        CFRunLoopAddSource(loop, source, kCFRunLoopDefaultMode);
        while (!jt_main_loop_finished) {
            @autoreleasepool {
                CFRunLoopRunInMode(kCFRunLoopDefaultMode, 1.0e10, true);
            }
        }
        CFRunLoopRemoveSource(loop, source, kCFRunLoopDefaultMode);
        CFRunLoopSourceInvalidate(source);
        CFRelease(source);
        return 0;
    }
}

void jt_main_loop_request_stop(void) {
    CFRunLoopRef loop = CFRunLoopGetMain();
    CFRunLoopPerformBlock(loop, kCFRunLoopDefaultMode, ^{
        jt_main_loop_finished = YES;
        CFRunLoopStop(loop);
    });
    CFRunLoopWakeUp(loop);
}
