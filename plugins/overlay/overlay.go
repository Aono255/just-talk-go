package overlay

import (
	"context"
	"log/slog"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/c/just-talk-go/config"
	"github.com/c/just-talk-go/engine"
	"github.com/c/just-talk-go/plugins/voice"
)

const (
	idlePollInterval   = 100 * time.Millisecond
	activePollInterval = 50 * time.Millisecond
	// doneHold keeps the done frame up long enough for the macOS helper's
	// own 0.6s done animation before a hide can follow.
	doneHold = 800 * time.Millisecond
	// levelSteps quantizes mic level to 0.05 so tiny fluctuations don't resend frames.
	levelSteps = 20
)

type backend interface {
	Show(f frame) error
	Hide() error
	Close() error
}

type statusColor struct {
	R uint16
	G uint16
	B uint16
}

// frame is one rendered overlay state. Backends that only draw a status
// capsule use Label and Color and ignore the rest.
type frame struct {
	State  string
	Label  string
	Color  statusColor
	Text   string
	Level  float64
	Detail string
}

type Plugin struct {
	logger      *slog.Logger
	cfg         config.OverlayConfig
	notch       bool // macOS notch mode: live text, level and done frames
	open        func(config.OverlayConfig) (backend, error)
	backend     backend
	last        frame
	lastVisible bool
	synced      bool
	doneSeq     uint64
	doneUntil   time.Time

	// OnConfigReload hands the newest config to the Start loop, which owns
	// the backend; reload signals it without blocking the caller.
	pendingMu sync.Mutex
	pending   *config.OverlayConfig
	reload    chan struct{}
}

func NewOverlayPlugin() *Plugin {
	return &Plugin{open: newBackend, reload: make(chan struct{}, 1)}
}

func (p *Plugin) Name() string    { return "overlay" }
func (p *Plugin) Version() string { return "0.1.0" }

func (p *Plugin) Init(env engine.PluginEnv) error {
	p.logger = env.Logger()
	p.cfg = env.Config().Overlay
	p.notch = notchMode(runtime.GOOS, p.cfg.Position)
	return nil
}

// notchMode reports whether the overlay renders the macOS notch (also the
// macOS default when position is unset). Every other platform and position
// keeps the plain status capsule.
func notchMode(goos, position string) bool {
	return goos == "darwin" && (position == "notch" || position == "")
}

func (p *Plugin) Start(ctx context.Context) error {
	defer p.closeBackend()
	if p.cfg.Enabled {
		p.openBackend()
	}
	timer := time.NewTimer(idlePollInterval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.reload:
			if cfg, ok := p.takePending(); ok {
				p.reconfigure(cfg)
			}
			// Sync right away so a rebuilt helper picks up the current state.
			timer.Reset(0)
		case <-voice.OverlayUpdates():
			// State or partial text changed: render now instead of on the
			// next poll tick. Polling still drives level and hold timeouts.
			timer.Reset(0)
		case now := <-timer.C:
			if p.backend == nil {
				// Disabled or unavailable: idle until a reload opens a backend.
				continue
			}
			interval := idlePollInterval
			if p.sync(voice.TUIStatus(), voice.OverlayStatus(), now) {
				interval = activePollInterval
			}
			timer.Reset(interval)
		}
	}
}

// Stop is a no-op: Start closes the backend when its context is cancelled,
// so the backend is only ever touched from the Start goroutine.
func (p *Plugin) Stop() error {
	return nil
}

// OnConfigReload never blocks hotkey or TUI callers; the Start loop applies
// the newest config on its next turn.
func (p *Plugin) OnConfigReload(cfg *config.Config) error {
	overlay := cfg.Overlay
	p.pendingMu.Lock()
	p.pending = &overlay
	p.pendingMu.Unlock()
	select {
	case p.reload <- struct{}{}:
	default:
	}
	return nil
}

func (p *Plugin) takePending() (config.OverlayConfig, bool) {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	if p.pending == nil {
		return config.OverlayConfig{}, false
	}
	cfg := *p.pending
	p.pending = nil
	return cfg, true
}

// needsNewBackend reports whether the helper or window must be rebuilt:
// backends read position and scale only when they are created.
func needsNewBackend(old, next config.OverlayConfig) bool {
	return old.Position != next.Position || old.Scale != next.Scale
}

// reconfigure applies a reloaded config. Enabling opens the backend,
// disabling closes it, and a position or scale change closes the old backend
// before opening the new one so two helpers never run at once. idle_visible
// and show_text are read by sync and take effect on the next frame.
func (p *Plugin) reconfigure(next config.OverlayConfig) {
	old := p.cfg
	p.cfg = next
	p.notch = notchMode(runtime.GOOS, next.Position)
	switch {
	case !next.Enabled:
		p.closeBackend()
	case p.backend == nil:
		p.openBackend()
	case needsNewBackend(old, next):
		// A rebuild keeps doneSeq so a result landing meanwhile still shows.
		seq := p.doneSeq
		p.closeBackend()
		p.openBackend()
		p.doneSeq = seq
		p.logger.Info("overlay rebuilt", "position", next.Position, "scale", next.Scale)
	}
}

