// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package openai

// OpenAI 兼容 Whisper 转写引擎（批处理）：
// POST {base_url}/audio/transcriptions，multipart/form-data。
// 兼容 OpenAI、Groq、本地 whisper.cpp server / faster-whisper 等任何 OpenAI 风格端点。
//
// 仅支持批处理（speech.Engine），录音场景由 BatchSession 包装成流式会话。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/c/just-talk-go/internal/speech"
)

func init() {
	speech.Register("openai", NewFromMap)
}

// 配置键：
//
//	api_key     必填，Bearer token
//	base_url    默认 https://api.openai.com/v1（可指向兼容端点，如 Groq / 本地 whisper）
//	model       默认 whisper-1
//	language    可选 BCP-47；空 = 自动

type Config struct {
	APIKey   string
	BaseURL  string
	Model    string
	Language string
}

func ParseConfig(m map[string]string) (Config, error) {
	if m == nil {
		m = map[string]string{}
	}
	c := Config{
		APIKey:   strings.TrimSpace(m["api_key"]),
		BaseURL:  strings.TrimRight(strings.TrimSpace(m["base_url"]), "/"),
		Model:    strings.TrimSpace(m["model"]),
		Language: strings.TrimSpace(m["language"]),
	}
	if c.BaseURL == "" {
		c.BaseURL = "https://api.openai.com/v1"
	}
	if c.Model == "" {
		c.Model = "whisper-1"
	}
	if c.APIKey == "" {
		return Config{}, fmt.Errorf("openai: 需要 api_key")
	}
	return c, nil
}

// Engine 实现 speech.Engine（仅 STT）。
type Engine struct {
	cfg    Config
	client *http.Client
}

// NewFromMap 工厂：map 配置 → Engine。
func NewFromMap(m map[string]string) (speech.Engine, error) {
	cfg, err := ParseConfig(m)
	if err != nil {
		return nil, err
	}
	return &Engine{
		cfg: cfg,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}, nil
}

func (e *Engine) Name() string {
	return "openai"
}

func (e *Engine) Capabilities() speech.Capabilities {
	return speech.Capabilities{STT: true}
}

func (e *Engine) Available(context.Context) (bool, string) {
	if e.cfg.APIKey == "" {
		return false, "未配置 api_key"
	}
	return true, fmt.Sprintf("base=%s model=%s", e.cfg.BaseURL, e.cfg.Model)
}

func (e *Engine) Speak(context.Context, speech.SpeakRequest) (*speech.SpeakResult, error) {
	return nil, speech.ErrUnsupported
}

func (e *Engine) ListVoices(context.Context) ([]speech.Voice, error) {
	return nil, speech.ErrUnsupported
}

func (e *Engine) Transcribe(ctx context.Context, req speech.TranscribeRequest) (*speech.TranscribeResult, error) {
	if len(req.Audio) == 0 {
		return nil, speech.ErrInvalidInput
	}
	lang := req.Lang
	if lang == "" {
		lang = e.cfg.Language
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fn := "recording.wav"
	if strings.Contains(strings.ToLower(req.ContentType), "mp3") {
		fn = "recording.mp3"
	}
	fw, err := mw.CreateFormFile("file", fn)
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(req.Audio); err != nil {
		return nil, err
	}
	_ = mw.WriteField("model", e.cfg.Model)
	_ = mw.WriteField("response_format", "json")
	if lang != "" {
		_ = mw.WriteField("language", lang)
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.BaseURL+"/audio/transcriptions", &buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", mw.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+e.cfg.APIKey)

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai transcribe: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai transcribe HTTP %s: %s", resp.Status, truncate(string(data), 300))
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("openai transcribe json: %w", err)
	}
	return &speech.TranscribeResult{
		Text:   strings.TrimSpace(out.Text),
		Lang:   lang,
		Engine: e.Name(),
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
