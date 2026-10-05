// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package dashscope

// DashScope Fun-ASR 流式识别会话：实现 speech.StreamSession。
//
// 会话流程：
//  1. Connect：建立 WebSocket，发送 run-task 指令，等待 task-started 事件
//  2. SendAudio：以二进制帧流式发送 PCM（8k 模型自动降采样到 8000 Hz）
//  3. 服务端 result-generated 事件：sentence_end=false 为中间结果，true 为最终结果
//  4. SendAudio(isLast=true) 后自动发送 finish-task；服务端回 task-finished 结束
//
// 音频要求：16 kHz 16-bit 单声道 PCM（s16le），与录音器输出一致；
// 8k 模型（fun-asr-flash-8k-realtime 等）仅支持 8000 Hz，会话内做 2:1 降采样。

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/c/just-talk-go/internal/speech"
)

const (
	// taskStartedTimeout 等待 task-started 事件的最长时间。
	taskStartedTimeout = 15 * time.Second
	// chunkBytes 音频分片大小：约 100ms@16k s16le（16k×2B×0.1s = 3200B）。
	chunkBytes = 3200
)

// NewStream 实现 speech.StreamingEngine（在 engine.go 中，见 NewStreamSession）。
// NewStreamSession 创建会话（不连接；连接在 Connect 中进行）。
func NewStreamSession(cfg Config, params speech.StreamParams) *StreamSession {
	writeSlot := make(chan struct{}, 1)
	writeSlot <- struct{}{}
	return &StreamSession{
		cfg:       cfg,
		params:    params,
		logger:    slog.Default(),
		writeSlot: writeSlot,
		started:   make(chan struct{}),
		resultCh:  make(chan speech.StreamResult, 64),
		done:      make(chan struct{}),
		final:     make(chan struct{}),
	}
}

// StreamSession 一次流式识别会话。
type StreamSession struct {
	cfg    Config
	params speech.StreamParams
	logger *slog.Logger

	conn      *websocket.Conn
	taskID    string
	writeSlot chan struct{}
	started   chan struct{}
	startOnce sync.Once

	resultCh  chan speech.StreamResult
	done      chan struct{}
	final     chan struct{}
	finalOnce sync.Once

	textMu    sync.RWMutex
	doneText  []string // 已结束的句子
	pending   string   // 当前未结束句子（中间结果）
	lastText  string
	audioSent func([]byte)
}

// SetLogger 覆盖默认日志器（供插件注入带上下文的 logger）。
func (s *StreamSession) SetLogger(l *slog.Logger) {
	if l != nil {
		s.logger = l
	}
}

// SetAudioSentObserver 设置已成功写入上游连接的音频观察器。
func (s *StreamSession) SetAudioSentObserver(observer func([]byte)) { s.audioSent = observer }

func (s *StreamSession) AudioSentSampleRate() int { return s.cfg.SampleRate }

// Connect 建立连接并启动识别任务；task-started 前不会返回。
func (s *StreamSession) Connect(ctx context.Context) error {
	header := http.Header{}
	header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	s.logger.Info("connecting to DashScope ASR", "url", s.cfg.BaseURL, "model", s.cfg.Model)
	conn, resp, err := websocket.Dial(ctx, s.cfg.BaseURL, &websocket.DialOptions{
		HTTPHeader: header,
	})
	if err != nil {
		if resp != nil {
			return fmt.Errorf("dashscope websocket dial: HTTP %d %s: %w", resp.StatusCode, resp.Status, err)
		}
		return fmt.Errorf("dashscope websocket dial: %w", err)
	}
	conn.SetReadLimit(16 << 20)
	s.conn = conn

	taskID := uuid.NewString()
	s.taskID = taskID
	runTask, err := encodeRunTask(s.cfg, taskID, s.effectiveLang())
	if err != nil {
		conn.Close(websocket.StatusInternalError, "encode failed")
		return fmt.Errorf("dashscope run-task encode: %w", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, runTask); err != nil {
		conn.Close(websocket.StatusInternalError, "run-task failed")
		return fmt.Errorf("dashscope run-task: %w", err)
	}
	s.logger.Info("DashScope ASR run-task sent", "task_id", taskID)

	// 接收循环随连接启动；等待 task-started 后返回。
	go s.receiveLoop(ctx)
	select {
	case <-s.started:
		s.logger.Info("DashScope ASR task started")
		return nil
	case <-ctx.Done():
		conn.Close(websocket.StatusNormalClosure, "canceled")
		return ctx.Err()
	case <-time.After(taskStartedTimeout):
		conn.Close(websocket.StatusNormalClosure, "start timeout")
		return fmt.Errorf("dashscope: 等待 task-started 超时（%s）", taskStartedTimeout)
	}
}

// SendAudio 发送音频；isLast=true 时送完后自动发 finish-task。
func (s *StreamSession) SendAudio(ctx context.Context, pcm []byte, isLast bool) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.writeSlot:
	}
	defer func() { s.writeSlot <- struct{}{} }()
	if s.conn == nil {
		return fmt.Errorf("dashscope: ASR connection is not open")
	}
	data := pcm
	if s.cfg.SampleRate == 8000 {
		data = downsample16kTo8k(pcm)
	}
	for i := 0; i < len(data); i += chunkBytes {
		end := i + chunkBytes
		if end > len(data) {
			end = len(data)
		}
		if err := s.conn.Write(ctx, websocket.MessageBinary, data[i:end]); err != nil {
			return fmt.Errorf("dashscope send audio: %w", err)
		}
		if s.audioSent != nil {
			s.audioSent(data[i:end])
		}
	}
	if isLast {
		return s.sendFinishTask(ctx)
	}
	return nil
}

