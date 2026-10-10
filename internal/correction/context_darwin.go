//go:build darwin && cgo

package correction

// #cgo CFLAGS: -x objective-c
// #cgo LDFLAGS: -framework ApplicationServices -framework AppKit
// #include <stdlib.h>
// #include "context_darwin.h"
import "C"

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"
	"unsafe"
)

type applicationIdentity struct {
	PID      int    `json:"pid"`
	BundleID string `json:"bundle_id"`
}

func focusHelperPlatform(output io.Writer) error {
	var pid C.pid_t
	var bundle, failure *C.char
	status := C.jt_correction_frontmost(&pid, &bundle, &failure)
	if bundle != nil {
		defer C.free(unsafe.Pointer(bundle))
	}
	if failure != nil {
		defer C.free(unsafe.Pointer(failure))
	}
	if status != 1 || bundle == nil {
		if failure != nil {
			return errors.New(C.GoString(failure))
		}
		return errors.New("当前输入应用检测失败")
	}
	return json.NewEncoder(output).Encode(applicationIdentity{PID: int(pid), BundleID: C.GoString(bundle)})
}

func currentApplication() (applicationIdentity, error) {
	executable, err := os.Executable()
	if err != nil {
		return applicationIdentity{}, errors.New("无法找到输入应用检测助手")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return queryApplication(ctx, executable)
}

func queryApplication(ctx context.Context, executable string) (applicationIdentity, error) {
	data, err := exec.CommandContext(ctx, executable, "--focus-helper").Output()
	if err != nil {
		if ctx.Err() != nil {
			return applicationIdentity{}, errors.New("当前输入应用检测超时，未执行纠错或自动上屏")
		}
		return applicationIdentity{}, errors.New("当前输入应用检测助手失败，未执行纠错或自动上屏")
	}
	var app applicationIdentity
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if len(data) > 4096 || decoder.Decode(&app) != nil || app.PID <= 0 || app.PID > 1<<31-1 || app.BundleID == "" || len(app.BundleID) > 256 {
		return applicationIdentity{}, errors.New("当前输入应用检测结果无效")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return applicationIdentity{}, errors.New("当前输入应用检测返回多余内容")
	}
	return app, nil
}

func capturePlatform() (*Target, error) {
	app, err := currentApplication()
	if err != nil {
		return nil, err
	}
	if app.BundleID != "com.openai.codex" {
		return nil, ErrNotCodex
	}
	var native *C.jt_correction_target
	var data, failure *C.char
	status := C.jt_correction_capture(C.pid_t(app.PID), &native, &data, &failure)
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
	messages, err = Recent(messages)
	if err != nil {
		C.jt_correction_release(native)
		return nil, err
	}
	return &Target{Messages: messages, AvailableUsers: availableUsers, AvailableAssistants: availableAssistants, Close: func() { C.jt_correction_release(native) },
		Guard: func() error {
			front, err := currentApplication()
			if err != nil {
				return err
			}
			if front.BundleID != "com.openai.codex" {
				return errors.New("纠错期间切换了应用，结果未上屏")
			}
			var errorText *C.char
			if C.jt_correction_guard(native, C.pid_t(front.PID), &errorText) == 1 {
				return nil
			}
			if errorText != nil {
				defer C.free(unsafe.Pointer(errorText))
				return errors.New(C.GoString(errorText))
			}
			return errors.New("Codex 输入框已经变化")
		}}, nil
}
