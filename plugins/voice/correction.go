package voice

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/c/just-talk-go/internal/autotype"
	"github.com/c/just-talk-go/internal/correction"
)

func (p *VoicePlugin) correctionSessionCurrent(sessionID uint64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, canceled := p.canceledSessions[sessionID]
	return !canceled && p.sessionID == sessionID && p.env.Engine().Context().Err() == nil
}

func (p *VoicePlugin) copyCorrectionText(text string) error {
	if p.correctionClipboard != nil {
		return p.correctionClipboard(text)
	}
	return writeClipboard(text)
}

func (p *VoicePlugin) outputTranscript(session *recordingSession, text string) {
	if !session.correction.Enabled {
		p.dispatchTextOutput(text, session.autoSubmit)
		return
	}
	if !p.correctionSessionCurrent(session.sessionID) {
		return
	}
	capture := p.correctionCapture
	if capture == nil {
		capture = correction.Capture
	}
	target, err := capture()
	if errors.Is(err, correction.ErrNotCodex) {
		// Codex 纠错仅作用于 Codex；其他应用仍按原语音输入流程上屏。
		p.dispatchTextOutput(text, session.autoSubmit)
		return
	}
	if target != nil {
		defer target.Close()
	}
	if !p.correctionSessionCurrent(session.sessionID) {
		return
	}
	// 模型错误、超时或焦点改变时，识别原文仍可从剪贴板取回。
	if copyErr := p.copyCorrectionText(text); copyErr != nil {
		p.publishError("识别原文复制失败，未调用纠错服务", session.sessionID)
		return
	}
	if err != nil {
		p.correctionFailed(session.sessionID, err)
		return
	}
	ctx, cancel := context.WithCancel(p.env.Engine().Context())
	defer cancel()
	p.mu.Lock()
	if p.correctionCancels == nil {
		p.correctionCancels = make(map[uint64]context.CancelFunc)
	}
	p.correctionCancels[session.sessionID] = cancel
	p.publishStatusLocked()
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.correctionCancels, session.sessionID)
		p.publishStatusLocked()
		p.mu.Unlock()
	}()
	if !p.correctionSessionCurrent(session.sessionID) {
		return
	}
	users, assistants, chars := 0, 0, 0
	mentionsDeepSeek := false
	for _, message := range target.Messages {
		if message.Role == "user" {
			users++
		} else {
			assistants++
		}
		chars += len([]rune(message.Text))
		mentionsDeepSeek = mentionsDeepSeek || strings.Contains(strings.ToLower(message.Text), "deepseek")
	}
	p.logger.Info("correction context", "available_user_messages", target.AvailableUsers, "available_assistant_messages", target.AvailableAssistants, "user_messages", users, "assistant_messages", assistants, "context_chars", chars, "mentions_deepseek", mentionsDeepSeek)
	pout("✨ 纠错中 · 上下文用户 %d / 助手 %d · %d 字符", users, assistants, chars)
	started := time.Now()
	corrected, err := correction.Correct(ctx, session.correction, text, target.Messages, session.hotwords)
	if !p.correctionSessionCurrent(session.sessionID) || ctx.Err() != nil {
		return
	}
	if err != nil {
		p.correctionFailed(session.sessionID, err)
		return
	}
	guard := func() error {
		if !p.correctionSessionCurrent(session.sessionID) {
			return errors.New("已取消或开始了新录音，结果未上屏")
		}
		if err := target.Guard(); err != nil {
			return err
		}
		if !p.correctionSessionCurrent(session.sessionID) {
			return errors.New("已取消或开始了新录音，结果未上屏")
		}
		return nil
	}
	if session.autoSubmit {
		err = autotype.PasteChecked(corrected, p.logger, guard)
	} else {
		err = guard()
		if err == nil {
			err = p.copyCorrectionText(corrected)
		}
	}
	if err != nil {
		if p.correctionSessionCurrent(session.sessionID) {
			if restoreErr := p.copyCorrectionText(text); restoreErr != nil {
				p.publishError("纠错未上屏，识别原文复制失败", session.sessionID)
				return
			}
		}
		p.correctionFailed(session.sessionID, err)
		return
	}
	p.updateTranscript(session.sessionID, corrected)
	p.logger.Info("correction completed", "model", session.correction.Model,
		"duration_ms", time.Since(started).Milliseconds(), "context_messages", len(target.Messages), "text_len", len(corrected))
	pout("✨ 已纠错整理 (%dms)", time.Since(started).Milliseconds())
	if session.autoSubmit {
		pout("✅ 已上屏")
	} else {
		pout("📋 已复制整理结果")
	}
}

func (p *VoicePlugin) correctionFailed(sessionID uint64, err error) {
	if !p.correctionSessionCurrent(sessionID) {
		return
	}
	pout("❌ 纠错或上屏未完成: %s；原始识别已复制", err)
	p.publishError("纠错未上屏: "+shortError(err)+"；可粘贴原始识别", sessionID)
}
