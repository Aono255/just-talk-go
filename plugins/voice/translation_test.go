package voice

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c/just-talk-go/config"
	"github.com/c/just-talk-go/engine"
	"github.com/c/just-talk-go/hotkey"
	"github.com/c/just-talk-go/internal/correction"
)

func TestCancelDuringTranslationNeverPastesLateResult(t *testing.T) {
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": `{"translated_text":"Late result"}`}}}})
	}))
	defer server.Close()
	cfg := config.Default()
	cfg.Correction = config.CorrectionConfig{Provider: "openai-compatible", BaseURL: server.URL, Model: "test", APIKey: "test-only", TimeoutMS: 2000}
	p := NewVoicePlugin()
	eng := engine.New(hotkey.NewMockProvider(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := eng.LoadPlugin(p); err != nil {
		t.Fatal(err)
	}
	defer eng.Stop()
	p.sessionID = 1
	p.pendingDone = 1
	p.finishingSessions = map[uint64]struct{}{1: {}}
	p.correctionDestinationCapture = func() (*correction.Destination, error) {
		return &correction.Destination{Guard: func() error { return nil }, Close: func() {}}, nil
	}
	p.correctionInputCapture = func() (*correction.Input, error) { return nil, correction.ErrUnsupportedInput }
	p.correctionClipboard = func(string) error { return nil }
	var pasted atomic.Int32
	p.correctionPaste = func(string, func() error) error { pasted.Add(1); return nil }
	go func() {
		defer close(done)
		p.outputTranscript(&recordingSession{sessionID: 1, autoSubmit: true, correction: cfg.Correction, translation: "ja"}, "测试取消翻译")
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("translation did not start")
	}
	p.cancelRecording()
	close(release)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("translation cancellation blocked")
	}
	if pasted.Load() != 0 {
		t.Fatal("canceled translation pasted a late result")
	}
}

