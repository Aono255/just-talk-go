// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package speech

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
)

// 录音器输出固定为 16 kHz 16-bit 单声道 PCM（s16le）。
const (
	PCMRate        = 16000
	PCMBits        = 16
	PCMChannels    = 1
	PCMBlockAlign  = PCMChannels * PCMBits / 8
	PCMByteRate    = PCMRate * PCMBlockAlign
	PCMContentType = "audio/wav"
)

// BatchSession 把只支持批处理 Transcribe 的引擎包装成 StreamSession：
// 录音期间缓冲 PCM，最后一包触发 Transcribe，完成后输出一条最终结果。
// 供 auralwise / openai 等非流式引擎使用。
type BatchSession struct {
	engine Engine
	params StreamParams

	mu      sync.Mutex
	buf     []byte
	started bool
	ctx     context.Context
	cancel  context.CancelFunc

	resultCh  chan StreamResult
	done      chan struct{}
	final     chan struct{}
	finalOnce sync.Once

	textMu    sync.RWMutex
	lastText  string
	audioSent func([]byte)
}

// SetAudioSentObserver 设置已被批处理会话接受、将作为转写请求音频的数据观察器。
func (s *BatchSession) SetAudioSentObserver(observer func([]byte)) { s.audioSent = observer }

func (s *BatchSession) AudioSentSampleRate() int { return PCMRate }

// NewBatchSession 包装一个批处理引擎。
func NewBatchSession(engine Engine, params StreamParams) *BatchSession {
	ctx, cancel := context.WithCancel(context.Background())
	return &BatchSession{
		engine:   engine,
		params:   params,
		ctx:      ctx,
		cancel:   cancel,
		resultCh: make(chan StreamResult, 4),
		done:     make(chan struct{}),
		final:    make(chan struct{}),
	}
}

// Connect 校验引擎可用性；不可用返回可读错误。
func (s *BatchSession) Connect(ctx context.Context) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	ok, detail := s.engine.Available(ctx)
	if !ok {
		return errors.New(detail)
	}
	return nil
}

// SendAudio 缓冲音频；isLast=true 时触发批处理转写。
func (s *BatchSession) SendAudio(ctx context.Context, pcm []byte, isLast bool) error {
	if err := ctx.Err(); err != nil {
		return ctx.Err()
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}
	if len(pcm) > 0 {
		s.buf = append(s.buf, pcm...)
		if s.audioSent != nil {
			s.audioSent(pcm)
		}
	}
	if !isLast {
		s.mu.Unlock()
		return nil
	}
	s.started = true
	audio := s.buf
	s.buf = nil
	sessCtx := s.ctx
	s.mu.Unlock()

	go s.transcribe(sessCtx, audio)
	return nil
}

func (s *BatchSession) transcribe(ctx context.Context, audio []byte) {
	defer close(s.done)
	defer close(s.resultCh)
	s.finish(ctx, audio)
}

func (s *BatchSession) finish(ctx context.Context, audio []byte) {
	// 空录音直接结束，不调用引擎。
	if len(audio) == 0 {
		s.finalOnce.Do(func() { close(s.final) })
		return
	}
	req := TranscribeRequest{
		Audio:       EncodeWAV(audio),
		ContentType: PCMContentType,
		Lang:        s.params.Lang,
	}
	result, err := s.engine.Transcribe(ctx, req)
	if err != nil {
		select {
		case s.resultCh <- StreamResult{Error: err}:
		case <-ctx.Done():
		}
		s.finalOnce.Do(func() { close(s.final) })
		return
	}
	text := result.Text
	if text != "" {
		s.textMu.Lock()
		s.lastText = text
		s.textMu.Unlock()
	}
	select {
	case s.resultCh <- StreamResult{Text: text, IsFinal: true}:
	case <-ctx.Done():
	}
	s.finalOnce.Do(func() { close(s.final) })
}

// Results 返回结果流（批处理引擎仅一条最终结果）。
func (s *BatchSession) Results() <-chan StreamResult { return s.resultCh }

func (s *BatchSession) Done() <-chan struct{}  { return s.done }
func (s *BatchSession) Final() <-chan struct{} { return s.final }

func (s *BatchSession) LastText() string {
	s.textMu.RLock()
	defer s.textMu.RUnlock()
	return s.lastText
}

// Close 取消进行中的转写。
func (s *BatchSession) Close() error {
	s.cancel()
	return nil
}

// EncodeWAV 把 16 kHz 16-bit 单声道 PCM 打包成 WAV（RIFF/WAVE）。
func EncodeWAV(pcm []byte) []byte { return EncodeWAVWithSampleRate(pcm, PCMRate) }

// EncodeWAVWithSampleRate 把单声道 16-bit PCM 打包成给定采样率的 WAV。
func EncodeWAVWithSampleRate(pcm []byte, sampleRate int) []byte {
	if sampleRate <= 0 {
		sampleRate = PCMRate
	}
	dataLen := len(pcm)
	out := make([]byte, 44+dataLen)
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(36+dataLen))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16)                               // fmt chunk size
	binary.LittleEndian.PutUint16(out[20:22], 1)                                // PCM
	binary.LittleEndian.PutUint16(out[22:24], PCMChannels)                      // channels
	binary.LittleEndian.PutUint32(out[24:28], uint32(sampleRate))               // sample rate
	binary.LittleEndian.PutUint32(out[28:32], uint32(sampleRate*PCMBlockAlign)) // byte rate
	binary.LittleEndian.PutUint16(out[32:34], PCMBlockAlign)
	binary.LittleEndian.PutUint16(out[34:36], PCMBits)
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(dataLen))
	copy(out[44:], pcm)
	return out
}
