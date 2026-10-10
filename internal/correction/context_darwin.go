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
	"unsafe"
)

func capturePlatform() (*Target, error) {
	var native *C.jt_correction_target
	var data, failure *C.char
	status := C.jt_correction_capture(&native, &data, &failure)
	if data != nil {
		defer C.free(unsafe.Pointer(data))
	}
	if failure != nil {
		defer C.free(unsafe.Pointer(failure))
	}
	if status == 0 {
		return nil, ErrNotCodex
	}
	if status != 1 {
		if failure != nil {
			return nil, errors.New(C.GoString(failure))
		}
		return nil, errors.New("无法读取当前 Codex 聊天上下文")
	}
	var messages []Message
	if json.Unmarshal([]byte(C.GoString(data)), &messages) != nil {
		C.jt_correction_release(native)
		return nil, errors.New("当前聊天上下文格式不正确")
	}
	availableUsers, availableAssistants := 0, 0
	for _, message := range messages {
		if message.Role == "user" {
			availableUsers++
		} else {
			availableAssistants++
		}
	}
	messages, err := Recent(messages)
	if err != nil {
		C.jt_correction_release(native)
		return nil, err
	}
	return &Target{Messages: messages, AvailableUsers: availableUsers, AvailableAssistants: availableAssistants, Close: func() { C.jt_correction_release(native) },
		Guard: func() error {
			var errorText *C.char
			if C.jt_correction_guard(native, &errorText) == 1 {
				return nil
			}
			if errorText != nil {
				defer C.free(unsafe.Pointer(errorText))
				return errors.New(C.GoString(errorText))
			}
			return errors.New("Codex 输入框已经变化")
		}}, nil
}