func TestTranslationPromotesOnePendingMicrophoneAndKeepsLanguageSnapshot(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS modifier snapshots")
	}
	cfg := config.Default()
	cfg.Voice.Mode = "hold"
	cfg.Translation.Enabled = true
	cfg.Correction.APIKey = "test-only"
	p := NewVoicePlugin()
	eng := engine.New(hotkey.NewMockProvider(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	entered, release := make(chan struct{}), make(chan struct{})
	var opens atomic.Int32
	p.recorderStart = func(*Recorder) error {
		opens.Add(1)
		close(entered)
		<-release
		return errors.New("synthetic stopped startup")
	}
	if err := eng.LoadPlugin(p); err != nil {
		t.Fatal(err)
	}
	defer func() {
		eng.Stop()
		close(release)
		waitStartState(t, p, func(p *VoicePlugin) bool { return !p.starting })
	}()
	base := hotkey.ModAlt | hotkey.ModSuper
	extra := base | hotkey.ModShift
	p.handleHotkey(hotkey.Event{Combo: hotkey.Combo{Mods: base}, Type: hotkey.KeyDown, Sequence: 1, Modifiers: base})
	<-entered
	next := *cfg
	next.Translation.Language = "ja"
	if err := p.OnConfigReload(&next); err != nil {
		t.Fatal(err)
	}
	p.handleHotkey(hotkey.Event{Combo: hotkey.Combo{Mods: extra}, Type: hotkey.KeyDown, Sequence: 2, Modifiers: extra, ModifierHistory: extra})
	p.mu.Lock()
	good := p.sessionID == 1 && p.starting && p.sessionTranslate && p.sessionTranslation.Language == "en"
	p.mu.Unlock()
	if !good || opens.Load() != 1 {
		t.Fatal("extra modifier reopened microphone or changed in-flight language")
	}
	p.handleHotkey(hotkey.Event{Combo: hotkey.Combo{Mods: extra}, Type: hotkey.KeyUp, Sequence: 3, Modifiers: base, ModifierHistory: extra})
	p.mu.Lock()
	released := p.holdReleased
	p.mu.Unlock()
	if released {
		t.Fatal("releasing Shift alone stopped the held recording")
	}
	p.handleHotkey(hotkey.Event{Combo: hotkey.Combo{Mods: base}, Type: hotkey.KeyUp, Sequence: 4, Modifiers: hotkey.ModSuper, ModifierHistory: extra})
}

func TestTranslationChordAllPressAndReleaseOrders(t *testing.T) {
	keys := []hotkey.Modifier{hotkey.ModAlt, hotkey.ModSuper, hotkey.ModShift}
	orders := [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	base := keys[0] | keys[1]
	extra := base | keys[2]
	for _, mode := range []string{"hold", "toggle"} {
		for _, press := range orders {
			for _, release := range orders {
				c := translationChord{}
				mods := hotkey.ModNone
				seq := uint64(0)
				starts, stops := 0, 0
				translated := false
				apply := func() {
					seq++
					evt := hotkey.Event{Sequence: seq, Modifiers: mods}
					a := c.step(evt, base, extra, mode, starts > 0)
					if a.emit {
						if a.edge == hotkey.KeyDown {
							starts++
						} else {
							stops++
						}
					}
					translated = translated || a.translate || a.promote
					if duplicate := c.step(evt, base, extra, mode, true); duplicate.emit || duplicate.promote {
						t.Fatal("same native event dispatched twice")
					}
				}
				for _, i := range press {
					mods |= keys[i]
					apply()
				}
				for _, i := range release {
					mods &^= keys[i]
					apply()
				}
				if starts != 1 || (mode == "hold" && stops != 1) || (mode == "toggle" && stops != 0) || !translated {
					t.Fatalf("%s %v/%v: starts=%d stops=%d translated=%v", mode, press, release, starts, stops, translated)
				}
			}
		}
	}
	c := translationChord{}
	c.step(hotkey.Event{Sequence: 1, Modifiers: base}, base, extra, "hold", false)
	// The base release can overtake the extra-key notification in another
	// dispatch goroutine. Native hold history still promotes before stopping.
	a := c.step(hotkey.Event{Sequence: 3, Modifiers: hotkey.ModSuper, ModifierHistory: extra}, base, extra, "hold", true)
	if !a.promote || !a.emit || a.edge != hotkey.KeyUp {
		t.Fatal("overtaken translation press lost")
	}
	if late := c.step(hotkey.Event{Sequence: 2, Modifiers: extra}, base, extra, "hold", false); late.emit || late.promote {
		t.Fatal("late event restarted recording")
	}
	normal := c.step(hotkey.Event{Sequence: 4, Modifiers: base, ModifierHistory: base}, base, extra, "hold", false)
	if !normal.emit || normal.translate || normal.promote {
		t.Fatal("translation leaked into next ordinary input")
	}
	p := NewVoicePlugin()
	p.cfg = config.Default()
	p.combo = hotkey.Combo{Mods: base}
	if !p.handleTranslationChord(hotkey.Event{Combo: hotkey.Combo{Mods: extra}, Sequence: 5, Modifiers: extra, Type: hotkey.KeyDown}) {
		t.Fatal("unregistered translation notification reached ordinary recorder")
	}
}

func TestTranslationPipelineOnlyPastesFinalLanguageAndKeepsGuards(t *testing.T) {
	for _, mode := range []string{"correct-en", "correct-ja", "raw-ja", "empty", "empty-changed", "guard-changed", "model-failure"} {
		t.Run(mode, func(t *testing.T) {
			raw := "那个 D01 检查一下"
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			language := "en"
			want := "Please check D01."
			enabled := mode != "raw-ja"
			if mode == "raw-ja" || mode == "correct-ja" {
				language = "ja"
				want = "D01 を確認してください。"
			}
			var calls []string
			var callsMu sync.Mutex
			changed := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct{ Messages []struct{ Content string } }
				json.NewDecoder(r.Body).Decode(&request)
				var payload map[string]any
				json.Unmarshal([]byte(request.Messages[1].Content), &payload)
				callsMu.Lock()
				calls = append(calls, "translate")
				callsMu.Unlock()
				if payload["target_language"] != language || payload["draft"] != raw || payload["correct_asr"] != enabled {
					t.Error("combined request lost raw text, language or correction switch")
				}
				if _, ok := payload["text"]; ok {
					t.Error("unexpected intermediate corrected text")
				}
				if mode == "model-failure" {
					w.WriteHeader(503)
					return
				}
				if payload["translation_memory"].([]any)[0] != language+"-only" {
					t.Error("wrong translation memory group")
				}
				if enabled {
					if payload["correction_memory"].([]any)[0] != "input-only" {
						t.Error("ordinary correction memory missing")
					}
				} else if _, ok := payload["correction_memory"]; ok {
					t.Error("disabled correction received ordinary memory")
				}
				body := map[string]string{"translated_text": want}
				data, _ := json.Marshal(body)
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(data)}}}})
			}))
			defer server.Close()
			cfg := config.Default()
			cfg.Correction = config.CorrectionConfig{Enabled: enabled, Context: mode == "empty" || mode == "empty-changed", Learning: true, Provider: "openai-compatible", BaseURL: server.URL, Model: "test", APIKey: "test-only", TimeoutMS: 1000}
			eng := engine.New(hotkey.NewMockProvider(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
			p := NewVoicePlugin()
			if err := eng.LoadPlugin(p); err != nil {
				t.Fatal(err)
			}
			defer eng.Stop()
			p.sessionID = 1
			for _, group := range []string{"", "en", "ja"} {
				path, err := InputMemoryPath(group)
				if err != nil {
					t.Fatal(err)
				}
				note := group + "-only"
				if group == "" {
					note = "input-only"
				}
				if err := correction.SaveMemory(path, []string{note}); err != nil {
					t.Fatal(err)
				}
			}
			p.correctionInputCapture = func() (*correction.Input, error) { return nil, correction.ErrUnsupportedInput }
			p.correctionDestinationCapture = func() (*correction.Destination, error) {
				return &correction.Destination{Guard: func() error {
					if changed {
						return errors.New("destination changed")
					}
					return nil
				}, Close: func() {}}, nil
			}
			if cfg.Correction.Context {
				p.correctionCapture = func() (*correction.Target, error) {
					return &correction.Target{Source: correction.Source{ID: correction.CodexDesktopSourceID, Name: "Codex Desktop"}, Messages: []correction.Message{}, Close: func() {}, Guard: func() error {
						if changed {
							return errors.New("empty conversation changed")
						}
						return nil
					}}, nil
				}
			}
			clipboard := ""
			p.correctionClipboard = func(s string) error { clipboard = s; return nil }
			var pasted []string
			p.correctionPaste = func(s string, guard func() error) error {
				if mode == "guard-changed" || mode == "empty-changed" {
					changed = true
				}
				if err := guard(); err != nil {
					return err
				}
				pasted = append(pasted, s)
				return nil
			}
			p.outputTranscript(&recordingSession{sessionID: 1, autoSubmit: true, correction: cfg.Correction, translation: language}, raw)
			callsMu.Lock()
			defer callsMu.Unlock()
			wantCalls := []string{"translate"}
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatalf("calls %v", calls)
			}
			if (mode == "guard-changed" || mode == "empty-changed") || mode == "model-failure" {
				if len(pasted) != 0 || clipboard != raw {
					t.Fatal("failure pasted or lost original")
				}
			} else if !reflect.DeepEqual(pasted, []string{want}) {
				t.Fatalf("pasted %v", pasted)
			}
			if len(p.translatingSessions) != 0 || len(p.correctionCancels) != 0 {
				t.Fatal("processing state leaked")
			}
		})
	}
}
