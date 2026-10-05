// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package doubao

// 豆包（火山引擎）语音识别引擎：仅 STT，不做 TTS / 闲时版。
//
// 覆盖：
//   - 流式大模型 ASR（WebSocket：stream / nostream / async），批处理整段识别
//   - 录音文件：极速版 flash（直返）/ 标准版 standard（submit+query，需公网 URL）
//
// Transcribe：默认走 nostream 流式输入模式（整段字节一次送完），本地录音无需公网 URL。
// file_mode=flash 且 audio 带公网 url 元数据时走 HTTP 极速版（见 TranscribeRequest 扩展约定）。
//
// 实时录音场景请使用 Stream（流式会话）：见 stream.go。

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

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/c/just-talk-go/internal/speech"
)

func init() {
	speech.Register("doubao-stream", NewFromMap)
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
	return "doubao-stream"
}

func (e *Engine) Capabilities() speech.Capabilities {
	return speech.Capabilities{
		STT:          true,
		StreamingSTT: true,
	}
}

func (e *Engine) Available(ctx context.Context) (bool, string) {
	if !e.cfg.hasAuth() {
		return false, "未配置 api_key 或 app_id+access_key"
	}
	detail := fmt.Sprintf("model=%s stream=%s file=%s resource_stream=%s",
		e.cfg.ModelVersion, e.cfg.StreamMode, e.cfg.FileMode, e.cfg.StreamResourceID())
	return true, detail
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
	// 标准文件模式需要 URL：若 ContentType 以 url: 前缀约定（内部），走 HTTP 文件 API
	if strings.HasPrefix(req.ContentType, "url:") {
		return e.transcribeFileURL(ctx, strings.TrimPrefix(req.ContentType, "url:"), req.Lang)
	}
	// 默认：WebSocket nostream/stream 整段识别（本地字节）
	return e.transcribeStream(ctx, req)
}

// ---- WebSocket 流式 / nostream ----

func (e *Engine) transcribeStream(ctx context.Context, req speech.TranscribeRequest) (*speech.TranscribeResult, error) {
	format := mapContentTypeToFormat(req.ContentType, req.Audio)
	lang := req.Lang
	if lang == "" {
		lang = e.cfg.Language
	}

	fullReq := map[string]any{
		"user": map[string]any{
			"uid": "just-talk-plus",
		},
		"audio": map[string]any{
			"format":  format,
			"rate":    speech.PCMRate,
			"bits":    speech.PCMBits,
			"channel": speech.PCMChannels,
			"codec":   codecForFormat(format),
		},
		"request": map[string]any{
			"model_name":  "bigmodel",
			"enable_itn":  e.cfg.EnableITN,
			"enable_punc": e.cfg.EnablePunc,
			"enable_ddc":  e.cfg.EnableDDC,
		},
	}
	if lang != "" {
		fullReq["audio"].(map[string]any)["language"] = lang
	}
	payload, err := json.Marshal(fullReq)
	if err != nil {
		return nil, err
	}

	header := http.Header{}
	setAuthHeaders(header, e.cfg, e.cfg.StreamResourceID(), uuid.NewString())
	header.Set("X-Api-Connect-Id", uuid.NewString())

	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, e.cfg.streamURL(), &websocket.DialOptions{
		HTTPHeader: header,
	})
	if err != nil {
		return nil, fmt.Errorf("doubao ws dial: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// 读超时：整段识别给足时间
	conn.SetReadLimit(8 << 20)

	frame, err := EncodeFullClientRequest(payload, true)
	if err != nil {
		return nil, err
	}
	if err := conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
		return nil, fmt.Errorf("doubao full client request: %w", err)
	}
	// 读 full client 的响应（可忽略内容，但要排空）
	if _, err := e.readServerJSON(ctx, conn); err != nil {
		return nil, fmt.Errorf("doubao full client response: %w", err)
	}

	// 分片发送音频（约 100ms@16k mono s16le ≈ 3200B；容器格式整包也切块）
	const chunk = 8 << 10
	audio := req.Audio
	for i := 0; i < len(audio); i += chunk {
		end := i + chunk
		last := end >= len(audio)
		if last {
			end = len(audio)
		}
		af, err := EncodeAudioOnly(audio[i:end], true, last)
		if err != nil {
			return nil, err
		}
		if err := conn.Write(ctx, websocket.MessageBinary, af); err != nil {
			return nil, fmt.Errorf("doubao audio chunk: %w", err)
		}
		// 每包读响应；最后一包取最终文本
		text, isLast, err := e.readServerText(ctx, conn)
		if err != nil {
			return nil, err
		}
		if last || isLast {
			return &speech.TranscribeResult{
				Text:   text,
				Lang:   lang,
				Engine: e.Name(),
			}, nil
		}
		// 非最后包继续；text 可能是中间结果
		if text != "" && last {
			return &speech.TranscribeResult{Text: text, Lang: lang, Engine: e.Name()}, nil
		}
	}
	// 若循环未返回，再读一帧
	text, _, err := e.readServerText(ctx, conn)
	if err != nil {
		return nil, err
	}
	return &speech.TranscribeResult{Text: text, Lang: lang, Engine: e.Name()}, nil
}

