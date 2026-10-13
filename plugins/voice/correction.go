package voice

import (
	"context"
	"errors"
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

func (p *VoicePlugin) copyTranscriptText(text string) error {
	return p.copyTranscriptTextChecked(text, nil)
}

func (p *VoicePlugin) copyTranscriptTextChecked(text string, guard func() error) error {
	p.outputMu.Lock()
	defer p.outputMu.Unlock()
	if guard != nil {
		if err := guard(); err != nil {
			return err
		}
	}
	if p.correctionClipboard != nil {
		return p.correctionClipboard(text)
	}
	return writeClipboard(text)
}

func (p *VoicePlugin) pasteTranscript(text string, guard func() error, delivered func()) error {
	p.outputMu.Lock()
	defer p.outputMu.Unlock()
	// Programmatic output must never be mistaken for a user edit of the prior
	// dictation. Install the next watch before another paste can start.
	p.flushLearning(0)
	if guard == nil {
		guard = func() error { return nil }
	}
	var err error
	if p.correctionPaste != nil {
		err = p.correctionPaste(text, guard)
	} else {
		err = autotype.PasteChecked(text, p.logger, guard)
	}
	if err == nil && delivered != nil {
		delivered()
	}
	return err
}

func (p *VoicePlugin) outputTranscript(session *recordingSession, text string) {
	// Preserve ordinary voice delivery when no input-learning binding is needed.
	if !session.correction.Enabled && session.translation == "" && (!session.autoSubmit || !correction.LearningReady(session.correction)) {
		p.dispatchTextOutput(session, text)
		return
	}
	if !p.correctionSessionCurrent(session.sessionID) {
		if !session.correction.Enabled && session.translation == "" {
			p.dispatchTextOutput(session, text)
		}
		return
	}
	p.flushLearning(session.sessionID)
	if err := p.copyTranscriptText(text); err != nil {
		p.publishError("识别原文复制失败", session.sessionID)
		return
	}
	var target *correction.Target
	var input *correction.Input
	var destination *correction.Destination
	inputRetained := false
	defer func() {
		if destination != nil {
			destination.Close()
		}
		if target != nil {
			target.Close()
		}
		if input != nil && !inputRetained {
			input.Close()
		}
	}()
	model := session.correction.Enabled || session.translation != ""
	if session.correction.Enabled && session.correction.Context {
		capture := p.correctionCapture
		if capture == nil {
			capture = correction.Capture
		}
		var err error
		target, err = capture()
		if err != nil && !errors.Is(err, correction.ErrUnsupportedContext) {
			p.correctionFailed(session.sessionID, err)
			return
		}
	}
	// Unmatched applications explicitly use input-only correction. A failure
	// from a matched context adapter never reaches this path.
	if model && target == nil {
		capture := p.correctionDestinationCapture
		if capture == nil {
			capture = correction.CaptureDestination
		}
		var err error
		destination, err = capture()
		if err != nil {
			if session.translation != "" || !errors.Is(err, correction.ErrUnsupportedInput) {
				p.correctionFailed(session.sessionID, err)
				return
			}
			model = false
			pout("ℹ️ 当前平台使用普通语音输入")
		}
	}
	if (model && target == nil) || (session.autoSubmit && correction.LearningReady(session.correction)) {
		capture := p.correctionInputCapture
		if capture == nil {
			capture = correction.CaptureInput
		}
		var err error
		input, err = capture()
		if err != nil {
			p.logger.Info("input revision capture unavailable", "error", err)
			if session.correction.Learning {
				pout("🧠 当前应用未提供可读输入框；本次不采集修订")
			}
		}
	}
	guard := func() error {
		if !model {
			return nil
		}
		if !p.correctionSessionCurrent(session.sessionID) {
			return errors.New("已取消或开始了新录音，结果未上屏")
		}
		if model {
			if target != nil {
				if err := target.Guard(); err != nil {
					return err
				}
			} else if destination != nil {
				if err := destination.Guard(); err != nil {
					return err
				}
				if input != nil {
					if err := input.Guard(); err != nil {
						return err
					}
				}
			} else {
				return errors.New("没有绑定可用输入框，结果未上屏")
			}
		}
		if !p.correctionSessionCurrent(session.sessionID) {
			return errors.New("已取消或开始了新录音，结果未上屏")
		}
		return nil
	}
	output := text
	started := time.Now()
	if model {
		ctx, cancel := context.WithCancel(p.env.Engine().Context())
		defer cancel()
		p.mu.Lock()
		if p.correctionCancels == nil {
			p.correctionCancels = make(map[uint64]context.CancelFunc)
		}
		p.correctionCancels[session.sessionID] = cancel
		if session.translation != "" {
			if p.translatingSessions == nil {
				p.translatingSessions = make(map[uint64]string)
			}
			action := "正在翻译为"
			if session.correction.Enabled {
				action = "正在整理并翻译为"
			}
			p.translatingSessions[session.sessionID] = action + correction.TranslationLanguage(session.translation)
		}
		p.publishStatusLocked()
		p.mu.Unlock()
		defer func() {
			p.mu.Lock()
			delete(p.correctionCancels, session.sessionID)
			delete(p.translatingSessions, session.sessionID)
			p.publishStatusLocked()
			p.mu.Unlock()
		}()
		if err := guard(); err != nil {
			p.correctionFailed(session.sessionID, err)
			return
		}
		var messages []correction.Message
		var notes []string
		if session.correction.Enabled {
			notes = p.sharedMemory(session.correction.Learning, "")
		}
		if target != nil {
			messages = target.Messages
			users, assistants, chars := 0, 0, 0
			for _, m := range messages {
				if m.Role == "user" {
					users++
				} else {
					assistants++
				}
				chars += len([]rune(m.Text))
			}
			p.logger.Info("correction context", "source", target.Source.ID, "available_user_messages", target.AvailableUsers,
				"available_assistant_messages", target.AvailableAssistants, "user_messages", users, "assistant_messages", assistants, "context_chars", chars)
			pout("✨ 上下文 · %s · 用户 %d / 助手 %d · %d 字符", target.Source.Name, users, assistants, chars)
		}
		var err error
		if session.translation != "" {
			translationNotes := p.sharedMemory(session.correction.Learning, session.translation)
			action := "翻译"
			if session.correction.Enabled {
				action = "整理并翻译"
			}
			pout("🌐 %s为%s · 一次模型请求 · 普通记忆 %d / 译文记忆 %d 条", action, correction.TranslationLanguage(session.translation), len(notes), len(translationNotes))
			output, err = correction.Translate(ctx, session.correction, text, session.translation, messages, session.hotwords, notes, translationNotes)
		} else {
			pout("✨ AI 输入修正 · 普通记忆 %d 条", len(notes))
			output, err = correction.Correct(ctx, session.correction, text, messages, session.hotwords, notes)
		}
		if ctx.Err() != nil || !p.correctionSessionCurrent(session.sessionID) {
			return
		}
		if err != nil {
			p.correctionFailed(session.sessionID, err)
			return
		}

	}
	var err error
	if session.autoSubmit {
		err = p.pasteTranscript(output, guard, func() {
			if input != nil {
				inputRetained = p.startLearning(session, input, text, output)
			}
		})
	} else {
		err = guard()
		if err == nil && output != text {
			err = p.copyTranscriptTextChecked(output, guard)
		}
	}
	if err != nil {
		if p.correctionSessionCurrent(session.sessionID) {
			if restoreErr := p.copyTranscriptText(text); restoreErr != nil {
				p.publishError("上屏未完成，识别原文复制失败", session.sessionID)
				return
			}
		}
		p.correctionFailed(session.sessionID, err)
		return
	}
	p.updateTranscript(session.sessionID, output)
	if model {
		p.logger.Info("correction completed", "model", session.correction.Model, "duration_ms", time.Since(started).Milliseconds(), "text_len", len(output))
		if session.translation != "" {
			pout("🌐 已翻译为%s (%dms)", correction.TranslationLanguage(session.translation), time.Since(started).Milliseconds())
		} else {
			pout("✨ 已纠错整理 (%dms)", time.Since(started).Milliseconds())
		}
	}
	if session.autoSubmit {
		pout("✅ 已上屏")
	} else {
		pout("📋 已复制")
	}
}

func (p *VoicePlugin) correctionFailed(sessionID uint64, err error) {
	if !p.correctionSessionCurrent(sessionID) {
		return
	}
	p.logger.Warn("correction or paste failed", "session_id", sessionID, "error", err)
	pout("❌ 输入处理或上屏未完成: %s；原始识别已复制", err)
	p.publishError("处理未上屏: "+shortError(err)+"；可粘贴原始识别", sessionID)
}
