// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package doubao

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"testing"
)

func TestEncodeDecodeFullClientRoundTrip(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{"request": map[string]any{"model_name": "bigmodel"}})
	frame, err := EncodeFullClientRequest(payload, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(frame) < 8 {
		t.Fatalf("frame too short: %d", len(frame))
	}
	// 伪造一帧 server response：同样 gzip JSON
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte(`{"result":{"text":"你好"}}`))
	_ = zw.Close()
	gz := buf.Bytes()
	h := buildHeader(msgFullServer, flagPosSequence, serJSON, compGzip)
	out := append([]byte{}, h...)
	out = append(out, 0, 0, 0, 1) // sequence 1
	out = append(out, byte(len(gz)>>24), byte(len(gz)>>16), byte(len(gz)>>8), byte(len(gz)))
	out = append(out, gz...)
	sf, err := DecodeServerFrame(out)
	if err != nil {
		t.Fatal(err)
	}
	if sf.MsgType != msgFullServer {
		t.Fatalf("type %d", sf.MsgType)
	}
	if !sf.HasSequence || sf.Sequence != 1 {
		t.Fatalf("seq %+v", sf)
	}
	text := extractASRText(sf.Payload)
	if text != "你好" {
		t.Fatalf("text %q", text)
	}
}

func TestEncodeAudioOnlyLastFlag(t *testing.T) {
	f, err := EncodeAudioOnly([]byte("pcm"), false, true)
	if err != nil {
		t.Fatal(err)
	}
	flags := f[1] & 0x0f
	if flags != flagLastNoSeq {
		t.Fatalf("flags %04b", flags)
	}
}

func TestParseConfigResourceIDs(t *testing.T) {
	c, err := ParseConfig(map[string]string{
		"api_key": "k", "model_version": "1.0", "stream_billing": "concurrent", "file_mode": "standard",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.StreamResourceID() != "volc.bigasr.sauc.concurrent" {
		t.Fatal(c.StreamResourceID())
	}
	if c.FileResourceID() != "volc.bigasr.auc" {
		t.Fatal(c.FileResourceID())
	}
	c2, _ := ParseConfig(map[string]string{"api_key": "k", "model_version": "2.0", "file_mode": "flash"})
	if c2.FileResourceID() != "volc.bigasr.auc_turbo" {
		t.Fatal(c2.FileResourceID())
	}
	if c2.StreamResourceID() != "volc.seedasr.sauc.duration" {
		t.Fatal(c2.StreamResourceID())
	}
}

func TestParseConfigResourceIDOverride(t *testing.T) {
	c, err := ParseConfig(map[string]string{
		"api_key": "k", "resource_id": "volc.custom.resource",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.StreamResourceID() != "volc.custom.resource" {
		t.Fatalf("override not honored: %s", c.StreamResourceID())
	}
}

func TestParseConfigDefaultsLegacyCompat(t *testing.T) {
	// 旧版 just-talk 行为：默认 async + duration 资源
	c, err := ParseConfig(map[string]string{"app_id": "a", "access_key": "s"})
	if err != nil {
		t.Fatal(err)
	}
	if c.StreamMode != "async" {
		t.Fatalf("stream_mode=%s", c.StreamMode)
	}
	if c.StreamResourceID() != "volc.bigasr.sauc.duration" {
		t.Fatalf("resource=%s", c.StreamResourceID())
	}
	if c.streamURL() != "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async" {
		t.Fatalf("url=%s", c.streamURL())
	}
}

func TestParseConfigRejectIdle(t *testing.T) {
	_, err := ParseConfig(map[string]string{"api_key": "k", "file_mode": "idle"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseConfigRejectLegacyStreamMode(t *testing.T) {
	// 文档仅 bigmodel_nostream / bigmodel_async 两个端点，stream 模式已移除
	if _, err := ParseConfig(map[string]string{"api_key": "k", "stream_mode": "stream"}); err == nil {
		t.Fatal("stream_mode=stream should be rejected")
	}
}

func TestParseConfigRecognitionFlags(t *testing.T) {
	c, err := ParseConfig(map[string]string{
		"api_key":                  "k",
		"enable_lid":               "true",
		"enable_nonstream":         "false",
		"enable_itn":               "false",
		"enable_punc":              "false",
		"enable_ddc":               "true",
		"boosting_table_name":      "tbl",
		"boosting_table_id":        "id1",
		"correct_table_name":       "ctn",
		"correct_table_id":         "id2",
		"regex_correct_table_name": "rtn",
		"regex_correct_table_id":   "id3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !c.EnableLID {
		t.Fatal("enable_lid should be true")
	}
	if c.EnableNonstream {
		t.Fatal("enable_nonstream should be false")
	}
	if c.EnableITN {
		t.Fatal("enable_itn should be false")
	}
	if c.EnablePunc {
		t.Fatal("enable_punc should be false")
	}
	if !c.EnableDDC {
		t.Fatal("enable_ddc should be true")
	}
	if c.BoostingTableName != "tbl" || c.BoostingTableID != "id1" {
		t.Fatalf("boosting table = %q/%q", c.BoostingTableName, c.BoostingTableID)
	}
	if c.CorrectTableName != "ctn" || c.CorrectTableID != "id2" {
		t.Fatalf("correct table = %q/%q", c.CorrectTableName, c.CorrectTableID)
	}
	if c.RegexCorrectTableName != "rtn" || c.RegexCorrectTableID != "id3" {
		t.Fatalf("regex correct table = %q/%q", c.RegexCorrectTableName, c.RegexCorrectTableID)
	}
}

func TestParseConfigRecognitionDefaults(t *testing.T) {
	c, err := ParseConfig(map[string]string{"api_key": "k"})
	if err != nil {
		t.Fatal(err)
	}
	if c.EnableLID {
		t.Fatal("enable_lid default should be false")
	}
	if !c.EnableNonstream {
		t.Fatal("enable_nonstream default should be true")
	}
	if !c.EnableITN {
		t.Fatal("enable_itn default should be true")
	}
	if !c.EnablePunc {
		t.Fatal("enable_punc default should be true")
	}
	if c.EnableDDC {
		t.Fatal("enable_ddc default should be false")
	}
	if c.BoostingTableName != "" || c.BoostingTableID != "" ||
		c.CorrectTableName != "" || c.CorrectTableID != "" ||
		c.RegexCorrectTableName != "" || c.RegexCorrectTableID != "" {
		t.Fatal("table fields should default empty")
	}
}
