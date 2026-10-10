package overlay

import (
	"context"
	"log/slog"
	"time"

	"github.com/c/just-talk-go/config"
	"github.com/c/just-talk-go/engine"
	"github.com/c/just-talk-go/plugins/voice"
)

type backend interface {
	Show(label, text string, color statusColor) error
	Hide() error
	Close() error
}

type statusColor struct {
	R uint16
	G uint16
	B uint16
}

type Plugin struct {
	logger      *slog.Logger
	cfg         config.OverlayConfig
	backend     backend
	lastState   string
	lastLabel   string
	lastText    string
	lastVisible bool
	lastLevel   float64
}

func NewOverlayPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string    { return "overlay" }
func (p *Plugin) Version() string { return "0.1.0" }

func (p *Plugin) Init(env engine.PluginEnv) error {
	p.logger = env.Logger()
	p.cfg = env.Config().Overlay
	return nil
}

func (p *Plugin) Start(ctx context.Context) error {
	if !p.cfg.Enabled {
		return nil
	}
	b, err := newBackend(p.cfg)
	if err != nil {
		p.logger.Warn("overlay unavailable", "error", err)
		return nil
	}
	p.backend = b
	defer p.backend.Close()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	p.logger.Info("overlay started")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			p.sync(voice.TUIStatus())
		}
	}
}

func (p *Plugin) Stop() error {
	if p.backend != nil {
		return p.backend.Close()
	}
	return nil
}

func (p *Plugin) sync(status voice.TUIVoiceStatus) {
	label, color, visible := displayForStatus(status, p.cfg.IdleVisible)
	text := status.Transcript
	if status.State == "error" && status.Detail != "" {
		text = status.Detail
	}
	if status.AudioLevel != p.lastLevel {
		if err := updateAudioLevel(p.backend, status.AudioLevel); err != nil {
			p.logger.Debug("overlay meter failed", "error", err)
		} else {
			p.lastLevel = status.AudioLevel
		}
	}
	if status.State == "idle" && !status.TranscriptUntil.After(time.Now()) {
		text = ""
	}
	if runes := []rune(text); len(runes) > 100 {
		text = "…" + string(runes[len(runes)-100:])
	}
	if status.State == p.lastState && label == p.lastLabel && text == p.lastText && visible == p.lastVisible {
		return
	}

	if !visible {
		if err := p.backend.Hide(); err != nil {
			p.logger.Debug("overlay hide failed", "error", err)
			return
		}
		p.lastState, p.lastLabel, p.lastText, p.lastVisible = status.State, label, text, visible
		return
	}
	if err := p.backend.Show(label, text, color); err != nil {
		p.logger.Debug("overlay show failed", "error", err)
		return
	}
	p.lastState, p.lastLabel, p.lastText, p.lastVisible = status.State, label, text, visible
}

func displayForStatus(status voice.TUIVoiceStatus, idleVisible bool) (string, statusColor, bool) {
	switch status.State {
	case "connecting":
		return "CON", statusColor{R: 245 << 8, G: 190 << 8, B: 70 << 8}, true
	case "recording":
		return "REC", statusColor{R: 255 << 8, G: 65 << 8, B: 65 << 8}, true
	case "stopping_delayed":
		return "STP", statusColor{R: 255 << 8, G: 140 << 8, B: 60 << 8}, true
	case "stopping":
		return "WAI", statusColor{R: 255 << 8, G: 160 << 8, B: 70 << 8}, true
	case "correcting":
		return "FIX", statusColor{R: 125 << 8, G: 170 << 8, B: 255 << 8}, true
	case "error":
		return "ERR", statusColor{R: 255 << 8, G: 65 << 8, B: 65 << 8}, true
	default:
		if status.State == "idle" && status.Transcript != "" && status.TranscriptUntil.After(time.Now()) {
			return "DONE", statusColor{R: 80 << 8, G: 210 << 8, B: 145 << 8}, true
		}
		return "IDL", statusColor{R: 145 << 8, G: 145 << 8, B: 145 << 8}, idleVisible
	}
}
