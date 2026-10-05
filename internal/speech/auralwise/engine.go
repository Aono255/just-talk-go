// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package auralwise

// AuralWise STT：异步任务 API（提交 → 轮询 → 取结果）。
// 文档：https://auralwise.cn/api-docs  BASE https://api.auralwise.cn/v1
// 仅识别；默认关说话人分离与声音事件，适合对话麦克风整段转写。
//
// 仅支持批处理（speech.Engine），录音场景由 BatchSession 包装成流式会话。

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/c/just-talk-go/internal/speech"
)

func init() {
	speech.Register("auralwise", NewFromMap)
}

// Engine 实现 speech.Engine（仅 STT）。
type Engine struct {
	cfg    Config
	client *http.Client
}

func NewFromMap(m map[string]string) (speech.Engine, error) {
	cfg, err := ParseConfig(m)
	if err != nil {
		return nil, err
	}
	return &Engine{
		cfg: cfg,
		client: &http.Client{
			// 单次 HTTP；整体超时由 ctx / poll_timeout 控制
			Timeout: 120 * time.Second,
		},
	}, nil
}

func (e *Engine) Name() string {
	return "auralwise"
}

func (e *Engine) Capabilities() speech.Capabilities {
	return speech.Capabilities{STT: true}
}

func (e *Engine) Available(context.Context) (bool, string) {
	if e.cfg.APIKey == "" {
		return false, "未配置 api_key"
	}
	return true, fmt.Sprintf("base=%s optimize=%s diarize=%v events=%v",
		e.cfg.BaseURL, e.cfg.Optimize, e.cfg.EnableDiarize, e.cfg.EnableEvents)
}

func (e *Engine) Speak(context.Context, speech.SpeakRequest) (*speech.SpeakResult, error) {
	return nil, speech.ErrUnsupported
}

func (e *Engine) ListVoices(context.Context) ([]speech.Voice, error) {
	return nil, speech.ErrUnsupported
}