func (e *Engine) readServerJSON(ctx context.Context, conn *websocket.Conn) (map[string]any, error) {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	sf, err := DecodeServerFrame(data)
	if err != nil {
		return nil, err
	}
	if sf.MsgType == msgErrorServer {
		return nil, fmt.Errorf("doubao server error %d: %s", sf.ErrorCode, sf.ErrorMessage)
	}
	if len(sf.Payload) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(sf.Payload, &m); err != nil {
		return nil, fmt.Errorf("doubao json: %w", err)
	}
	return m, nil
}

func (e *Engine) readServerText(ctx context.Context, conn *websocket.Conn) (text string, last bool, err error) {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return "", false, err
	}
	sf, err := DecodeServerFrame(data)
	if err != nil {
		return "", false, err
	}
	if sf.MsgType == msgErrorServer {
		return "", false, fmt.Errorf("doubao server error %d: %s", sf.ErrorCode, sf.ErrorMessage)
	}
	last = sf.Flags == flagLastNoSeq || sf.Flags == flagLastNegSeq
	if len(sf.Payload) == 0 {
		return "", last, nil
	}
	text = extractASRText(sf.Payload)
	return text, last, nil
}

// extractASRText 从 full server response JSON 抽最终/累积文本。
func extractASRText(payload []byte) string {
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		return ""
	}
	// result 可能是 object 或 list
	if r, ok := root["result"]; ok {
		switch v := r.(type) {
		case map[string]any:
			if t, ok := v["text"].(string); ok {
				return t
			}
		case []any:
			var b strings.Builder
			for _, item := range v {
				if m, ok := item.(map[string]any); ok {
					if t, ok := m["text"].(string); ok {
						b.WriteString(t)
					}
				}
			}
			return b.String()
		}
	}
	if t, ok := root["text"].(string); ok {
		return t
	}
	return ""
}

// ---- 文件识别：flash / standard ----

func (e *Engine) transcribeFileURL(ctx context.Context, audioURL, lang string) (*speech.TranscribeResult, error) {
	audioURL = strings.TrimSpace(audioURL)
	if audioURL == "" {
		return nil, speech.ErrInvalidInput
	}
	if lang == "" {
		lang = e.cfg.Language
	}
	if e.cfg.FileMode == "flash" {
		return e.flashRecognize(ctx, audioURL, lang)
	}
	return e.standardRecognize(ctx, audioURL, lang)
}

