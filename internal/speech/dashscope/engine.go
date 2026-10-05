// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package dashscope

// 千问 AI 平台（DashScope）Fun-ASR 实时语音识别引擎（WebSocket 流式）。
//
// 协议对齐官方 WebSocket API（api-ws/v1/inference）：
//   - 鉴权：HTTP 头 Authorization: Bearer <api_key>
//   - 客户端事件（JSON 文本帧）：run-task 启动、finish-task 结束
//   - 音频：二进制帧流式发送（约 100ms 分片）
//   - 服务端事件：task-started / result-generated / task-finished / task-failed
//
// 仅 STT（流式 + 整段），不支持 TTS。录音场景由 StreamSession 提供
// 边录边出字；Transcribe 走同一 WebSocket 通道整段识别。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/c/just-talk-go/internal/speech"
)

func init() {
	speech.Register("dashscope", NewFromMap)
}

// Engine 实现 speech.Engine（仅 STT，流式）。
type Engine struct {
	cfg Config
}

// NewFromMap 工厂：map 配置 → Engine。
func NewFromMap(m map[string]string) (speech.Engine, error) {
	cfg, err := ParseConfig(m)
	if err != nil {
		return nil, err
	}
	return &Engine{cfg: cfg}, nil
}

func (e *Engine) Name() string { return "dashscope" }

func (e *Engine) Capabilities() speech.Capabilities {
	return speech.Capabilities{STT: true, StreamingSTT: true}
}

func (e *Engine) Available(context.Context) (bool, string) {
	if e.cfg.APIKey == "" {
		return false, "未配置 api_key"
	}
	return true, fmt.Sprintf("model=%s rate=%d", e.cfg.Model, e.cfg.SampleRate)
}

func (e *Engine) Speak(context.Context, speech.SpeakRequest) (*speech.SpeakResult, error) {
	return nil, speech.ErrUnsupported
}

func (e *Engine) ListVoices(context.Context) ([]speech.Voice, error) {
	return nil, speech.ErrUnsupported
}

// NewStream 实现 speech.StreamingEngine。
func (e *Engine) NewStream(ctx context.Context, params speech.StreamParams) (speech.StreamSession, error) {
	return NewStreamSession(e.cfg, params), nil
}

// Transcribe 整段识别：复用 WebSocket 流式通道，一次性送完全部音频。
func (e *Engine) Transcribe(ctx context.Context, req speech.TranscribeRequest) (*speech.TranscribeResult, error) {
	if len(req.Audio) == 0 {
		return nil, speech.ErrInvalidInput
	}
	sess := NewStreamSession(e.cfg, speech.StreamParams{Lang: req.Lang})
	if err := sess.Connect(ctx); err != nil {
		return nil, err
	}
	defer sess.Close()
	if err := sess.SendAudio(ctx, req.Audio, true); err != nil {
		return nil, fmt.Errorf("dashscope transcribe: %w", err)
	}
	select {
	case <-sess.Final():
	case <-sess.Done():
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	text := strings.TrimSpace(sess.LastText())
	if text == "" {
		return nil, fmt.Errorf("dashscope transcribe: 未识别到内容")
	}
	return &speech.TranscribeResult{Text: text, Lang: req.Lang, Engine: e.Name()}, nil
}

// ---- 协议 JSON ----

// clientEvent run-task / finish-task 指令（客户端事件）。
type clientEvent struct {
	Header  clientHeader  `json:"header"`
	Payload clientPayload `json:"payload"`
}

type clientHeader struct {
	Action    string `json:"action"`
	TaskID    string `json:"task_id"`
	Streaming string `json:"streaming"`
}

type clientPayload struct {
	TaskGroup  string         `json:"task_group"`
	Task       string         `json:"task"`
	Function   string         `json:"function"`
	Model      string         `json:"model"`
	Parameters map[string]any `json:"parameters"`
	Input      map[string]any `json:"input"`
}

// serverEvent 服务端事件。
type serverEvent struct {
	Header  serverHeader `json:"header"`
	Payload struct {
		Output struct {
			Sentence struct {
				Text        string `json:"text"`
				Heartbeat   bool   `json:"heartbeat"`
				SentenceEnd bool   `json:"sentence_end"`
			} `json:"sentence"`
		} `json:"output"`
		Usage *struct {
			Duration int `json:"duration"`
		} `json:"usage"`
	} `json:"payload"`
}

type serverHeader struct {
	Event        string `json:"event"`
	TaskID       string `json:"task_id"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

// encodeRunTask 构造 run-task 指令 JSON。
func encodeRunTask(cfg Config, taskID string, lang string) ([]byte, error) {
	params := map[string]any{
		"format":      "pcm",
		"sample_rate": cfg.SampleRate,
	}
	if hint := languageHint(lang); hint != "" {
		params["language_hints"] = []string{hint}
	}
	ev := clientEvent{
		Header: clientHeader{Action: "run-task", TaskID: taskID, Streaming: "duplex"},
		Payload: clientPayload{
			TaskGroup:  "audio",
			Task:       "asr",
			Function:   "recognition",
			Model:      cfg.Model,
			Parameters: params,
			Input:      map[string]any{},
		},
	}
	return json.Marshal(ev)
}

// encodeFinishTask 构造 finish-task 指令 JSON。
func encodeFinishTask(taskID string) ([]byte, error) {
	ev := clientEvent{
		Header:  clientHeader{Action: "finish-task", TaskID: taskID, Streaming: "duplex"},
		Payload: clientPayload{Input: map[string]any{}},
	}
	return json.Marshal(ev)
}
