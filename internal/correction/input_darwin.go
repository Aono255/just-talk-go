//go:build darwin && cgo

package correction

// #cgo CFLAGS: -x objective-c
// #cgo LDFLAGS: -framework ApplicationServices -framework AppKit
// #include <stdlib.h>
// #include "context_darwin.h"
import "C"

import (
	"encoding/json"
	"errors"
	"sync"
	"unsafe"
)

func captureInputPlatform() (*Input, error) {
	app, err := currentApplication()
	if err != nil {
		return nil, err
	}
	var native *C.jt_input_target
	var data, failure *C.char
	status := C.jt_input_capture(C.pid_t(app.PID), &native, &data, &failure)
	if data != nil {
		defer C.free(unsafe.Pointer(data))
	}
	if failure != nil {
		defer C.free(unsafe.Pointer(failure))
	}
	if status == 0 {
		return nil, ErrUnsupportedInput
	}
	if status != 1 {
		return nil, nativeInputError(failure)
	}
	var snapshot struct{ Value, Prefix, Suffix, Role string }
	if data == nil || json.Unmarshal([]byte(C.GoString(data)), &snapshot) != nil {
		C.jt_input_release(native)
		return nil, errors.New("输入框快照格式无效")
	}
	var closeOnce sync.Once
	return &Input{Before: snapshot.Value, Prefix: snapshot.Prefix, Suffix: snapshot.Suffix, Role: snapshot.Role,
		Close: func() { closeOnce.Do(func() { C.jt_input_release(native) }) },
		Guard: func() error {
			var failure *C.char
			if C.jt_input_guard(native, &failure) == 1 {
				return nil
			}
			if failure != nil {
				defer C.free(unsafe.Pointer(failure))
			}
			return nativeInputError(failure)
		},
		Read: func() (string, error) {
			var value, failure *C.char
			status := C.jt_input_read(native, &value, &failure)
			if value != nil {
				defer C.free(unsafe.Pointer(value))
			}
			if failure != nil {
				defer C.free(unsafe.Pointer(failure))
			}
			if status != 1 || value == nil {
				return "", nativeInputError(failure)
			}
			return C.GoString(value), nil
		},
	}, nil
}

func nativeInputError(failure *C.char) error {
	if failure != nil {
		return errors.New(C.GoString(failure))
	}
	return errors.New("当前输入框无法读取或绑定已变化")
}

func captureDestinationPlatform() (*Destination, error) {
	app, err := currentApplication()
	if err != nil {
		return nil, err
	}
	var native *C.jt_input_target
	var failure *C.char
	status := C.jt_window_capture(C.pid_t(app.PID), &native, &failure)
	if failure != nil {
		defer C.free(unsafe.Pointer(failure))
	}
	if status != 1 {
		return nil, nativeInputError(failure)
	}
	var closeOnce sync.Once
	return &Destination{Close: func() { closeOnce.Do(func() { C.jt_input_release(native) }) }, Guard: func() error {
		front, err := currentApplication()
		if err != nil {
			return err
		}
		var failure *C.char
		status := C.jt_window_guard(native, C.pid_t(front.PID), &failure)
		if failure != nil {
			defer C.free(unsafe.Pointer(failure))
		}
		if status != 1 {
			return nativeInputError(failure)
		}
		return nil
	}}, nil
}
