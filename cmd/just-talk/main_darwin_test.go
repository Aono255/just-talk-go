//go:build darwin && cgo

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Exercise the real Go entry and Core Foundation loop in a separate process:
// test goroutines themselves are not on the macOS process main thread.
// Notifications are synthetic; this never opens a microphone or reads UI.
func TestDarwinMainLoopLifecycle(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"main_darwin.go", "main_loop_darwin.h", "main_loop_darwin.m"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range map[string]string{
		"go.mod":  "module justtalk-main-loop-test\n\ngo 1.25.0\n",
		"main.go": darwinMainLoopFixture,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	binary := filepath.Join(dir, "main-loop-test")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-race", "-o", binary, ".")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build native main-loop fixture: %v\n%s", err, out)
	}
	for _, scenario := range []string{"idle-notifications", "fast-return", "prequeued-stop", "wrong-thread"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, binary, scenario).CombinedOutput()
			if err != nil || !strings.Contains(string(out), scenario+" ok") {
				t.Fatalf("main-loop scenario %s: %v\n%s", scenario, err, out)
			}
		})
	}
}

const darwinMainLoopFixture = `package main

// #include <CoreFoundation/CoreFoundation.h>
// #include <dispatch/dispatch.h>
// #include "main_loop_darwin.h"
// static int jt_test_device_notifications(void) {
//     __block int oldConnected = 1, newConnected = 0, generation = 0, valid = 0;
//     dispatch_async(dispatch_get_main_queue(), ^{
//         oldConnected = 0;
//         generation++;
//     });
//     dispatch_async(dispatch_get_main_queue(), ^{
//         newConnected = 1;
//         generation++;
//     });
//     dispatch_sync(dispatch_get_main_queue(), ^{
//         valid = CFRunLoopGetCurrent() == CFRunLoopGetMain() &&
//                 !oldConnected && newConnected && generation == 2;
//     });
//     return valid;
// }
import "C"

import (
    "fmt"
    "os"
    "time"
)

func main() {
    scenario := os.Args[1]
    switch scenario {
    case "idle-notifications":
        runPlatformMain(func() {
            // A synchronous main-queue barrier would deadlock if the worker
            // ran on the main thread or the loop stopped during idle.
            for i := 0; i < 3; i++ {
                time.Sleep(20 * time.Millisecond)
                if C.jt_test_device_notifications() != 1 {
                    panic("device notifications were not delivered on the main thread")
                }
            }
        })
    case "fast-return":
        calls := 0
        for i := 0; i < 500; i++ {
            runPlatformMain(func() { calls++ })
        }
        if calls != 500 { panic("application return was lost") }
    case "prequeued-stop":
        // Force the ordering that loses a direct CFRunLoopStop: stop first,
        // start later. The production queued stop must still be delivered.
        for i := 0; i < 50; i++ {
            C.jt_main_loop_request_stop()
            if C.jt_main_loop_run() != 0 { panic("prequeued stop failed") }
        }
    case "wrong-thread":
        runPlatformMain(func() {
            if C.jt_main_loop_run() != -1 { panic("background thread accepted as main") }
        })
    default:
        panic("unknown scenario")
    }
    fmt.Println(scenario + " ok")
}
`