func (p *Plugin) openBackend() {
	b, err := p.open(p.cfg)
	if err != nil {
		p.logger.Warn("overlay unavailable", "error", err)
		return
	}
	p.backend = b
	// Results finished before the overlay opened are not replayed.
	p.doneSeq = voice.OverlayStatus().LastOutput.Seq
	p.doneUntil = time.Time{}
	p.synced = false
	p.logger.Info("overlay started")
}

func (p *Plugin) closeBackend() {
	if p.backend == nil {
		return
	}
	if err := p.backend.Close(); err != nil {
		p.logger.Debug("overlay close failed", "error", err)
	}
	p.backend = nil
	p.synced = false
	p.doneUntil = time.Time{}
}

// sync renders the current voice state and reports whether the overlay is
// animating and should be polled at the faster interval.
func (p *Plugin) sync(status voice.TUIVoiceStatus, live voice.OverlayVoiceStatus, now time.Time) bool {
	if !p.notch {
		// Capsule backends only draw label and color: no text, level or done
		// frames, so level changes never trigger redraws.
		f, visible := frameForStatus(status, live, p.cfg.IdleVisible, false)
		p.apply(frame{State: f.State, Label: f.Label, Color: f.Color}, visible)
		return false
	}
	if out := live.LastOutput; out.Seq != p.doneSeq {
		p.doneSeq = out.Seq
		// A result that lands while a new recording or an error is on screen
		// is consumed silently instead of covering it.
		if status.State == "idle" || status.State == "stopping" {
			p.doneUntil = now.Add(doneHold)
			p.apply(doneFrame(out, p.cfg.ShowText), true)
			return true
		}
	}

	switch status.State {
	case "connecting", "recording", "stopping_delayed", "error":
		p.doneUntil = time.Time{}
	case "idle":
		if now.Before(p.doneUntil) {
			return true
		}
		if live.OutputPending {
			// Text is being pasted or copied; keep the last frame until the
			// result is published so done follows without a hide in between.
			return true
		}
	case "stopping":
		if now.Before(p.doneUntil) {
			return true
		}
	}

	f, visible := frameForStatus(status, live, p.cfg.IdleVisible, p.cfg.ShowText)
	p.apply(f, visible)
	return status.State != "idle" && status.State != "error"
}

func (p *Plugin) apply(f frame, visible bool) {
	if p.synced && f == p.last && visible == p.lastVisible {
		return
	}
	p.synced, p.last, p.lastVisible = true, f, visible

	if !visible {
		if err := p.backend.Hide(); err != nil {
			p.logger.Debug("overlay hide failed", "error", err)
		}
		return
	}
	if err := p.backend.Show(f); err != nil {
		p.logger.Debug("overlay show failed", "error", err)
	}
}

func frameForStatus(status voice.TUIVoiceStatus, live voice.OverlayVoiceStatus, idleVisible, showText bool) (frame, bool) {
	text := ""
	if showText {
		text = live.Partial
	}
	switch status.State {
	case "connecting":
		return frame{State: status.State, Label: "CON", Color: statusColor{R: 245 << 8, G: 190 << 8, B: 70 << 8}, Text: text, Level: quantizeLevel(live.Level)}, true
	case "recording":
		return frame{State: status.State, Label: "REC", Color: statusColor{R: 255 << 8, G: 65 << 8, B: 65 << 8}, Text: text, Level: quantizeLevel(live.Level)}, true
	case "stopping_delayed":
		return frame{State: status.State, Label: "STP", Color: statusColor{R: 255 << 8, G: 140 << 8, B: 60 << 8}, Text: text, Level: quantizeLevel(live.Level)}, true
	case "stopping":
		return frame{State: status.State, Label: "WAI", Color: statusColor{R: 255 << 8, G: 160 << 8, B: 70 << 8}, Text: text}, true
	case "error":
		return frame{State: status.State, Label: "ERR", Color: statusColor{R: 255 << 8, G: 65 << 8, B: 65 << 8}, Detail: status.Detail}, true
	default:
		return frame{State: "idle", Label: "IDL", Color: statusColor{R: 145 << 8, G: 145 << 8, B: 145 << 8}}, idleVisible
	}
}

func doneFrame(out voice.VoiceOutput, showText bool) frame {
	f := frame{State: "done", Label: "END", Color: statusColor{R: 52 << 8, G: 199 << 8, B: 89 << 8}, Detail: out.Detail}
	if showText {
		f.Text = out.Text
	}
	return f
}

func quantizeLevel(level float64) float64 {
	q := math.Round(level*levelSteps) / levelSteps
	return math.Max(0, math.Min(1, q))
}
