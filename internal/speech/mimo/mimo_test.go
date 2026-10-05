// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package mimo

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c/just-talk-go/internal/speech"
)

func TestParseConfig(t *testing.T) {
	if _, err := ParseConfig(nil); err == nil {
		t.Fatal("want error when api_key missing")
	}
	if _, err := ParseConfig(map[string]string{"api_key": " k "}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c, err := ParseConfig(map[string]string{"api_key": "k"})
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL != "https://api.xiaomimimo.com/v1" {
		t.Fatalf("default base_url = %q", c.BaseURL)
	}
	if c.Model != "mimo-v2.5-asr" {
		t.Fatalf("default model = %q", c.Model)
	}
	// 带尾斜杠的 base_url 会被去尾
	c2, _ := ParseConfig(map[string]string{"api_key": "k", "base_url": "https://x.example/v1/"})
	if c2.BaseURL != "https://x.example/v1" {
		t.Fatalf("base_url = %q", c2.BaseURL)
	}
}

func TestEngineNameAndCapabilities(t *testing.T) {
	e, err := NewFromMap(map[string]string{"api_key": "k"})
	if err != nil {
		t.Fatal(err)
	}
	if e.Name() != "mimo-asr" {
		t.Fatalf("name = %q", e.Name())
	}
	if !e.Capabilities().STT {
		t.Fatal("should have STT")
	}
	if _, err := e.Speak(context.Background(), speech.SpeakRequest{}); err != speech.ErrUnsupported {
		t.Fatalf("Speak err = %v, want ErrUnsupported", err)
	}
}

func TestEngineAvailable(t *testing.T) {
	ok, detail := NewFromMapMust(t, map[string]string{"api_key": "k"}).Available(context.Background())
	if !ok {
		t.Fatalf("Available should be true, detail=%q", detail)
	}
}

func TestEngineTranscribe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != "k123" {
			http.Error(w, "bad key", http.StatusUnauthorized)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body)
		lang := ""
		if opts, ok := body["asr_options"].(map[string]any); ok {
			lang, _ = opts["language"].(string)
		}
		if lang != "zh" {
			http.Error(w, "want language zh", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"语音识别结果"}}]}`))
	}))
	t.Cleanup(srv.Close)

	e, err := NewFromMap(map[string]string{"api_key": "k123", "base_url": srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.Transcribe(context.Background(), speech.TranscribeRequest{
		Audio: speech.EncodeWAV([]byte{0, 0, 1, 2}), Lang: "zh",
	})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if res.Text != "语音识别结果" {
		t.Fatalf("text = %q", res.Text)
	}
	if res.Engine != "mimo-asr" {
		t.Fatalf("engine = %q", res.Engine)
	}
}

func TestEngineTranscribeEmpty(t *testing.T) {
	e, err := NewFromMap(map[string]string{"api_key": "k"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Transcribe(context.Background(), speech.TranscribeRequest{}); err != speech.ErrInvalidInput {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestEngineRegistry(t *testing.T) {
	eng, err := speech.Build("mimo-asr", map[string]string{"api_key": "k"})
	if err != nil {
		t.Fatalf("build via registry: %v", err)
	}
	if eng.Name() != "mimo-asr" {
		t.Fatalf("name = %q", eng.Name())
	}
}

// NewFromMapMust 构造引擎，配置非法时直接失败（测试辅助）。
func NewFromMapMust(t *testing.T, m map[string]string) speech.Engine {
	t.Helper()
	e, err := NewFromMap(m)
	if err != nil {
		t.Fatalf("NewFromMap(%v): %v", m, err)
	}
	return e
}
