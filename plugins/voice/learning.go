package voice

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/c/just-talk-go/config"
	"github.com/c/just-talk-go/hotkey"
	"github.com/c/just-talk-go/internal/correction"
)

type learningWatch struct {
	revision *correction.Revision
	cancel   context.CancelFunc
	context  context.Context
	cfg      config.CorrectionConfig
	hotwords []string
	epoch    uint64
	started  time.Time
	confirm  chan time.Time
}

var confirmLearningCombo = hotkey.Combo{Key: hotkey.KeyEnter}

func (p *VoicePlugin) onLearningEnter(evt hotkey.Event) {
	if evt.Type != hotkey.KeyDown || evt.Combo != confirmLearningCombo {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	w := p.learningWatch
	if w == nil || evt.Time.Before(w.started) {
		return
	}
	select {
	case w.confirm <- evt.Time:
	default:
	}
}

func sameLearningService(a, b config.CorrectionConfig) bool {
	return a.Provider == b.Provider && a.BaseURL == b.BaseURL && a.Model == b.Model && a.APIKey == b.APIKey && a.TimeoutMS == b.TimeoutMS
}

func CorrectionMemoryPath() string {
	return filepath.Join(filepath.Dir(statsPath()), "correction-memory.json")
}

// Scope comes from the recording snapshot, never from model-generated notes.
// Keep the original file and format intact; translation groups start empty.
func InputMemoryPath(language string) (string, error) {
	switch language {
	case "":
		return CorrectionMemoryPath(), nil
	case "en", "ja":
		return filepath.Join(filepath.Dir(statsPath()), "translation-memory."+language+".json"), nil
	default:
		return "", fmt.Errorf("不支持的记忆分组：%s", language)
	}
}

func MemoryGroupName(language string) string {
	if language == "" {
		return "普通输入"
	}
	return correction.TranslationLanguage(language) + "翻译"
}

// Caller holds p.mu and learningCommitMu. The second lock orders disable/quit
// against file commits without holding the voice-state mutex during disk I/O.
func (p *VoicePlugin) stopLearningLocked() {
	p.learningEpoch++
	if p.learningCancel != nil {
		p.learningCancel()
		p.learningCancel = nil
	}
	p.learningContext = nil
	if p.learningWatch != nil {
		p.learningWatch.revision.Stop()
		p.learningWatch.cancel()
		p.learningWatch = nil
	}
	if p.learningHotkeyRegistered {
		if err := p.env.UnregisterHotkey(confirmLearningCombo); err != nil {
			p.logger.Warn("unregister learning Enter failed", "error", err)
		}
		p.learningHotkeyRegistered = false
	}
}

func (p *VoicePlugin) configureLearningLocked(cfg config.CorrectionConfig) {
	old := p.cfg.Correction
	changed := old.Learning != cfg.Learning || !sameLearningService(old, cfg)
	if changed || !correction.LearningReady(cfg) {
		p.stopLearningLocked()
	}
	if !p.learningStopped && p.learningContext == nil && correction.LearningReady(cfg) {
		// Only macOS has readable input capture. Its existing event tap passes
		// Enter through; do not register a grabbed Enter on other platforms.
		if runtime.GOOS == "darwin" && !p.learningHotkeyRegistered {
			if err := p.env.RegisterHotkeyWithOptions(confirmLearningCombo, hotkey.RegisterOptions{Suppress: false}, p.onLearningEnter); err != nil {
				p.logger.Warn("register learning Enter failed", "error", err)
				return
			}
			p.learningHotkeyRegistered = true
		}
		p.learningEpoch++
		p.learningContext, p.learningCancel = context.WithCancel(p.env.Engine().Context())
	}
}

func (p *VoicePlugin) sharedMemory(enabled bool, language string) []string {
	if !enabled {
		return nil
	}
	path, err := InputMemoryPath(language)
	if err != nil {
		p.logger.Warn("invalid memory scope", "error", err)
		return nil
	}
	notes, err := correction.LoadMemory(path)
	if err != nil {
		p.logger.Warn("input memory load failed", "error", err)
		pout("🧠 %s记忆读取失败；本次输入处理继续", MemoryGroupName(language))
		return nil
	}
	return notes
}

func (p *VoicePlugin) flushLearning(sessionID uint64) {
	p.mu.Lock()
	if (sessionID != 0 && p.sessionID != sessionID) || p.learningStopped {
		p.mu.Unlock()
		return
	}
	watch := p.learningWatch
	p.learningWatch = nil
	if watch != nil {
		watch.revision.Stop()
		watch.cancel()
	}
	p.mu.Unlock()
}

func (p *VoicePlugin) startLearning(session *recordingSession, input *correction.Input, raw, output string) bool {
	if !session.autoSubmit || !correction.LearningReady(session.correction) {
		return false
	}
	p.mu.Lock()
	_, canceled := p.canceledSessions[session.sessionID]
	if canceled || p.learningStopped || p.sessionID != session.sessionID || p.learningContext == nil || p.learningContext.Err() != nil || !sameLearningService(p.cfg.Correction, session.correction) {
		p.mu.Unlock()
		return false
	}
	ctx, cancel := context.WithCancel(p.learningContext)
	watch := &learningWatch{revision: correction.NewRevision(input, raw, output, session.translation), cancel: cancel,
		context: p.learningContext, cfg: session.correction, hotwords: session.hotwords, epoch: p.learningEpoch,
		started: time.Now(), confirm: make(chan time.Time, 1)}
	p.learningWatch = watch
	p.mu.Unlock()
	go p.watchRevision(ctx, watch, input)
	return true
}

func (p *VoicePlugin) watchRevision(ctx context.Context, watch *learningWatch, input *correction.Input) {
	defer input.Close()
	defer watch.cancel()
	defer watch.revision.Stop()
	defer func() {
		p.mu.Lock()
		if p.learningWatch == watch {
			p.learningWatch = nil
		}
		p.mu.Unlock()
	}()
	started := time.Now()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	confirm := func(at time.Time) {
		// Recheck the same application/window/editor, even if Enter has already
		// cleared its text. Never read another field or infer that sending worked.
		value, err := input.Read()
		if err != nil || ctx.Err() != nil {
			return
		}
		if feedback := watch.revision.Confirm(at, value); feedback != nil {
			p.learnRevision(watch, feedback)
		}
	}
	for {
		if ctx.Err() != nil {
			return
		}
		select {
		case at := <-watch.confirm:
			confirm(at)
			return
		default:
		}
		value, err := input.Read()
		// A read already in flight when this watch was canceled must not be
		// accepted as a revision of a later paste.
		if ctx.Err() != nil || err != nil {
			return
		}
		now := time.Now()
		select {
		case at := <-watch.confirm:
			confirm(at)
			return
		default:
		}
		if !watch.revision.Observe(value, now) {
			// The app can clear before the pipe dispatcher delivers Enter. Wait
			// briefly for that earlier event; clearing itself never confirms.
			if value == "" {
				timer := time.NewTimer(250 * time.Millisecond)
				defer timer.Stop()
				select {
				case at := <-watch.confirm:
					if !at.After(now) {
						confirm(at)
					}
				case <-timer.C:
				case <-ctx.Done():
				}
			}
			return
		}
		if now.Sub(started) >= 10*time.Minute {
			return
		}
		if !watch.revision.Observed() && now.Sub(started) >= time.Second {
			return
		}
		select {
		case <-ctx.Done():
			return
		case at := <-watch.confirm:
			confirm(at)
			return
		case <-ticker.C:
			continue
		}
	}
}

func (p *VoicePlugin) learnRevision(watch *learningWatch, feedback *correction.Feedback) {
	if watch.context.Err() != nil {
		return
	}
	path, err := InputMemoryPath(feedback.Language)
	if err != nil {
		p.logger.Warn("invalid learning scope", "error", err)
		return
	}
	notes, err := correction.LoadMemory(path)
	if err != nil {
		pout("🧠 %s自学习未完成：%s", MemoryGroupName(feedback.Language), err)
		return
	}
	updates, err := correction.Learn(watch.context, watch.cfg, feedback, notes, watch.hotwords)
	if err != nil {
		if watch.context.Err() == nil {
			p.logger.Warn("input revision learning failed", "error", err)
			pout("🧠 自学习未完成：%s", err)
		}
		return
	}
	if len(updates) == 0 {
		return
	}
	p.learningCommitMu.Lock()
	defer p.learningCommitMu.Unlock()
	p.mu.Lock()
	current := !p.learningStopped && p.learningEpoch == watch.epoch && watch.context.Err() == nil &&
		correction.LearningReady(p.cfg.Correction)
	p.mu.Unlock()
	if !current {
		return
	}
	count, err := correction.Remember(path, updates)
	if err != nil {
		p.logger.Warn("input memory save failed", "error", err)
		pout("🧠 自学习记忆保存失败")
		return
	}
	p.logger.Info("input memory updated", "scope", feedback.Language, "notes", count, "updates", len(updates))
	pout("🧠 %s记忆 %d 条 · 本次提炼 %d 条", MemoryGroupName(feedback.Language), count, len(updates))
}