func (s *StreamSession) sendFinishTask(ctx context.Context) error {
	if s.conn == nil {
		return nil
	}
	taskID := s.taskID
	msg, err := encodeFinishTask(taskID)
	if err != nil {
		return err
	}
	s.logger.Debug("DashScope ASR finish-task")
	return s.conn.Write(ctx, websocket.MessageText, msg)
}

func (s *StreamSession) Results() <-chan speech.StreamResult { return s.resultCh }
func (s *StreamSession) Done() <-chan struct{}               { return s.done }
func (s *StreamSession) Final() <-chan struct{}              { return s.final }

// LastText 返回当前累积文本（已结束句子 + 当前中间结果）。
func (s *StreamSession) LastText() string {
	s.textMu.RLock()
	defer s.textMu.RUnlock()
	return s.lastText
}

// receiveLoop 消费服务端消息直到连接断开；调用方以 goroutine 运行。
func (s *StreamSession) receiveLoop(ctx context.Context) {
	defer close(s.resultCh)
	defer close(s.done)
	for {
		_, data, err := s.conn.Read(ctx)
		if err != nil {
			return
		}
		var ev serverEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			s.logger.Debug("bad DashScope event", "error", err)
			continue
		}
		switch ev.Header.Event {
		case "task-started":
			s.startOnce.Do(func() { close(s.started) })
		case "result-generated":
			s.handleResult(&ev)
		case "task-finished":
			s.logger.Info("DashScope ASR task finished")
			s.finalOnce.Do(func() { close(s.final) })
		case "task-failed":
			err := fmt.Errorf("dashscope %s: %s", ev.Header.ErrorCode, ev.Header.ErrorMessage)
			s.logger.Error("DashScope ASR task failed", "code", ev.Header.ErrorCode, "message", ev.Header.ErrorMessage)
			select {
			case s.resultCh <- speech.StreamResult{Error: err}:
			default:
			}
			s.finalOnce.Do(func() { close(s.final) })
		default:
			s.logger.Debug("unknown DashScope event", "event", ev.Header.Event)
		}
	}
}

// handleResult 处理 result-generated：累积句子文本并输出结果。
func (s *StreamSession) handleResult(ev *serverEvent) {
	sentence := ev.Payload.Output.Sentence
	if sentence.Heartbeat {
		return // 心跳结果，跳过
	}
	text := strings.TrimSpace(sentence.Text)
	if text == "" && !sentence.SentenceEnd {
		return
	}

	s.textMu.Lock()
	if sentence.SentenceEnd {
		if text != "" {
			// 避免与 pending 重复：最终结果即该句完整文本
			if s.pending == text {
				s.pending = ""
			} else {
				s.doneText = append(s.doneText, text)
				s.pending = ""
			}
		} else {
			s.pending = ""
		}
	} else {
		s.pending = text
	}
	full := strings.Join(s.doneText, "") + s.pending
	s.lastText = full
	s.textMu.Unlock()

	if full != "" {
		select {
		case s.resultCh <- speech.StreamResult{Text: full, IsFinal: sentence.SentenceEnd}:
		default:
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

// effectiveLang 参数语言优先，其次配置语言。
func (s *StreamSession) effectiveLang() string {
	if s.params.Lang != "" {
		return s.params.Lang
	}
	return s.cfg.Language
}

// downsample16kTo8k 把 16 kHz s16le 单声道 PCM 降采样到 8 kHz（2:1 平均）。
// 输入长度须为偶数（奇数尾部样本丢弃）。
func downsample16kTo8k(pcm []byte) []byte {
	n := len(pcm) / 4 // 每 2 个样本一组（每组 4 字节）
	out := make([]byte, n*2)
	for i := 0; i < n; i++ {
		a := int16(binary.LittleEndian.Uint16(pcm[i*4:]))
		b := int16(binary.LittleEndian.Uint16(pcm[i*4+2:]))
		v := int16((int32(a) + int32(b)) / 2)
		binary.LittleEndian.PutUint16(out[i*2:], uint16(v))
	}
	return out
}
