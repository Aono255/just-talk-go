// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package dashscope

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseConfig(t *testing.T) {
	c, err := ParseConfig(map[string]string{"api_key": "sk-123"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Model != "fun-asr-realtime" {
		t.Fatalf("default model = %q", c.Model)
	}
	if c.BaseURL != "wss://dashscope.aliyuncs.com/api-ws/v1/inference/" {
		t.Fatalf("default base_url = %q", c.BaseURL)
	}
	if c.SampleRate != 16000 {
		t.Fatalf("default sample_rate = %d", c.SampleRate)
	}

	// 8k 模型自动降采样
	c, err = ParseConfig(map[string]string{"api_key": "sk-123", "model": "fun-asr-flash-8k-realtime"})
	if err != nil {
		t.Fatal(err)
	}
	if c.SampleRate != 8000 {
		t.Fatalf("8k model sample_rate = %d, want 8000", c.SampleRate)
	}

	// 显式 sample_rate 覆盖
	c, err = ParseConfig(map[string]string{"api_key": "sk-123", "model": "fun-asr-flash-8k-realtime", "sample_rate": "16000"})
	if err != nil {
		t.Fatal(err)
	}
	if c.SampleRate != 16000 {
		t.Fatalf("explicit sample_rate = %d", c.SampleRate)
	}

	// 非法采样率
	if _, err := ParseConfig(map[string]string{"api_key": "sk-123", "sample_rate": "44100"}); err == nil {
		t.Fatal("expected error for 44100")
	}

	// 缺 key
	if _, err := ParseConfig(nil); err == nil {
		t.Fatal("expected error without api_key")
	}
}

func TestLanguageHint(t *testing.T) {
	for in, want := range map[string]string{
		"":      "",
		"zh-CN": "zh",
		"en_US": "en",
		"zh":    "zh",
		"  ja ": "ja",
		"pt-BR": "pt",
	} {
		if got := languageHint(in); got != want {
			t.Errorf("languageHint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEncodeRunTask(t *testing.T) {
	cfg := Config{APIKey: "sk-1", Model: "fun-asr-realtime", SampleRate: 16000, BaseURL: "wss://x"}
	b, err := encodeRunTask(cfg, "task-abc", "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	var ev clientEvent
	if err := json.Unmarshal(b, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Header.Action != "run-task" {
		t.Fatalf("action = %q", ev.Header.Action)
	}
	if ev.Header.TaskID != "task-abc" {
		t.Fatalf("task_id = %q", ev.Header.TaskID)
	}
	if ev.Header.Streaming != "duplex" {
		t.Fatalf("streaming = %q", ev.Header.Streaming)
	}
	if ev.Payload.TaskGroup != "audio" || ev.Payload.Task != "asr" || ev.Payload.Function != "recognition" {
		t.Fatalf("payload task = %+v", ev.Payload)
	}
	if ev.Payload.Model != "fun-asr-realtime" {
		t.Fatalf("model = %q", ev.Payload.Model)
	}
	if ev.Payload.Parameters["format"] != "pcm" {
		t.Fatalf("format = %v", ev.Payload.Parameters["format"])
	}
	if ev.Payload.Parameters["sample_rate"] != float64(16000) {
		t.Fatalf("sample_rate = %v", ev.Payload.Parameters["sample_rate"])
	}
	hints, _ := ev.Payload.Parameters["language_hints"].([]any)
	if len(hints) != 1 || hints[0] != "zh" {
		t.Fatalf("language_hints = %v", ev.Payload.Parameters["language_hints"])
	}

	// 无语言时不出 language_hints
	b, _ = encodeRunTask(cfg, "t2", "")
	ev = clientEvent{}
	_ = json.Unmarshal(b, &ev)
	if _, ok := ev.Payload.Parameters["language_hints"]; ok {
		t.Fatal("language_hints should be absent when lang empty")
	}
}

func TestEncodeFinishTask(t *testing.T) {
	b, err := encodeFinishTask("task-abc")
	if err != nil {
		t.Fatal(err)
	}
	var ev clientEvent
	if err := json.Unmarshal(b, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Header.Action != "finish-task" || ev.Header.TaskID != "task-abc" {
		t.Fatalf("header = %+v", ev.Header)
	}
}

func TestDownsample16kTo8k(t *testing.T) {
	// 构造 16k s16le：两两平均应等于 (a+b)/2
	pcm := make([]byte, 8) // 4 个样本
	samples := []int16{1000, 2000, -1000, -3000}
	for i, s := range samples {
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(s))
	}
	out := downsample16kTo8k(pcm)
	if len(out) != 4 {
		t.Fatalf("out len = %d, want 4", len(out))
	}
	want := []int16{1500, -2000}
	for i, w := range want {
		got := int16(binary.LittleEndian.Uint16(out[i*2:]))
		if got != w {
			t.Errorf("out[%d] = %d, want %d", i, got, w)
		}
	}

	// 奇数样本丢弃尾部
	out = downsample16kTo8k(make([]byte, 10))
	if len(out) != 4 {
		t.Fatalf("odd input out len = %d, want 4", len(out))
	}
}

func TestEngineNameAndCapabilities(t *testing.T) {
	eng, err := NewFromMap(map[string]string{"api_key": "sk-1"})
	if err != nil {
		t.Fatal(err)
	}
	if eng.Name() != "dashscope" {
		t.Fatalf("name = %q", eng.Name())
	}
	caps := eng.Capabilities()
	if !caps.STT || !caps.StreamingSTT {
		t.Fatalf("capabilities = %+v", caps)
	}
	ok, detail := eng.Available(nil)
	if !ok || !strings.Contains(detail, "fun-asr-realtime") {
		t.Fatalf("available = %v %q", ok, detail)
	}
}