func (e *Engine) Transcribe(ctx context.Context, req speech.TranscribeRequest) (*speech.TranscribeResult, error) {
	if len(req.Audio) == 0 && !strings.HasPrefix(req.ContentType, "url:") {
		return nil, speech.ErrInvalidInput
	}

	body := map[string]any{
		"options": e.buildOptions(req.Lang),
		// 对话场景默认不参与训练
		"allow_training": false,
	}

	if strings.HasPrefix(req.ContentType, "url:") {
		u := strings.TrimSpace(strings.TrimPrefix(req.ContentType, "url:"))
		if u == "" {
			u = strings.TrimSpace(string(req.Audio))
		}
		if u == "" {
			return nil, speech.ErrInvalidInput
		}
		body["audio_url"] = u
		if fn := filenameFromURL(u); fn != "" {
			body["audio_filename"] = fn
		}
	} else {
		fn := filenameFromContentType(req.ContentType)
		body["audio_base64"] = base64.StdEncoding.EncodeToString(req.Audio)
		body["audio_filename"] = fn
	}

	taskID, err := e.createTask(ctx, body)
	if err != nil {
		return nil, err
	}
	if err := e.waitDone(ctx, taskID); err != nil {
		return nil, err
	}
	text, lang, conf, err := e.fetchResult(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if lang == "" {
		lang = req.Lang
	}
	return &speech.TranscribeResult{
		Text:       text,
		Lang:       lang,
		Confidence: conf,
		Engine:     e.Name(),
	}, nil
}

func (e *Engine) buildOptions(lang string) map[string]any {
	opt := map[string]any{
		"enable_asr":          true,
		"enable_diarize":      e.cfg.EnableDiarize,
		"enable_audio_events": e.cfg.EnableEvents,
	}
	l := strings.TrimSpace(lang)
	if l == "" {
		l = e.cfg.Language
	}
	if l != "" {
		// 文档常用 ISO 639-1：zh / en；也接受 zh-CN → zh
		if i := strings.IndexByte(l, '-'); i > 0 {
			l = l[:i]
		}
		opt["asr_language"] = strings.ToLower(l)
	}
	switch e.cfg.Optimize {
	case "true":
		opt["optimize"] = true
	case "false":
		opt["optimize"] = false
		// 标准档才有词级时间戳；聊天场景仍只要正文
	}
	return opt
}

func (e *Engine) createTask(ctx context.Context, body map[string]any) (string, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.BaseURL+"/tasks", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	e.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("auralwise create: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("auralwise create HTTP %s: %s", resp.Status, truncate(string(data), 400))
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.ID == "" {
		return "", fmt.Errorf("auralwise create: 无 task id: %s", truncate(string(data), 300))
	}
	return out.ID, nil
}

func (e *Engine) waitDone(ctx context.Context, taskID string) error {
	deadline := time.Now().Add(e.cfg.PollTimeout)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		st, errMsg, err := e.getStatus(ctx, taskID)
		if err != nil {
			return err
		}
		switch st {
		case "done":
			return nil
		case "failed", "abandoned":
			if errMsg == "" {
				errMsg = st
			}
			return fmt.Errorf("auralwise task %s: %s", st, errMsg)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("auralwise: 等待超时（%s）task=%s status=%s", e.cfg.PollTimeout, taskID, st)
		}
		t := time.NewTimer(e.cfg.PollInterval)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

func (e *Engine) getStatus(ctx context.Context, taskID string) (status, errMsg string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.cfg.BaseURL+"/tasks/"+taskID, nil)
	if err != nil {
		return "", "", err
	}
	e.setHeaders(req)
	resp, err := e.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("auralwise status: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("auralwise status HTTP %s: %s", resp.Status, truncate(string(data), 300))
	}
	var out struct {
		Status       string `json:"status"`
		ErrorMessage string `json:"error_message"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", "", fmt.Errorf("auralwise status json: %w", err)
	}
	return out.Status, out.ErrorMessage, nil
}

func (e *Engine) fetchResult(ctx context.Context, taskID string) (text, lang string, conf float64, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.cfg.BaseURL+"/tasks/"+taskID+"/result", nil)
	if err != nil {
		return "", "", 0, err
	}
	e.setHeaders(req)
	resp, err := e.client.Do(req)
	if err != nil {
		return "", "", 0, fmt.Errorf("auralwise result: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", 0, fmt.Errorf("auralwise result HTTP %s: %s", resp.Status, truncate(string(data), 300))
	}
	return extractResult(data)
}

func (e *Engine) setHeaders(req *http.Request) {
	req.Header.Set("X-API-Key", e.cfg.APIKey)
	req.Header.Set("Accept", "application/json")
}

// extractResult 拼接 segments[].text；兼容顶层 text。
func extractResult(data []byte) (text, lang string, conf float64, err error) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return "", "", 0, fmt.Errorf("auralwise result json: %w", err)
	}
	if l, ok := root["language"].(string); ok {
		lang = l
	}
	if p, ok := root["language_probability"].(float64); ok {
		conf = p
	}
	if segs, ok := root["segments"].([]any); ok {
		var b strings.Builder
		for _, item := range segs {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			t, _ := m["text"].(string)
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			if b.Len() > 0 {
				// 中文段之间通常不需要空格；若上一段以拉丁字母结尾则补空格
				prev := b.String()
				if needsSpace(prev, t) {
					b.WriteByte(' ')
				}
			}
			b.WriteString(t)
		}
		text = strings.TrimSpace(b.String())
	}
	if text == "" {
		if t, ok := root["text"].(string); ok {
			text = strings.TrimSpace(t)
		}
	}
	return text, lang, conf, nil
}

func needsSpace(prev, next string) bool {
	if prev == "" || next == "" {
		return false
	}
	pr := []rune(prev)
	nr := []rune(next)
	last, first := pr[len(pr)-1], nr[0]
	return isLatin(last) && isLatin(first)
}

func isLatin(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

func filenameFromContentType(ct string) string {
	ct = strings.ToLower(ct)
	switch {
	case strings.Contains(ct, "wav"):
		return "recording.wav"
	case strings.Contains(ct, "mpeg"), strings.Contains(ct, "mp3"):
		return "recording.mp3"
	case strings.Contains(ct, "ogg"):
		return "recording.ogg"
	case strings.Contains(ct, "webm"):
		return "recording.webm"
	case strings.Contains(ct, "mp4"), strings.Contains(ct, "m4a"):
		return "recording.m4a"
	case strings.Contains(ct, "aac"):
		return "recording.aac"
	default:
		return "recording.webm"
	}
}

func filenameFromURL(u string) string {
	u = strings.Split(u, "?")[0]
	if i := strings.LastIndexByte(u, '/'); i >= 0 && i+1 < len(u) {
		return u[i+1:]
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
