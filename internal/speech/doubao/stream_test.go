// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package doubao

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/c/just-talk-go/internal/speech"
)

func newTestSession() *StreamSession {
	s := NewStreamSession(Config{EnableITN: true, EnablePunc: true}, speech.StreamParams{})
	s.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	return s
}

func TestParseResponseClosesFinalForEmptyText(t *testing.T) {
	client := newTestSession()
	client.parseResponse(testASRResponse(t, 0x03, "", false))

	select {
	case <-client.Final():
	default:
		t.Fatal("empty final response did not close Final")
	}
}

func TestParseResponseDoesNotCloseFinalForDefiniteUtterance(t *testing.T) {
	client := newTestSession()
	client.parseResponse(testASRResponse(t, 0x01, "partial", true))

	select {
	case <-client.Final():
		t.Fatal("definite utterance closed Final before the last packet")
	default:
	}

	select {
	case result := <-client.Results():
		if !result.IsFinal {
			t.Fatal("definite utterance was not marked final in result stream")
		}
		if result.Text != "partial" {
			t.Fatalf("text = %q", result.Text)
		}
	default:
		t.Fatal("definite utterance result was not published")
	}
}

func TestParseResponseTracksLastText(t *testing.T) {
	client := newTestSession()
	client.parseResponse(testASRResponse(t, 0x01, "你好", false))
	if got := client.LastText(); got != "你好" {
		t.Fatalf("LastText = %q", got)
	}
}

func TestLogIDFromResponse(t *testing.T) {
	resp := &http.Response{Header: http.Header{"X-Tt-Logid": []string{"trace-123"}}}
	if got := logIDFromResponse(resp); got != "trace-123" {
		t.Fatalf("logIDFromResponse = %q", got)
	}
	if got := logIDFromResponse(nil); got != "" {
		t.Fatalf("nil response log ID = %q", got)
	}
}

func TestParseResponseErrorFrameClosesFinal(t *testing.T) {
	client := newTestSession()
	frame := buildHeader(msgErrorServer, flagNone, serJSON, compNone)
	frame = append(frame, 0, 0, 0, 1) // error code
	frame = append(frame, 0, 0, 0, 3) // message length
	frame = append(frame, 'b', 'a', 'd')
	if client.parseResponse(frame) {
		t.Fatal("fatal error frame should terminate the session")
	}
	if client.parseResponse(frame) {
		t.Fatal("repeated fatal error frame should terminate the session")
	}

	select {
	case <-client.Final():
	default:
		t.Fatal("error frame did not close Final")
	}
	select {
	case result := <-client.Results():
		if result.Error == nil {
			t.Fatal("error frame should publish an error result")
		}
	default:
		t.Fatal("error frame result was not published")
	}
}

func TestSendAudioRejectsAfterFatalError(t *testing.T) {
	client := newTestSession()
	frame := buildHeader(msgErrorServer, flagNone, serJSON, compNone)
	frame = append(frame, 0, 0, 0, 1, 0, 0, 0, 3, 'b', 'a', 'd')
	if client.parseResponse(frame) {
		t.Fatal("fatal error frame should terminate the session")
	}
	if err := client.SendAudio(context.Background(), nil, false); err == nil {
		t.Fatal("SendAudio should reject a failed ASR session")
	}
}

func TestSendAudioHonorsContextWhileWaitingForWriter(t *testing.T) {
	client := newTestSession()
	<-client.writeSlot

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := client.SendAudio(ctx, nil, true)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SendAudio error = %v, want context deadline exceeded", err)
	}
}

func TestHotwordsContextDedupesAndSkipsEmpty(t *testing.T) {
	ctxJSON, err := hotwordsContext([]string{"", "Wayland", "Wayland", " wtype "})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ctxJSON, "Wayland") || !strings.Contains(ctxJSON, "wtype") {
		t.Fatalf("missing hotwords: %s", ctxJSON)
	}
	if strings.Count(ctxJSON, "Wayland") != 1 {
		t.Fatalf("duplicate hotword kept: %s", ctxJSON)
	}
}

func TestStreamRequestUsesConfigFlags(t *testing.T) {
	// 默认 async；enable_nonstream 仅在 async（双向流+二遍识别）开启
	async := NewStreamSession(Config{StreamMode: "async"}, speech.StreamParams{})
	if async.cfg.StreamMode != "async" {
		t.Fatal("default stream mode should be async")
	}
	nostream := NewStreamSession(Config{StreamMode: "nostream"}, speech.StreamParams{})
	if nostream.cfg.StreamMode != "nostream" {
		t.Fatal("nostream mode not honored")
	}
}

func TestBuildBigmodelRequest(t *testing.T) {
	req := buildBigmodelRequest(Config{
		StreamMode:        "async",
		EnableITN:         true,
		EnablePunc:        true,
		EnableLID:         true,
		EnableNonstream:   true,
		BoostingTableName: "tbl",
		CorrectTableID:    "cid",
	})
	if req["model_name"] != "bigmodel" {
		t.Fatalf("model_name = %v", req["model_name"])
	}
	if req["enable_lid"] != true {
		t.Fatal("enable_lid not true")
	}
	if req["enable_nonstream"] != true {
		t.Fatal("enable_nonstream not true (async)")
	}
	if req["enable_itn"] != true || req["enable_punc"] != true || req["enable_ddc"] != false {
		t.Fatal("itn/punc/ddc mismatch")
	}
	if req["boosting_table_name"] != "tbl" {
		t.Fatalf("boosting_table_name = %v", req["boosting_table_name"])
	}
	if _, ok := req["boosting_table_id"]; ok {
		t.Fatal("empty boosting_table_id should be omitted")
	}
	if req["correct_table_id"] != "cid" {
		t.Fatalf("correct_table_id = %v", req["correct_table_id"])
	}
	if _, ok := req["correct_table_name"]; ok {
		t.Fatal("empty correct_table_name should be omitted")
	}

	// nostream 模式下二遍识别不生效
	ns := buildBigmodelRequest(Config{StreamMode: "nostream", EnableNonstream: true})
	if ns["enable_nonstream"] != false {
		t.Fatal("nostream mode should disable nonstream")
	}
}

func testASRResponse(t *testing.T, flags byte, text string, definite bool) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"result": map[string]any{
			"text": text,
			"utterances": []map[string]any{
				{"text": text, "definite": definite},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	response := make([]byte, 12+len(payload))
	response[0] = 0x11 // protocolVersion<<4 | headerSizeVal
	response[1] = 0x90 | flags
	binary.BigEndian.PutUint32(response[4:8], ^uint32(0))
	binary.BigEndian.PutUint32(response[8:12], uint32(len(payload)))
	copy(response[12:], payload)
	return response
}
