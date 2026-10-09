//go:build darwin

package main

import "runtime"

func init() {
	// AppKit's overlay helper must initialize and run on the process main thread.
	runtime.LockOSThread()
}
