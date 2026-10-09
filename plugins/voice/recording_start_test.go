package voice

import (
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c/just-talk-go/config"
	"github.com/c/just-talk-go/engine"
	"github.com/c/just-talk-go/hotkey"
)

// Reproduce a driver that stays inside Start until after the user releases
// the hotkey. No microphone, network connection, or credentials are used.
func TestBlockedMicrophoneStartKeepsHotkeysResponsive(t *testing.T) {
	for _, mode := range []string{"hold", "toggle", "failure"} {
		t.Run(mode, func(t *testing.T) {
			cfg := config.Default()
			cfg.Voice.Mode = mode
			if mode == "failure" {
				cfg.Voice.Mode = "hold"
			}
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			eng := engine.New(hotkey.NewMockProvider(), cfg, logger)
			plugin := NewVoicePlugin()
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			var opens, closes atomic.Int32
			plugin.recorderStart = func(rec *Recorder) error {
				opens.Add(1)
				close(entered)
				<-release
				if mode == "failure" {
					return errors.New("simulated microphone failure")
				}
				rec.started = true
				rec.backend = "test"
				rec.stdout = io.NopCloser(strings.NewReader(""))
				rec.stopFunc = func() error { closes.Add(1); return nil }
				return nil
			}
			if err := eng.LoadPlugin(plugin); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				eng.Stop()
				releaseOnce.Do(func() { close(release) })
				waitStartState(t, plugin, func(p *VoicePlugin) bool { return !p.starting })
				plugin.mu.Lock()
				plugin.clearErrorLocked()
				plugin.mu.Unlock()
			})

			plugin.onHotkey(hotkey.Event{Type: hotkey.KeyDown})
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("microphone start worker did not run")
			}
			if status := TUIStatus(); status.Detail != "正在打开麦克风" || status.Recording {
				t.Fatalf("opening was presented as idle or recording: %+v", status)
			}

			if mode == "toggle" {
				plugin.onHotkey(hotkey.Event{Type: hotkey.KeyDown})
				waitStartState(t, plugin, func(p *VoicePlugin) bool { return p.startCanceled })
			} else {
				plugin.onHotkey(hotkey.Event{Type: hotkey.KeyUp})
				waitStartState(t, plugin, func(p *VoicePlugin) bool { return p.holdReleased })
				// Repeated edges must be processed promptly without opening more queues.
				if mode == "hold" {
					plugin.onHotkey(hotkey.Event{Type: hotkey.KeyDown})
					waitStartState(t, plugin, func(p *VoicePlugin) bool { return !p.holdReleased })
					plugin.onHotkey(hotkey.Event{Type: hotkey.KeyUp})
					waitStartState(t, plugin, func(p *VoicePlugin) bool { return p.holdReleased })
				}
			}
			if opens.Load() != 1 {
				t.Fatal("blocked start created duplicate microphone queues")
			}
			releaseOnce.Do(func() { close(release) })
			waitStartState(t, plugin, func(p *VoicePlugin) bool { return !p.starting })
			plugin.mu.Lock()
			defer plugin.mu.Unlock()
			if plugin.recording || plugin.recorder != nil || plugin.asrClient != nil || plugin.asrCancel != nil {
				t.Fatal("released/canceled startup began a late recording or ASR connection")
			}
			if mode == "failure" {
				if !strings.Contains(plugin.lastError, "simulated microphone failure") {
					t.Fatal("native start error was lost after key release")
				}
			} else if closes.Load() != 1 {
				t.Fatal("canceled startup did not release its microphone")
			}
		})
	}
}

func TestExpiredStopTimerCannotStopNewOrResumedRecording(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	eng := engine.New(hotkey.NewMockProvider(), config.Default(), logger)
	plugin := NewVoicePlugin()
	if err := eng.LoadPlugin(plugin); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(eng.Stop)
	rec := NewRecorder(logger, 1)
	plugin.recorder, plugin.recording, plugin.stopping, plugin.sessionGen = rec, true, true, 2
	plugin.stopRecordingAsync(1)
	if !plugin.recording || plugin.recorder != rec {
		t.Fatal("old stop timer detached a newer recording")
	}
	plugin.stopping = false
	plugin.stopRecordingAsync(2)
	if !plugin.recording || plugin.recorder != rec {
		t.Fatal("canceled stop timer detached a resumed recording")
	}
}

func waitStartState(t *testing.T, plugin *VoicePlugin, ready func(*VoicePlugin) bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if plugin.mu.TryLock() {
			ok := ready(plugin)
			plugin.mu.Unlock()
			if ok {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("hotkey/state update blocked while the microphone was opening")
}
