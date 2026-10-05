// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package dashscope

// 千问 AI 平台（DashScope）Fun-ASR 实时语音识别引擎配置。
//
// 协议为 WebSocket（api-ws/v1/inference），与官方 Go/Python SDK 的
// Recognition 模式一致：连接后发送 run-task JSON 指令，随后以二进制帧
// 流式发送音频（100ms 分片），服务端以 result-generated 事件返回
// 中间/最终结果，最后发 finish-task 结束。
//
// 配置键（speech.Config.config map）：
//
//	api_key       必填，DASHSCOPE_API_KEY（sk-…），Bearer 鉴权
//	model         默认 fun-asr-realtime；8k 模型（fun-asr-flash-8k-realtime 等）自动降采样到 8kHz
//	base_url      默认 wss://dashscope.aliyuncs.com/api-ws/v1/inference/
//	sample_rate   默认 16000；模型名含 "8k" 时自动 8000，可显式覆盖（仅支持 8000 / 16000）
//	language      可选 BCP-47；映射到 language_hints 首项（如 zh-CN → zh），空 = 自动检测

import (
	"fmt"
	"strings"
)

type Config struct {
	APIKey     string
	Model      string
	BaseURL    string
	SampleRate int
	Language   string
}

func ParseConfig(m map[string]string) (Config, error) {
	if m == nil {
		m = map[string]string{}
	}
	c := Config{
		APIKey:     strings.TrimSpace(m["api_key"]),
		Model:      strings.TrimSpace(m["model"]),
		BaseURL:    strings.TrimRight(strings.TrimSpace(m["base_url"]), "/"),
		SampleRate: parseIntDefault(m["sample_rate"], 0),
		Language:   strings.TrimSpace(m["language"]),
	}
	if c.Model == "" {
		c.Model = "fun-asr-realtime"
	}
	if c.BaseURL == "" {
		c.BaseURL = "wss://dashscope.aliyuncs.com/api-ws/v1/inference/"
	}
	if c.SampleRate == 0 {
		// 8k 模型仅支持 8000 Hz；其他模型支持任意采样率（录音器固定 16k）。
		if strings.Contains(c.Model, "8k") {
			c.SampleRate = 8000
		} else {
			c.SampleRate = 16000
		}
	}
	if c.APIKey == "" {
		return Config{}, fmt.Errorf("dashscope: 需要 api_key")
	}
	switch c.SampleRate {
	case 8000, 16000:
	default:
		return Config{}, fmt.Errorf("dashscope: sample_rate 仅支持 8000 或 16000")
	}
	return c, nil
}

func parseIntDefault(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// languageHint BCP-47 → 语言代码（zh-CN → zh）；空返回空。
func languageHint(lang string) string {
	lang = strings.TrimSpace(lang)
	if lang == "" {
		return ""
	}
	if i := strings.IndexAny(lang, "-_"); i > 0 {
		lang = lang[:i]
	}
	return strings.ToLower(lang)
}
