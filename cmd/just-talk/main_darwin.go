//go:build darwin && cgo

package main

// #cgo CFLAGS: -fblocks
// #cgo LDFLAGS: -framework CoreFoundation -framework Foundation
// #include "main_loop_darwin.h"
import "C"

import (
	"fmt"
	"os"
	"runtime"
)

func init() {
	// AppKit helpers and the device notification loop belong to the process main thread.
	runtime.LockOSThread()
}

func runPlatformMain(run func()) {
	done := make(chan struct{})
	go func() {
		// Queue the stop instead of stopping an as-yet inactive run loop. Even
		// an application that returns immediately must let the main loop exit.
		defer C.jt_main_loop_request_stop()
		defer close(done)
		run()
	}()
	if status := C.jt_main_loop_run(); status != 0 {
		fmt.Fprintf(os.Stderr, "macOS 主事件循环启动失败: %d\n", status)
		os.Exit(1)
	}
	<-done
}
