package overlay

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/c/just-talk-go/config"
	"github.com/c/just-talk-go/plugins/voice"
)

type recordingBackend struct {
	calls []frame // Hide is recorded as frame{State: "hide"}
}

func (b *recordingBackend) Show(f frame) error { b.calls = append(b.calls, f); return nil }
func (b *recordingBackend) Hide() error        { b.calls = append(b.calls, frame{State: "hide"}); return nil }
func (b *recordingBackend) Close() error       { return nil }

func (b *recordingBackend) states() []string {
	states := make([]string, len(b.calls))
	for i, f := range b.calls {
		states[i] = f.State
	}
	return states
}

func newTestPlugin(showText bool) (*Plugin, *recordingBackend) {
	b := &recordingBackend{}
	return &Plugin{
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		cfg:     config.OverlayConfig{Enabled: true, ShowText: showText},
		notch:   true,
		backend: b,
	}, b
}

func status(state string) voice.TUIVoiceStatus { return voice.TUIVoiceStatus{State: state} }

func assertStates(t *testing.T, b *recordingBackend, want ...string) {
	t.Helper()
	got := b.states()
	if len(got) != len(want) {
		t.Fatalf("backend calls = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("backend calls = %v, want %v", got, want)
		}
	}
}

func TestDoneShownOnceAndHeldBeforeHide(t *testing.T) {
	p, b := newTestPlugin(true)
	t0 := time.Now()
	tick := func(d time.Duration, state string, live voice.OverlayVoiceStatus) {
		p.sync(status(state), live, t0.Add(d))
	}

	tick(0, "recording", voice.OverlayVoiceStatus{Partial: "你好", Level: 0.42})
	tick(50*time.Millisecond, "stopping", voice.OverlayVoiceStatus{Partial: "你好世界"})
	// Session finished but paste still running: the stopping frame stays up.
	tick(100*time.Millisecond, "idle", voice.OverlayVoiceStatus{Partial: "你好世界", OutputPending: true})
	out := voice.VoiceOutput{Seq: 1, Text: "你好世界。", Detail: "pasted"}
	done := voice.OverlayVoiceStatus{Partial: "你好世界", LastOutput: out}
	tick(150*time.Millisecond, "idle", done)
	tick(200*time.Millisecond, "idle", done)
	tick(900*time.Millisecond, "idle", done)
	assertStates(t, b, "recording", "stopping", "done")

	if f := b.calls[0]; f.Text != "你好" || f.Level != 0.4 {
		t.Fatalf("recording frame = %+v, want partial text and level quantized to 0.4", f)
	}
	if f := b.calls[2]; f.Text != out.Text || f.Detail != "pasted" {
		t.Fatalf("done frame = %+v, want final text with pasted detail", f)
	}

	tick(950*time.Millisecond, "idle", done)
	tick(time.Second, "idle", done)
	assertStates(t, b, "recording", "stopping", "done", "hide")
}

func TestNoDoneWithoutOutput(t *testing.T) {
	p, b := newTestPlugin(true)
	t0 := time.Now()
	p.sync(status("recording"), voice.OverlayVoiceStatus{}, t0)
	p.sync(status("stopping"), voice.OverlayVoiceStatus{}, t0.Add(50*time.Millisecond))
	// Cancelled or empty sessions never publish an output.
	p.sync(status("idle"), voice.OverlayVoiceStatus{}, t0.Add(100*time.Millisecond))
	assertStates(t, b, "recording", "stopping", "hide")
}

func TestOutputDuringNewRecordingIsNotShown(t *testing.T) {
	p, b := newTestPlugin(true)
	t0 := time.Now()
	p.sync(status("recording"), voice.OverlayVoiceStatus{}, t0)
	late := voice.OverlayVoiceStatus{LastOutput: voice.VoiceOutput{Seq: 1, Text: "old", Detail: "copied"}}
	p.sync(status("recording"), late, t0.Add(50*time.Millisecond))
	p.sync(status("idle"), late, t0.Add(100*time.Millisecond))
	assertStates(t, b, "recording", "hide")
}

func TestShowTextDisabledSendsNoText(t *testing.T) {
	p, b := newTestPlugin(false)
	t0 := time.Now()
	p.sync(status("recording"), voice.OverlayVoiceStatus{Partial: "secret"}, t0)
	p.sync(status("idle"), voice.OverlayVoiceStatus{LastOutput: voice.VoiceOutput{Seq: 1, Text: "secret", Detail: "copied"}}, t0.Add(50*time.Millisecond))
	assertStates(t, b, "recording", "done")
	for _, f := range b.calls {
		if f.Text != "" {
			t.Fatalf("frame %+v carries text with show_text disabled", f)
		}
	}
}

