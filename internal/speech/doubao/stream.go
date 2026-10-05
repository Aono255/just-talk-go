// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package doubao

// 豆包流式识别会话：实现 speech.StreamingEngine / speech.StreamSession。
//
// 由 just-talk 旧版 ASRClient 演化而来：默认走 bigmodel_async（大模型流式 ASR 异步接口），
// 录音期间边录边发 PCM，服务端返回带 definite 标记的 utterances 与最后一包（flags 0x02/0x03）。
// 帧编码与旧版逐字节兼容（不压缩），鉴权统一走 setAuthHeaders（新/旧两套凭据都支持）。

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/c/just-talk-go/internal/speech"
)

// NewStream 实现 speech.StreamingEngine。
func (e *Engine) NewStream(ctx context.Context, params speech.StreamParams) (speech.StreamSession, error) {
	return NewStreamSession(e.cfg, params), nil
}

// StreamSession 一次流式识别会话。
type StreamSession struct {
	cfg       Config
	params    speech.StreamParams
	logger    *slog.Logger
	conn      *websocket.Conn
	logID     string
	writeSlot chan struct{}
	resultCh  chan speech.StreamResult
	done      chan struct{}
	final     chan struct{}
	finalOnce sync.Once
	errorOnce sync.Once
	failed    atomic.Bool
	textMu    sync.RWMutex
	lastText  string
	audioSent func([]byte)
}

// NewStreamSession 创建会话（不连接；连接在 Connect 中进行）。
func NewStreamSession(cfg Config, params speech.StreamParams) *StreamSession {
	writeSlot := make(chan struct{}, 1)
	writeSlot <- struct{}{}
	return &StreamSession{
		cfg:       cfg,
		params:    params,
		logger:    slog.Default(),
		writeSlot: writeSlot,
		resultCh:  make(chan speech.StreamResult, 64),
		done:      make(chan struct{}),
		final:     make(chan struct{}),
	}
}

// SetLogger 覆盖默认日志器（供插件注入带上下文的 logger）。
func (s *StreamSession) SetLogger(l *slog.Logger) {
	if l != nil {
		s.logger = l
	}
}

// SetAudioSentObserver 设置已成功写入上游连接的音频观察器。
func (s *StreamSession) SetAudioSentObserver(observer func([]byte)) { s.audioSent = observer }

func (s *StreamSession) AudioSentSampleRate() int { return speech.PCMRate }

func (s *StreamSession) Connect(ctx context.Context) error {
	url := s.cfg.streamURL()
	s.logger.Info("connecting to ASR", "url", url)
	header := http.Header{}
	setAuthHeaders(header, s.cfg, s.cfg.StreamResourceID(), uuid.NewString())
	header.Set("X-Api-Connect-Id", uuid.NewString())
	conn, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: header,
	})
	if err != nil {
		if resp != nil {
			return fmt.Errorf("websocket dial: HTTP %d %s: %w", resp.StatusCode, resp.Status, err)
		}
		return fmt.Errorf("websocket dial: %w", err)
	}
	s.conn = conn
	s.logID = logIDFromResponse(resp)
	if err := s.sendFullClientRequest(ctx); err != nil {
		conn.Close(websocket.StatusInternalError, "init failed")
		return fmt.Errorf("send init: %w", err)
	}
	s.logger.Info("ASR connected", "upstream_log_id", s.logID)
	// 接收循环随连接启动，解析结果/关闭 final 与 done。
	go s.ReceiveLoop(ctx)
	return nil
}

func (s *StreamSession) SendAudio(ctx context.Context, pcm []byte, isLast bool) error {
	if s.failed.Load() {
		return fmt.Errorf("ASR session has failed")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.writeSlot:
	}
	defer func() { s.writeSlot <- struct{}{} }()
	if s.conn == nil {
		return fmt.Errorf("ASR connection is not open")
	}
	frame, err := EncodeAudioOnly(pcm, false, isLast)
	if err != nil {
		return err
	}
	if err := s.conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
		return err
	}
	if len(pcm) > 0 && s.audioSent != nil {
		s.audioSent(pcm)
	}
	return nil
}

func (s *StreamSession) Results() <-chan speech.StreamResult { return s.resultCh }
func (s *StreamSession) Done() <-chan struct{}               { return s.done }
func (s *StreamSession) Final() <-chan struct{}              { return s.final }

func (s *StreamSession) LastText() string {
	s.textMu.RLock()
	defer s.textMu.RUnlock()
	return s.lastText
}

// UpstreamLogID 返回豆包握手响应中的 X-Tt-Logid，供历史记录和排障使用。
func (s *StreamSession) UpstreamLogID() string { return s.logID }

func logIDFromResponse(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	return resp.Header.Get("X-Tt-Logid")
}

// ReceiveLoop 消费服务端消息直到连接断开或错误帧；调用方以 goroutine 运行。
func (s *StreamSession) ReceiveLoop(ctx context.Context) {
	defer close(s.resultCh)
	defer close(s.done)
	for {
		_, data, err := s.conn.Read(ctx)
		if err != nil {
			return
		}
		if !s.parseResponse(data) {
			return
		}
	}
}

// Close 关闭连接。
func (s *StreamSession) Close() error {
	if s.conn != nil {
		return s.conn.Close(websocket.StatusNormalClosure, "done")
	}
	return nil
}

