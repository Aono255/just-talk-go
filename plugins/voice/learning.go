package voice

import (
	"context"
	"path/filepath"
	"time"

	"github.com/c/just-talk-go/config"
	"github.com/c/just-talk-go/internal/correction"
)

type learningWatch struct {
	revision *correction.Revision
	cancel   context.CancelFunc
	context  context.Context
	cfg      config.CorrectionConfig
	hotwords []string
	epoch    uint64
}

func sameLearningService(a, b config.CorrectionConfig) bool {
	return a.Provider == b.Provider && a.BaseURL == b.BaseURL && a.Model == b.Model && a.APIKey == b.APIKey && a.TimeoutMS == b.TimeoutMS
}

func CorrectionMemoryPath() string {
	return filepath.Join(filepath.Dir(statsPath()), "correction-memory.json")
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
		p.learningWatch.revision.Stop(false)
		p.learningWatch.cancel()
		p.learningWatch = nil
	}
}

func (p *VoicePlugin) configureLearningLocked(cfg config.CorrectionConfig) {
	old := p.cfg.Correction
	changed := old.Learning != cfg.Learning || !sameLearningService(old, cfg)
	if changed || !correction.LearningReady(cfg) {
		p.stopLearningLocked()
	}
	if !p.learningStopped && p.learningContext == nil && correction.LearningReady(cfg) {
		p.learningEpoch++
		p.learningContext, p.learningCancel = context.WithCancel(p.env.Engine().Context())
	}
}

func (p *VoicePlugin) sharedMemory(enabled bool) []string {
	if !enabled {
		return nil
	}
	notes, err := correction.LoadMemory(CorrectionMemoryPath())
	if err != nil {
		p.logger.Warn("input memory load failed", "error", err)
		pout("🧠 记忆读取失败；本次输入修正继续")
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
	var feedback *correction.Feedback
	if watch != nil {
		feedback = watch.revision.Stop(true)
		watch.cancel()
	}
	p.mu.Unlock()
	if feedback != nil {
		go p.learnRevision(watch, feedback)
	}
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
	watch := &learningWatch{revision: correction.NewRevision(input, raw, output), cancel: cancel,
		context: p.learningContext, cfg: session.correction, hotwords: session.hotwords, epoch: p.learningEpoch}
	p.learningWatch = watch
	p.mu.Unlock()
	go p.watchRevision(ctx, watch, input)
	return true
}

func (p *VoicePlugin) watchRevision(ctx context.Context, watch *learningWatch, input *correction.Input) {
	defer input.Close()
	defer watch.cancel()
	started := time.Now()
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	keep := true
	for {
		if ctx.Err() != nil {
			keep = false
			break
		}
		value, err := input.Read()
		// A read already in flight when this watch was canceled must not be
		// accepted as a revision of a later paste.
		if ctx.Err() != nil {
			keep = false
			break
		}
		now := time.Now()
		if err != nil || !watch.revision.Observe(value, now) {
			break
		}
		if watch.revision.Ready(now) || now.Sub(started) >= 2*time.Minute {
			break
		}
		if !watch.revision.Observed() && now.Sub(started) >= time.Second {
			keep = false
			break
		}
		select {
		case <-ctx.Done():
			keep = false
		case <-ticker.C:
			continue
		}
		break
	}
	feedback := watch.revision.Stop(keep)
	p.mu.Lock()
	if p.learningWatch == watch {
		p.learningWatch = nil
	}
	p.mu.Unlock()
	if feedback != nil {
		p.learnRevision(watch, feedback)
	}
}

func (p *VoicePlugin) learnRevision(watch *learningWatch, feedback *correction.Feedback) {
	if watch.context.Err() != nil {
		return
	}
	notes := p.sharedMemory(true)
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
	count, err := correction.Remember(CorrectionMemoryPath(), updates)
	if err != nil {
		p.logger.Warn("input memory save failed", "error", err)
		pout("🧠 自学习记忆保存失败")
		return
	}
	p.logger.Info("input memory updated", "notes", count, "updates", len(updates))
	pout("🧠 自学习记忆 %d 条 · 本次提炼 %d 条", count, len(updates))
}
