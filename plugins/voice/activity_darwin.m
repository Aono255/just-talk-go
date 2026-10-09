//go:build darwin && cgo

#import <Foundation/Foundation.h>
#include "capture_darwin.h"

void *jt_audio_activity_begin(void) {
    @autoreleasepool {
        id token = [[NSProcessInfo processInfo]
            beginActivityWithOptions:NSActivityUserInitiatedAllowingIdleSystemSleep | NSActivityLatencyCritical
            reason:@"Just Talk user-requested microphone recording"];
        return [token retain];
    }
}

void jt_audio_activity_end(void *token) {
    if (!token) return;
    @autoreleasepool {
        [[NSProcessInfo processInfo] endActivity:(id)token];
        [(id)token release];
    }
}