func (e *Engine) flashRecognize(ctx context.Context, audioURL, lang string) (*speech.TranscribeResult, error) {
	body := map[string]any{
		"audio": map[string]any{
			"url":    audioURL,
			"format": guessFormatFromURL(audioURL),
		},
		"request": map[string]any{
			"model_name":  "bigmodel",
			"enable_itn":  e.cfg.EnableITN,
			"enable_punc": e.cfg.EnablePunc,
			"enable_ddc":  e.cfg.EnableDDC,
		},
	}
	if lang != "" {
		body["audio"].(map[string]any)["language"] = lang
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.flashURL(), bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	setAuthHeaders(req.Header, e.cfg, e.cfg.FileResourceID(), uuid.NewString())

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("doubao flash: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("doubao flash HTTP %s: %s", resp.Status, truncate(string(data), 300))
	}
	// 状态也可能在响应头
	if sc := resp.Header.Get("X-Api-Status-Code"); sc != "" && sc != "20000000" && sc != "0" {
		msg := resp.Header.Get("X-Api-Message")
		return nil, fmt.Errorf("doubao flash status %s: %s", sc, msg)
	}
	text := extractASRText(data)
	if text == "" {
		// 有的响应包一层 data
		var wrap map[string]any
		_ = json.Unmarshal(data, &wrap)
		if t := extractASRText(data); t != "" {
			text = t
		}
		_ = wrap
	}
	return &speech.TranscribeResult{Text: text, Lang: lang, Engine: e.Name()}, nil
}

func (e *Engine) standardRecognize(ctx context.Context, audioURL, lang string) (*speech.TranscribeResult, error) {
	taskID := uuid.NewString()
	body := map[string]any{
		"audio": map[string]any{
			"url":    audioURL,
			"format": guessFormatFromURL(audioURL),
		},
		"request": map[string]any{
			"model_name":  "bigmodel",
			"enable_itn":  e.cfg.EnableITN,
			"enable_punc": e.cfg.EnablePunc,
			"enable_ddc":  e.cfg.EnableDDC,
		},
	}
	if lang != "" {
		body["audio"].(map[string]any)["language"] = lang
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.submitURL(), bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	setAuthHeaders(req.Header, e.cfg, e.cfg.FileResourceID(), taskID)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("doubao submit: %w", err)
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("doubao submit HTTP %s: %s", resp.Status, truncate(string(data), 300))
	}

	// poll query
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1500 * time.Millisecond):
		}
		qreq, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.queryURL(), bytes.NewReader([]byte("{}")))
		if err != nil {
			return nil, err
		}
		qreq.Header.Set("Content-Type", "application/json")
		setAuthHeaders(qreq.Header, e.cfg, e.cfg.FileResourceID(), taskID)
		qresp, err := e.client.Do(qreq)
		if err != nil {
			return nil, fmt.Errorf("doubao query: %w", err)
		}
		qdata, _ := io.ReadAll(io.LimitReader(qresp.Body, 4<<20))
		qresp.Body.Close()
		sc := qresp.Header.Get("X-Api-Status-Code")
		// 20000000 成功；处理中码以文档为准，常见非终态继续
		if sc == "20000000" || sc == "" {
			text := extractASRText(qdata)
			if text != "" || sc == "20000000" {
				return &speech.TranscribeResult{Text: text, Lang: lang, Engine: e.Name()}, nil
			}
		}
		// 明确失败
		if sc != "" && sc != "20000000" && !strings.HasPrefix(sc, "2000") {
			// 20000001/02 等处理中 — 宽松：非 2 开头当失败
			if !strings.HasPrefix(sc, "2") {
				return nil, fmt.Errorf("doubao query status %s: %s", sc, qresp.Header.Get("X-Api-Message"))
			}
		}
	}
	return nil, fmt.Errorf("doubao standard: 查询超时")
}

// ---- helpers ----

func mapContentTypeToFormat(ct string, audio []byte) string {
	ct = strings.ToLower(ct)
	switch {
	case strings.Contains(ct, "wav"):
		return "wav"
	case strings.Contains(ct, "mpeg"), strings.Contains(ct, "mp3"):
		return "mp3"
	case strings.Contains(ct, "ogg"):
		return "ogg"
	case strings.Contains(ct, "webm"):
		// 豆包 format 列表无 webm；ogg/opus 接近。尝试 ogg，失败由服务端报错
		return "ogg"
	case strings.Contains(ct, "mp4"), strings.Contains(ct, "m4a"), strings.Contains(ct, "aac"):
		return "m4a"
	case strings.Contains(ct, "pcm"), strings.Contains(ct, "raw"):
		return "pcm"
	}
	// sniff
	if len(audio) >= 12 && string(audio[:4]) == "RIFF" {
		return "wav"
	}
	if len(audio) >= 4 && string(audio[:4]) == "OggS" {
		return "ogg"
	}
	if len(audio) >= 3 && string(audio[:3]) == "ID3" {
		return "mp3"
	}
	return "wav"
}

func codecForFormat(format string) string {
	if format == "ogg" {
		return "opus"
	}
	return "raw"
}

func guessFormatFromURL(u string) string {
	u = strings.ToLower(u)
	for _, ext := range []string{"wav", "mp3", "ogg", "m4a", "aac", "pcm", "amr"} {
		if strings.Contains(u, "."+ext) {
			return ext
		}
	}
	return "mp3"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// EncodeAudioBase64 供测试/调试。
func EncodeAudioBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