// buildBigmodelRequest 构造 bigmodel 识别请求体（与整段 WS 识别共用的模型参数）。
func buildBigmodelRequest(cfg Config) map[string]any {
	req := map[string]any{
		"model_name":       "bigmodel",
		"enable_itn":       cfg.EnableITN,
		"enable_punc":      cfg.EnablePunc,
		"enable_ddc":       cfg.EnableDDC,
		"enable_lid":       cfg.EnableLID,
		"enable_word":      false,
		"enable_nonstream": cfg.EnableNonstream && cfg.StreamMode == "async", // 二遍识别仅双向流（async）生效
		"result_type":      "full",
		"show_utterances":  true,
	}
	// 语境词表：需在控制台自学习平台建表后才有 name/id，空值不发。
	setIf := func(k, v string) {
		if v != "" {
			req[k] = v
		}
	}
	setIf("boosting_table_name", cfg.BoostingTableName)
	setIf("boosting_table_id", cfg.BoostingTableID)
	setIf("correct_table_name", cfg.CorrectTableName)
	setIf("correct_table_id", cfg.CorrectTableID)
	setIf("regex_correct_table_name", cfg.RegexCorrectTableName)
	setIf("regex_correct_table_id", cfg.RegexCorrectTableID)
	return req
}

func (s *StreamSession) sendFullClientRequest(ctx context.Context) error {
	request := buildBigmodelRequest(s.cfg)
	if len(s.params.Hotwords) > 0 {
		if contextJSON, err := hotwordsContext(s.params.Hotwords); err == nil && contextJSON != "" {
			request["corpus"] = map[string]any{"context": contextJSON}
		}
	}
	audio := map[string]any{
		"format": "pcm", "rate": speech.PCMRate, "bits": speech.PCMBits,
		"channel": speech.PCMChannels, "codec": "raw",
	}
	if lang := s.effectiveLang(); lang != "" {
		audio["language"] = lang
	}
	payload := map[string]any{
		"user":    map[string]string{"uid": "just-talk-plus"},
		"audio":   audio,
		"request": request,
	}
	jsonBytes, _ := json.Marshal(payload)
	frame, err := EncodeFullClientRequest(jsonBytes, false)
	if err != nil {
		return err
	}
	return s.conn.Write(ctx, websocket.MessageBinary, frame)
}

func (s *StreamSession) effectiveLang() string {
	if s.params.Lang != "" {
		return s.params.Lang
	}
	return s.cfg.Language
}

// parseResponse 解析一帧服务端二进制消息；返回 false 表示会话已失败，
// 接收循环应立即结束，避免远端结束后继续刷同一错误。
func (s *StreamSession) parseResponse(data []byte) bool {
	sf, err := DecodeServerFrame(data)
	if err != nil {
		s.logger.Debug("bad ASR frame", "error", err)
		return true
	}
	switch sf.MsgType {
	case msgErrorServer:
		s.failed.Store(true)
		err := fmt.Errorf("ASR 服务错误 %d: %s", sf.ErrorCode, sf.ErrorMessage)
		s.errorOnce.Do(func() {
			s.logger.Error("ASR server error", "code", sf.ErrorCode, "message", sf.ErrorMessage)
			select {
			case s.resultCh <- speech.StreamResult{Error: err}:
			default:
			}
			s.finalOnce.Do(func() { close(s.final) })
		})
		return false
	case msgFullServer:
		s.handleFullServerFrame(sf)
	}
	return true
}

func (s *StreamSession) handleFullServerFrame(sf *ServerFrame) {
	var resp struct {
		Result struct {
			Text       string `json:"text"`
			Utterances []struct {
				Text     string `json:"text"`
				Definite bool   `json:"definite"`
			} `json:"utterances"`
		} `json:"result"`
	}
	if json.Unmarshal(sf.Payload, &resp) != nil {
		return
	}
	text := resp.Result.Text
	isLastPacket := sf.Flags == flagLastNoSeq || sf.Flags == flagLastNegSeq
	isFinalResult := isLastPacket
	for _, u := range resp.Result.Utterances {
		if u.Definite {
			isFinalResult = true
		}
	}
	if text != "" {
		s.textMu.Lock()
		s.lastText = text
		s.textMu.Unlock()
		select {
		case s.resultCh <- speech.StreamResult{Text: text, IsFinal: isFinalResult}:
		default:
		}
	}
	if isLastPacket {
		s.finalOnce.Do(func() { close(s.final) })
	}
}

// hotwordsContext 把热词列表编码为 corpus context JSON。
func hotwordsContext(words []string) (string, error) {
	hotwords := make([]map[string]string, 0, len(words))
	seen := make(map[string]struct{}, len(words))
	for _, word := range words {
		word = strings.TrimSpace(word)
		if word == "" {
			continue
		}
		if _, ok := seen[word]; ok {
			continue
		}
		seen[word] = struct{}{}
		hotwords = append(hotwords, map[string]string{"word": word})
	}
	if len(hotwords) == 0 {
		return "", nil
	}
	b, err := json.Marshal(map[string]interface{}{"hotwords": hotwords})
	if err != nil {
		return "", err
	}
	return string(b), nil
}