func TestCapsuleModeKeepsOriginalBehavior(t *testing.T) {
	p, b := newTestPlugin(true)
	p.notch = false
	t0 := time.Now()
	fast := p.sync(status("recording"), voice.OverlayVoiceStatus{Partial: "\u4f60\u597d", Level: 0.2}, t0)
	// Level and text changes must not redraw capsule backends.
	p.sync(status("recording"), voice.OverlayVoiceStatus{Partial: "\u4f60\u597d\u4e16\u754c", Level: 0.9}, t0.Add(100*time.Millisecond))
	// No hold for pending output and no done frame: hide right away.
	p.sync(status("idle"), voice.OverlayVoiceStatus{OutputPending: true}, t0.Add(200*time.Millisecond))
	p.sync(status("idle"), voice.OverlayVoiceStatus{LastOutput: voice.VoiceOutput{Seq: 1, Text: "x", Detail: "pasted"}}, t0.Add(300*time.Millisecond))
	assertStates(t, b, "recording", "hide")
	if f := b.calls[0]; f.Text != "" || f.Level != 0 || f.Label != "REC" {
		t.Fatalf("capsule frame = %+v, want label and color only", f)
	}
	if fast {
		t.Fatal("capsule mode requested fast polling")
	}
	for _, goos := range []string{"linux", "windows"} {
		if notchMode(goos, "notch") {
			t.Fatalf("notchMode(%q, notch) = true, want capsule", goos)
		}
	}
}

// backendFactory opens recording backends and tracks how many are alive, so
// tests can catch leaked or doubled helpers.
type backendFactory struct {
	mu      sync.Mutex
	opened  []config.OverlayConfig
	live    int
	maxLive int
	last    *recordingBackend
}

type factoryBackend struct {
	*recordingBackend
	f      *backendFactory
	closed bool
}

func (b *factoryBackend) Close() error {
	b.f.mu.Lock()
	defer b.f.mu.Unlock()
	if !b.closed {
		b.closed = true
		b.f.live--
	}
	return nil
}

func (f *backendFactory) open(cfg config.OverlayConfig) (backend, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opened = append(f.opened, cfg)
	f.live++
	f.maxLive = max(f.maxLive, f.live)
	f.last = &recordingBackend{}
	return &factoryBackend{recordingBackend: f.last, f: f}, nil
}

func (f *backendFactory) snapshot() (positions []string, live, maxLive int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, cfg := range f.opened {
		positions = append(positions, cfg.Position)
	}
	return positions, f.live, f.maxLive
}

func TestReconfigureRebuildsOnlyForPositionAndScale(t *testing.T) {
	f := &backendFactory{}
	cfg := config.OverlayConfig{Enabled: true, Position: "bottom-center", Scale: 1}
	p := &Plugin{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), cfg: cfg, open: f.open}
	p.openBackend()
	t0 := time.Now()
	p.sync(status("recording"), voice.OverlayVoiceStatus{}, t0)

	// Display options reuse the backend and apply on the next frame.
	cfg.IdleVisible, cfg.ShowText = true, true
	p.reconfigure(cfg)
	p.sync(status("idle"), voice.OverlayVoiceStatus{}, t0.Add(100*time.Millisecond))
	if positions, _, _ := f.snapshot(); len(positions) != 1 {
		t.Fatalf("display-only reload opened backends %v, want the original only", positions)
	}
	assertStates(t, f.last, "recording", "idle")

	// A new position needs a new backend, which gets the current state again
	// even though the frame itself did not change.
	cfg.Position = "top-right"
	p.reconfigure(cfg)
	p.sync(status("idle"), voice.OverlayVoiceStatus{}, t0.Add(200*time.Millisecond))
	assertStates(t, f.last, "idle")

	cfg.Scale = 1.5
	p.reconfigure(cfg)
	cfg.Enabled = false
	p.reconfigure(cfg)
	if p.backend != nil {
		t.Fatal("disabled overlay kept its backend")
	}
	cfg.Enabled = true
	p.reconfigure(cfg)

	positions, live, maxLive := f.snapshot()
	want := []string{"bottom-center", "top-right", "top-right", "top-right"}
	if len(positions) != len(want) {
		t.Fatalf("opened backends %v, want %v", positions, want)
	}
	for i := range want {
		if positions[i] != want[i] {
			t.Fatalf("opened backends %v, want %v", positions, want)
		}
	}
	if live != 1 || maxLive != 1 {
		t.Fatalf("live backends = %d (max %d), want exactly one at a time", live, maxLive)
	}
}

func TestStartAppliesReloadsAndClosesOnCancel(t *testing.T) {
	f := &backendFactory{}
	p := NewOverlayPlugin()
	p.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	p.open = f.open
	p.cfg = config.OverlayConfig{Enabled: false, Position: "bottom-center", Scale: 1}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.Start(ctx)
	}()

	// Reloads return immediately, even back to back; the newest one wins.
	for _, position := range []string{"bottom-left", "top-left", "top-center"} {
		next := config.Default()
		next.Overlay = config.OverlayConfig{Enabled: true, Position: position, Scale: 1}
		if err := p.OnConfigReload(next); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		positions, live, _ := f.snapshot()
		if live == 1 && len(positions) > 0 && positions[len(positions)-1] == "top-center" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("overlay never enabled at top-center: opened %v, live %d", positions, live)
		}
		time.Sleep(5 * time.Millisecond)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after cancel")
	}
	if _, live, maxLive := f.snapshot(); live != 0 || maxLive != 1 {
		t.Fatalf("after cancel live backends = %d (max %d), want 0 (max 1)", live, maxLive)
	}
}
