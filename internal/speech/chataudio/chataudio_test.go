// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package chataudio

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureServer 记录收到的请求体与认证头，返回可配置的响应。
func captureServer(t *testing.T, status int, respBody string) (*httptest.Server, *chatRequest, *string) {
	t.Helper()
	var got chatRequest
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}
		authHeader = r.Header.Get("api-key")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)
	return srv, &got, &authHeader
}

func TestTranscribeSuccess(t *testing.T) {
	wav := []byte("RIFFxxxxWAVEfake-audio")
	srv, got, auth := captureServer(t, 200,
		`{"id":"x","choices":[{"message":{"content":"你好世界","role":"assistant"}}]}`)
	c := &Client{BaseURL: srv.URL, AuthHeader: "api-key", APIKey: "k123", Model: "mimo-v2.5-asr"}

	text, err := c.Transcribe(context.Background(), wav, "zh")
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if text != "你好世界" {
		t.Fatalf("text = %q, want 你好世界", text)
	}
	if *auth != "k123" {
		t.Fatalf("api-key header = %q, want k123", *auth)
	}
	if got.Model != "mimo-v2.5-asr" {
		t.Fatalf("model = %q", got.Model)
	}
	if got.Stream {
		t.Fatal("stream should be false (batch)")
	}
	if len(got.Messages) != 1 || len(got.Messages[0].Content) != 1 {
		t.Fatalf("messages structure wrong: %+v", got.Messages)
	}
	content := got.Messages[0].Content[0]
	if content.Type != "input_audio" || content.InputAudio == nil {
		t.Fatalf("content type wrong: %+v", content)
	}
	wantData := "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(wav)
	if content.InputAudio.Data != wantData {
		t.Fatalf("input_audio.data wrong: %q", content.InputAudio.Data)
	}
	if got.ASROpts == nil || got.ASROpts.Language != "zh" {
		t.Fatalf("asr_options = %+v, want language=zh", got.ASROpts)
	}
}

func TestTranscribeNoLanguage(t *testing.T) {
	srv, got, _ := captureServer(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`)
	c := &Client{BaseURL: srv.URL, AuthHeader: "api-key", APIKey: "k", Model: "m"}

	if _, err := c.Transcribe(context.Background(), []byte("audio"), ""); err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if got.ASROpts != nil {
		t.Fatalf("asr_options should be nil when lang empty, got %+v", got.ASROpts)
	}
}

func TestTranscribeHTTPError(t *testing.T) {
	srv, _, _ := captureServer(t, 401, `{"error":{"message":"invalid api key","type":"auth"}}`)
	c := &Client{BaseURL: srv.URL, AuthHeader: "api-key", APIKey: "bad", Model: "m"}

	_, err := c.Transcribe(context.Background(), []byte("audio"), "")
	if err == nil {
		t.Fatal("want error for HTTP 401")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("error = %v, want 401 + message", err)
	}
}

func TestTranscribeEmptyAudio(t *testing.T) {
	c := &Client{BaseURL: "http://x", Model: "m"}
	if _, err := c.Transcribe(context.Background(), nil, ""); err == nil {
		t.Fatal("want error for empty audio")
	}
}

func TestTranscribeNoChoices(t *testing.T) {
	srv, _, _ := captureServer(t, 200, `{"choices":[]}`)
	c := &Client{BaseURL: srv.URL, Model: "m"}
	if _, err := c.Transcribe(context.Background(), []byte("a"), ""); err == nil {
		t.Fatal("want error for empty choices")
	}
}
