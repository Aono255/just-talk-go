// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package speech

import "context"

// StreamResult 流式识别过程中的一条结果。
type StreamResult struct {
	Text    string
	IsFinal bool
	Error   error
}

// StreamParams 开启流式识别会话的参数。
type StreamParams struct {
	Lang     string   // BCP-47；空 = 引擎默认/自动
	Hotwords []string // 热词（引擎支持时生效）
}

// StreamSession 一次流式识别会话。实现方应保证：
//   - Connect 建立连接（可重试语义由调用方控制），失败返回可读错误
//   - SendAudio 并发安全（内部串行化写）
//   - Results/Done/Final 只读通道，调用方不得关闭
//   - LastText 返回当前累积文本（并发安全）
//   - Close 释放连接；调用后 Done 应关闭
type StreamSession interface {
	Connect(ctx context.Context) error
	SendAudio(ctx context.Context, pcm []byte, isLast bool) error
	Results() <-chan StreamResult
	Done() <-chan struct{}
	Final() <-chan struct{}
	LastText() string
	Close() error
}

// TraceableSession 可选地暴露上游服务返回的排障追踪 ID。
// 目前只有豆包 ASR 流式会话提供该字段。
type TraceableSession interface {
	UpstreamLogID() string
}

// AudioSentObservable 可选地报告已成功交给 ASR 传输层的 PCM。回调用于生成
// 与实际上游音频一致的本地审计录音；调用方不得阻塞回调。
type AudioSentObservable interface {
	SetAudioSentObserver(func([]byte))
}

// AudioSentFormatProvider 可选地描述 AudioSentObservable 回调中的 PCM 格式。
type AudioSentFormatProvider interface {
	AudioSentSampleRate() int
}

// UpstreamLogID 返回会话对应的上游日志 ID；不支持时返回空字符串。
func UpstreamLogID(session StreamSession) string {
	if traceable, ok := session.(TraceableSession); ok {
		return traceable.UpstreamLogID()
	}
	return ""
}

// StreamingEngine 由支持流式识别的引擎实现；批处理引擎可被 BatchSession 包装。
type StreamingEngine interface {
	Engine
	// NewStream 创建一个新会话（不连接；连接在 Session.Connect 中进行）。
	NewStream(ctx context.Context, params StreamParams) (StreamSession, error)
}
