// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package speech

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"time"
)

type stubEngine struct {
	text    string
	avail   bool
	err     error
	gotLang string
	gotCT   string
	gotWAV  []byte
}

func (s *stubEngine) Name() string               { return "stub" }
func (s *stubEngine) Capabilities() Capabilities { return Capabilities{STT: true} }
func (s *stubEngine) Available(context.Context) (bool, string) {
	return s.avail, "stub detail"
}
func (s *stubEngine) Speak(context.Context, SpeakRequest) (*SpeakResult, error) {
	return nil, ErrUnsupported
}
func (s *stubEngine) ListVoices(context.Context) ([]Voice, error) {
	return nil, ErrUnsupported
}
func (s *stubEngine) Transcribe(ctx context.Context, req TranscribeRequest) (*TranscribeResult, error) {
	s.gotLang = req.Lang
	s.gotCT = req.ContentType
	s.gotWAV = req.Audio
	if s.err != nil {
		return nil, s.err
	}
	return &TranscribeResult{Text: s.text, Engine: s.Name()}, nil
}

// blockingStub 阻塞直到 ctx 取消（模拟慢转写）。
type blockingStub struct{}

func (blockingStub) Name() string               { return "blocking" }
func (blockingStub) Capabilities() Capabilities { return Capabilities{STT: true} }
func (blockingStub) Available(context.Context) (bool, string) {
	return true, "ok"
}
func (blockingStub) Speak(context.Context, SpeakRequest) (*SpeakResult, error) {
	return nil, ErrUnsupported
}
func (blockingStub) ListVoices(context.Context) ([]Voice, error) {
	return nil, ErrUnsupported
}
func (blockingStub) Transcribe(ctx context.Context, _ TranscribeRequest) (*TranscribeResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestEncodeWAV(t *testing.T) {
	pcm := []byte{0x01, 0x02, 0x03, 0x04}
	wav := EncodeWAV(pcm)
	if len(wav) != 44+len(pcm) {
		t.Fatalf("len=%d", len(wav))
	}
	if !bytes.Equal(wav[:4], []byte("RIFF")) || !bytes.Equal(wav[8:12], []byte("WAVE")) {
		t.Fatal("bad RIFF header")
	}
	if binary.LittleEndian.Uint32(wav[40:44]) != uint32(len(pcm)) {
		t.Fatal("bad data size")
	}
	if binary.LittleEndian.Uint32(wav[24:28]) != PCMRate {
		t.Fatalf("sample rate = %d", binary.LittleEndian.Uint32(wav[24:28]))
	}
	if !bytes.Equal(wav[44:], pcm) {
		t.Fatal("pcm payload mismatch")
	}
}

func TestBatchSessionTranscribesOnLast(t *testing.T) {
	eng := &stubEngine{avail: true, text: "你好世界"}
	sess := NewBatchSession(eng, StreamParams{Lang: "zh-CN"})
	ctx := context.Background()
	if err := sess.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	if err := sess.SendAudio(ctx, []byte{1, 2, 3, 4}, false); err != nil {
		t.Fatal(err)
	}
	if err := sess.SendAudio(ctx, []byte{5, 6}, true); err != nil {
		t.Fatal(err)
	}

	select {
	case res := <-sess.Results():
		if !res.IsFinal || res.Text != "你好世界" {
			t.Fatalf("result = %+v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no result published")
	}

	select {
	case <-sess.Final():
	case <-time.After(2 * time.Second):
		t.Fatal("Final not closed")
	}
	select {
	case <-sess.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done not closed")
	}
	if sess.LastText() != "你好世界" {
		t.Fatalf("LastText = %q", sess.LastText())
	}
	if eng.gotLang != "zh-CN" {
		t.Fatalf("lang = %q", eng.gotLang)
	}
	if eng.gotCT != PCMContentType {
		t.Fatalf("content type = %q", eng.gotCT)
	}
	if len(eng.gotWAV) != 44+6 || !bytes.Equal(eng.gotWAV[:4], []byte("RIFF")) {
		t.Fatalf("engine did not receive wav payload (%d bytes)", len(eng.gotWAV))
	}
}

func TestBatchSessionConnectUnavailable(t *testing.T) {
	sess := NewBatchSession(&stubEngine{avail: false}, StreamParams{})
	if err := sess.Connect(context.Background()); err == nil {
		t.Fatal("expected connect error for unavailable engine")
	}
}

func TestBatchSessionEmptyAudio(t *testing.T) {
	eng := &stubEngine{avail: true, text: "x"}
	sess := NewBatchSession(eng, StreamParams{})
	ctx := context.Background()
	_ = sess.Connect(ctx)
	if err := sess.SendAudio(ctx, nil, true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sess.Final():
	case <-time.After(2 * time.Second):
		t.Fatal("Final not closed for empty audio")
	}
	select {
	case <-sess.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done not closed for empty audio")
	}
	select {
	case res, ok := <-sess.Results():
		if ok {
			t.Fatalf("unexpected result for empty audio: %+v", res)
		}
	default:
		t.Fatal("Results should be closed for empty audio")
	}
	if eng.gotWAV != nil {
		t.Fatal("engine should not be called for empty audio")
	}
}

func TestBatchSessionCloseCancelsTranscribe(t *testing.T) {
	sess := NewBatchSession(blockingStub{}, StreamParams{})
	ctx := context.Background()
	if err := sess.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	if err := sess.SendAudio(ctx, []byte{1, 2}, true); err != nil {
		t.Fatal(err)
	}
	// 给转写 goroutine 一点启动时间，然后关闭会话
	time.Sleep(20 * time.Millisecond)
	_ = sess.Close()
	select {
	case <-sess.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done not closed after Close")
	}
}

func TestEncodeWAVWithSampleRate(t *testing.T) {
	wav := EncodeWAVWithSampleRate([]byte{0, 0}, 8000)
	if got := binary.LittleEndian.Uint32(wav[24:28]); got != 8000 {
		t.Fatalf("sample rate = %d, want 8000", got)
	}
	if got := binary.LittleEndian.Uint32(wav[28:32]); got != 16000 {
		t.Fatalf("byte rate = %d, want 16000", got)
	}
}
