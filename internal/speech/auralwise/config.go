// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package auralwise

import (
	"fmt"
	"strings"
	"time"
)

// 配置键（speech.Config.config）：
//
//	api_key       必填，X-API-Key（asr_…）
//	base_url      默认 https://api.auralwise.cn/v1
//	language      asr_language，空=自动
//	optimize      auto|true|false（默认 auto → 不传 optimize，由服务按语言路由）
//	enable_diarize true/false（默认 false；聊天转写不需要说话人）
//	enable_events  true/false（默认 false；声音事件）
//	poll_interval  轮询间隔，默认 3s
//	poll_timeout   最长等待，默认 10m

type Config struct {
	APIKey        string
	BaseURL       string
	Language      string
	Optimize      string // auto | true | false
	EnableDiarize bool
	EnableEvents  bool
	PollInterval  time.Duration
	PollTimeout   time.Duration
}

func ParseConfig(m map[string]string) (Config, error) {
	if m == nil {
		m = map[string]string{}
	}
	c := Config{
		APIKey:        strings.TrimSpace(m["api_key"]),
		BaseURL:       strings.TrimRight(strings.TrimSpace(m["base_url"]), "/"),
		Language:      strings.TrimSpace(m["language"]),
		Optimize:      strings.ToLower(strings.TrimSpace(m["optimize"])),
		EnableDiarize: parseBoolDefault(m["enable_diarize"], false),
		EnableEvents:  parseBoolDefault(firstNonEmpty(m["enable_events"], m["enable_audio_events"]), false),
		PollInterval:  parseDurationDefault(m["poll_interval"], 3*time.Second),
		PollTimeout:   parseDurationDefault(m["poll_timeout"], 10*time.Minute),
	}
	if c.BaseURL == "" {
		c.BaseURL = "https://api.auralwise.cn/v1"
	}
	if c.Optimize == "" {
		c.Optimize = "auto"
	}
	if c.APIKey == "" {
		return Config{}, fmt.Errorf("auralwise: 需要 api_key")
	}
	switch c.Optimize {
	case "auto", "true", "false":
	default:
		return Config{}, fmt.Errorf("auralwise: optimize 须为 auto|true|false")
	}
	if c.PollInterval < time.Second {
		c.PollInterval = time.Second
	}
	if c.PollTimeout < 10*time.Second {
		c.PollTimeout = 10 * time.Second
	}
	return c, nil
}

func parseBoolDefault(s string, def bool) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return def
	}
	switch s {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func parseDurationDefault(s string, def time.Duration) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

func firstNonEmpty(a, b string) string {
	a = strings.TrimSpace(a)
	if a != "" {
		return a
	}
	return strings.TrimSpace(b)
}
