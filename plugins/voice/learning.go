package voice

import (
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/c/just-talk-go/internal/correction"
)

type pendingLearning struct {
	target           *correction.Target
	draft, corrected string
	created          time.Time
}

func CorrectionMemoryPath() string {
	return filepath.Join(filepath.Dir(statsPath()), "correction-memory.json")
}

func (p *VoicePlugin) prepareLearning(sessionID uint64, target *correction.Target, enabled bool) *correction.Learning {
	p.mu.Lock()
	_, canceled := p.canceledSessions[sessionID]
	if p.learningStopped || canceled || p.sessionID != sessionID {
		p.mu.Unlock()
		return nil
	}
	previous := p.learningPending
	p.learningPending = nil
	p.mu.Unlock()
	if previous != nil {
		defer previous.target.Close()
	}
	if !enabled {
		return nil
	}
	notes, err := correction.LoadMemory(CorrectionMemoryPath())
	if err != nil {
		p.logger.Warn("correction memory load failed", "error", err)
		pout("🧠 自学习记忆读取失败；本次纠错继续：%s", err)
		return nil
	}
	learning := &correction.Learning{Memory: notes}
	if previous == nil || time.Since(previous.created) > 30*time.Minute || target.SubmittedAfter == nil {
		return learning
	}
	submitted, ok := target.SubmittedAfter(previous.target)
	if !ok {
		return learning
	}
	// Bound feedback tokens without rejecting or delaying the current correction.
	for _, text := range []string{previous.draft, previous.corrected, submitted} {
		if utf8.RuneCountInString(text) > 2000 {
			p.logger.Info("correction learning sample skipped", "reason", "sample over 2000 characters")
			return learning
		}
	}
	learning.Feedback = &correction.Feedback{Draft: previous.draft, Corrected: previous.corrected, Submitted: submitted}
	return learning
}

func (p *VoicePlugin) retainLearningTarget(sessionID uint64, target *correction.Target, draft, corrected string) bool {
	if target.SubmittedAfter == nil {
		return false
	}
	p.mu.Lock()
	_, canceled := p.canceledSessions[sessionID]
	if p.learningStopped || canceled || p.sessionID != sessionID || !p.cfg.Correction.Enabled ||
		!p.cfg.Correction.Learning || p.env.Engine().Context().Err() != nil {
		p.mu.Unlock()
		return false
	}
	previous := p.learningPending
	p.learningPending = &pendingLearning{target: target, draft: draft, corrected: corrected, created: time.Now()}
	p.mu.Unlock()
	if previous != nil {
		previous.target.Close()
	}
	return true
}
