// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

// Package chataudio 提供 "OpenAI Chat Completions + input_audio" 形态的
// 通用转写客户端。该形态由部分云厂商（如小米 MiMo ASR）采用：音频以
// input_audio content（base64 data URL）随 chat/completions 请求发送，
// 识别文本从 choices[0].message.content 取回。
//
// 后续接入同形态厂商时复用本客户端即可：只需提供各自的 base_url、
// 认证头名与模型名，无需重写请求构造与响应解析。形态不同的厂商
// （如原生流式端点）仍走 speech.Registry 各自实现，不受本包影响。
package chataudio

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client 是 chat/completions + input_audio 形态的转写客户端。
// AuthHeader 是认证头名：MiMo 用 "api-key"，OpenAI 兼容端点通常为
// "Authorization"（值需自带 Bearer 前缀时由调用方拼好）。
type Client struct {
	BaseURL    string // 如 https://api.xiaomimimo.com/v1
	AuthHeader string
	APIKey     string
	Model      string
	HTTP       *http.Client
}

// Transcribe 发送 WAV 音频并返回识别文本。lang 非空时写入 asr_options.language。
func (c *Client) Transcribe(ctx context.Context, wav []byte, lang string) (string, error) {
	if len(wav) == 0 {
		return "", fmt.Errorf("chataudio: 空音频")
	}
	req := chatRequest{
		Model: c.Model,
		Messages: []chatMessage{{
			Role: "user",
			Content: []chatContent{{
				Type: "input_audio",
				InputAudio: &inputAudio{
					Data: "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(wav),
				},
			}},
		}},
		Stream: false,
	}
	if lang != "" {
		req.ASROpts = &asrOptions{Language: lang}
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("chataudio: 构造请求: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("chataudio: 构造 HTTP 请求: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.AuthHeader != "" && c.APIKey != "" {
		httpReq.Header.Set(c.AuthHeader, c.APIKey)
	}

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("chataudio: 请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", fmt.Errorf("chataudio: 读取响应: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return "", fmt.Errorf("chataudio: HTTP %d: %s", resp.StatusCode, msg)
	}

	var cr chatResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		return "", fmt.Errorf("chataudio: 解析响应: %w", err)
	}
	if cr.Error != nil && cr.Error.Message != "" {
		return "", fmt.Errorf("chataudio: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("chataudio: 响应无 choices")
	}
	return cr.Choices[0].Message.Content, nil
}

// chatRequest 请求体（OpenAI Chat Completions 音频输入子集）。
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	ASROpts  *asrOptions   `json:"asr_options,omitempty"`
}

type chatMessage struct {
	Role    string        `json:"role"`
	Content []chatContent `json:"content"`
}

type chatContent struct {
	Type       string      `json:"type"`
	InputAudio *inputAudio `json:"input_audio,omitempty"`
}

type inputAudio struct {
	Data string `json:"data"`
}

type asrOptions struct {
	Language string `json:"language"`
}

// chatResponse 响应体（只取需要的字段，未知字段忽略）。
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *apiError `json:"error,omitempty"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}
