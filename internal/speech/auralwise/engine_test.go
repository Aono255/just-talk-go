// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package auralwise

import (
	"testing"
)

func TestParseConfigDefaults(t *testing.T) {
	c, err := ParseConfig(map[string]string{"api_key": "asr_testkey"})
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL != "https://api.auralwise.cn/v1" {
		t.Fatalf("base %s", c.BaseURL)
	}
	if c.Optimize != "auto" {
		t.Fatalf("optimize %s", c.Optimize)
	}
	if c.EnableDiarize || c.EnableEvents {
		t.Fatal("diarize/events should default false")
	}
}

func TestParseConfigRequiresKey(t *testing.T) {
	if _, err := ParseConfig(map[string]string{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestExtractResultJoinsSegments(t *testing.T) {
	raw := []byte(`{
		"language":"zh",
		"language_probability":0.98,
		"segments":[
			{"id":0,"text":"你好"},
			{"id":1,"text":"世界"},
			{"id":2,"text":"Hello"},
			{"id":3,"text":"World"}
		]
	}`)
	text, lang, conf, err := extractResult(raw)
	if err != nil {
		t.Fatal(err)
	}
	if lang != "zh" || conf < 0.9 {
		t.Fatalf("lang/conf %s %v", lang, conf)
	}
	// 中文之间无空格；英文段之间有空格
	if text != "你好世界Hello World" {
		t.Fatalf("text %q", text)
	}
}

func TestBuildOptionsLanguageNormalize(t *testing.T) {
	e := &Engine{cfg: Config{Optimize: "false", Language: "zh-CN"}}
	opt := e.buildOptions("")
	if opt["asr_language"] != "zh" {
		t.Fatalf("%v", opt["asr_language"])
	}
	if opt["optimize"] != false {
		t.Fatalf("optimize %v", opt["optimize"])
	}
	e2 := &Engine{cfg: Config{Optimize: "auto"}}
	opt2 := e2.buildOptions("en")
	if _, ok := opt2["optimize"]; ok {
		t.Fatal("auto should omit optimize")
	}
}

func TestFilenameFromContentType(t *testing.T) {
	if filenameFromContentType("audio/webm;codecs=opus") != "recording.webm" {
		t.Fatal("webm")
	}
	if filenameFromContentType("audio/wav") != "recording.wav" {
		t.Fatal("wav")
	}
}
