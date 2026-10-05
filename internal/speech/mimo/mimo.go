// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

// Package mimo 实现小米 MiMo ASR（mimo-v2.5-asr）批处理转写引擎。
// 接口形态为 OpenAI Chat Completions + input_audio（base64 WAV），
// 复用 chataudio 通用客户端；仅支持批处理，录音场景由 BatchSession 包装。
package mimo

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/c/just-talk-go/internal/speech"
	"github.com/c/just-talk-go/internal/speech/chataudio"
)

func init() {
	speech.Register("mimo-asr", NewFromMap)
}

// 配置键：
//
//	api_key   必填（小米 MiMo API Key）
//	base_url  默认 https://api.xiaomimimo.com/v1
//	model     默认 mimo-v2.5-asr
//	language  可选 BCP-47 / auto / zh / en；空 = 自动

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
		c.BaseURL = "https://api.xiaomimimo.com/v1"
	}
	if c.Model == "" {
		c.Model = "mimo-v2.5-asr"
	}
	if c.APIKey == "" {
		return Config{}, fmt.Errorf("mimo: 需要 api_key")
	}
	return c, nil
}

// Engine 实现 speech.Engine（仅 STT，批处理）。
type Engine struct {
	cfg    Config
	client *chataudio.Client
}

// NewFromMap 工厂：map 配置 → Engine。
func NewFromMap(m map[string]string) (speech.Engine, error) {
	cfg, err := ParseConfig(m)
	if err != nil {
		return nil, err
	}
	return &Engine{
		cfg: cfg,
		client: &chataudio.Client{
			BaseURL:    cfg.BaseURL,
			AuthHeader: "api-key",
			APIKey:     cfg.APIKey,
			Model:      cfg.Model,
			HTTP:       &http.Client{Timeout: 120 * time.Second},
		},
	}, nil
}

func (e *Engine) Name() string { return "mimo-asr" }

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
	text, err := e.client.Transcribe(ctx, req.Audio, lang)
	if err != nil {
		return nil, err
	}
	return &speech.TranscribeResult{Text: text, Engine: e.Name()}, nil
}
