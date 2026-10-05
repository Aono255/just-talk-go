// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package doubao

// WebSocket 二进制帧（大模型流式 ASR）：header + optional sequence + payload size + payload。
// 整数均为大端。参考豆包语音 API：protocol version 1、header size 4。

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	protocolVersion = 0b0001
	headerSizeVal   = 0b0001 // 1 * 4 = 4 bytes

	msgFullClient  = 0b0001
	msgAudioOnly   = 0b0010
	msgFullServer  = 0b1001
	msgErrorServer = 0b1111

	flagNone        = 0b0000
	flagPosSequence = 0b0001
	flagLastNoSeq   = 0b0010
	flagLastNegSeq  = 0b0011

	serRaw  = 0b0000
	serJSON = 0b0001

	compNone = 0b0000
	compGzip = 0b0001
)

func gzipBytes(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(b); err != nil {
		_ = w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gunzipBytes(b []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func buildHeader(msgType, flags, serialization, compression byte) []byte {
	b0 := byte((protocolVersion << 4) | headerSizeVal)
	b1 := byte((msgType << 4) | (flags & 0x0f))
	b2 := byte((serialization << 4) | (compression & 0x0f))
	return []byte{b0, b1, b2, 0x00}
}

// EncodeFullClientRequest 首包：JSON 参数（可选 gzip）。
func EncodeFullClientRequest(jsonPayload []byte, useGzip bool) ([]byte, error) {
	payload := jsonPayload
	comp := byte(compNone)
	if useGzip {
		var err error
		payload, err = gzipBytes(jsonPayload)
		if err != nil {
			return nil, err
		}
		comp = compGzip
	}
	h := buildHeader(msgFullClient, flagNone, serJSON, comp)
	out := make([]byte, 0, 4+4+len(payload))
	out = append(out, h...)
	var sz [4]byte
	binary.BigEndian.PutUint32(sz[:], uint32(len(payload)))
	out = append(out, sz[:]...)
	out = append(out, payload...)
	return out, nil
}

// EncodeAudioOnly 音频包；last=true 表示最后一包（flags=0b0010，无 sequence）。
func EncodeAudioOnly(audio []byte, useGzip, last bool) ([]byte, error) {
	payload := audio
	comp := byte(compNone)
	if useGzip {
		var err error
		payload, err = gzipBytes(audio)
		if err != nil {
			return nil, err
		}
		comp = compGzip
	}
	flags := byte(flagNone)
	if last {
		flags = flagLastNoSeq
	}
	h := buildHeader(msgAudioOnly, flags, serRaw, comp)
	out := make([]byte, 0, 4+4+len(payload))
	out = append(out, h...)
	var sz [4]byte
	binary.BigEndian.PutUint32(sz[:], uint32(len(payload)))
	out = append(out, sz[:]...)
	out = append(out, payload...)
	return out, nil
}

// ServerFrame 解析后的服务端帧。
type ServerFrame struct {
	MsgType       byte
	Flags         byte
	Serialization byte
	Compression   byte
	Sequence      int32
	HasSequence   bool
	Payload       []byte // 已解压
	ErrorCode     uint32
	ErrorMessage  string
}

// DecodeServerFrame 解析一帧完整 binary message。
func DecodeServerFrame(raw []byte) (*ServerFrame, error) {
	if len(raw) < 4 {
		return nil, fmt.Errorf("doubao frame: too short (%d)", len(raw))
	}
	f := &ServerFrame{
		MsgType:       (raw[1] >> 4) & 0x0f,
		Flags:         raw[1] & 0x0f,
		Serialization: (raw[2] >> 4) & 0x0f,
		Compression:   raw[2] & 0x0f,
	}
	off := 4

	if f.MsgType == msgErrorServer {
		if len(raw) < off+8 {
			return nil, fmt.Errorf("doubao error frame: truncated")
		}
		f.ErrorCode = binary.BigEndian.Uint32(raw[off : off+4])
		off += 4
		esz := int(binary.BigEndian.Uint32(raw[off : off+4]))
		off += 4
		if esz < 0 || off+esz > len(raw) {
			return nil, fmt.Errorf("doubao error frame: bad size %d", esz)
		}
		msg := raw[off : off+esz]
		if f.Compression == compGzip {
			u, err := gunzipBytes(msg)
			if err == nil {
				msg = u
			}
		}
		f.ErrorMessage = string(msg)
		return f, nil
	}

	// sequence 在 flags 含 0b0001 或 0b0011 时存在
	if f.Flags == flagPosSequence || f.Flags == flagLastNegSeq {
		if len(raw) < off+4 {
			return nil, fmt.Errorf("doubao frame: missing sequence")
		}
		f.Sequence = int32(binary.BigEndian.Uint32(raw[off : off+4]))
		f.HasSequence = true
		off += 4
	}

	if len(raw) < off+4 {
		return nil, fmt.Errorf("doubao frame: missing payload size")
	}
	psz := int(binary.BigEndian.Uint32(raw[off : off+4]))
	off += 4
	if psz < 0 || off+psz > len(raw) {
		return nil, fmt.Errorf("doubao frame: bad payload size %d (remain %d)", psz, len(raw)-off)
	}
	payload := raw[off : off+psz]
	if f.Compression == compGzip && len(payload) > 0 {
		u, err := gunzipBytes(payload)
		if err != nil {
			return nil, fmt.Errorf("doubao gunzip: %w", err)
		}
		payload = u
	}
	f.Payload = payload
	return f, nil
}
