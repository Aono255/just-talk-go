// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package speech

import "context"

// Null 未配置语音引擎时的默认实现：不可用，调用一律 ErrNotConfigured。
type Null struct{}

func (Null) Name() string { return "null" }

func (Null) Capabilities() Capabilities {
	return Capabilities{}
}

func (Null) Available(context.Context) (bool, string) {
	return false, "未配置语音引擎"
}

func (Null) Speak(context.Context, SpeakRequest) (*SpeakResult, error) {
	return nil, ErrNotConfigured
}

func (Null) Transcribe(context.Context, TranscribeRequest) (*TranscribeResult, error) {
	return nil, ErrNotConfigured
}

func (Null) ListVoices(context.Context) ([]Voice, error) {
	return nil, ErrNotConfigured
}

// OrNull 保证调用方永不拿到 nil Engine。
func OrNull(e Engine) Engine {
	if e == nil {
		return Null{}
	}
	return e
}
