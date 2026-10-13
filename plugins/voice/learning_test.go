package voice

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c/just-talk-go/config"
	"github.com/c/just-talk-go/engine"
	"github.com/c/just-talk-go/hotkey"
	"github.com/c/just-talk-go/internal/correction"
)

func TestTranslationRevisionMemoryIsScopedAndReusable(t *testing.T) {
	for _, language := range []string{"en", "ja"} {
		t.Run(language, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			original, revised := "Please use D01.", "Use D01."
			if language == "ja" {
				original, revised = "D01を使ってください。", "D01をご利用ください。"
			}
			const raw = "使用 D01。"
			for _, group := range []string{"", "en", "ja"} {
				path, err := InputMemoryPath(group)
				if err != nil {
					t.Fatal(err)
				}
				if err := correction.SaveMemory(path, []string{group + "-existing"}); err != nil {
					t.Fatal(err)
				}
			}
			untouched := map[string][]byte{}
			for _, group := range []string{"", "en", "ja"} {
				if group == language {
					continue
				}
				path, _ := InputMemoryPath(group)
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				untouched[path] = data
			}
			var learned, translated atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct{ Messages []struct{ Content string } }
				if json.NewDecoder(r.Body).Decode(&request) != nil || len(request.Messages) != 2 {
					t.Error("invalid request")
					w.WriteHeader(400)
					return
				}
				var data struct {
					Feedback          *correction.Feedback `json:"feedback"`
					Memory            []string             `json:"memory"`
					CorrectionMemory  []string             `json:"correction_memory"`
					TranslationMemory []string             `json:"translation_memory"`
					Language          string               `json:"target_language"`
				}
				if err := json.Unmarshal([]byte(request.Messages[1].Content), &data); err != nil {
					t.Error(err)
				}
				body := map[string]any{"translated_text": revised}
				if data.Feedback != nil {
					learned.Add(1)
					if data.Feedback.Language != language || data.Feedback.Draft != raw || data.Feedback.Corrected != original || data.Feedback.Revised != revised {
						t.Error("translation feedback lost its recording scope or final output")
					}
					if !reflect.DeepEqual(data.Memory, []string{language + "-existing"}) {
						t.Error("learning received another group's memory")
					}
					body = map[string]any{"memory_updates": []string{language + "-learned"}}
				} else {
					translated.Add(1)
					if data.Language != language || !reflect.DeepEqual(data.CorrectionMemory, []string{"-existing"}) || !reflect.DeepEqual(data.TranslationMemory, []string{language + "-existing", language + "-learned"}) {
						t.Error("next translation did not receive the correct memory groups")
					}
				}
				content, _ := json.Marshal(body)
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(content)}}}})
			}))
			defer server.Close()
			cfg := config.Default()
			cfg.Correction = config.CorrectionConfig{Enabled: true, Learning: true, Provider: "openai-compatible", BaseURL: server.URL, Model: "test", APIKey: "test-only", TimeoutMS: 1000}
			eng := engine.New(hotkey.NewMockProvider(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
			p := NewVoicePlugin()
			if err := eng.LoadPlugin(p); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(eng.Stop)
			now := time.Now()
			unchanged := correction.NewRevision(&correction.Input{}, raw, original, language)
			unchanged.Observe(original, now)
			if unchanged.Confirm(now, original) != nil {
				t.Fatal("automatic translation created feedback without a user revision")
			}
			revision := correction.NewRevision(&correction.Input{}, raw, original, language)
			revision.Observe(original, now)
			revision.Observe(revised, now.Add(time.Millisecond))
			feedback := revision.Confirm(now.Add(2*time.Millisecond), revised)
			if feedback == nil {
				t.Fatal("translation revision was not confirmed")
			}
			// A setting change after paste must not reclassify the earlier revision.
			p.mu.Lock()
			p.cfg.Translation.Language = map[string]string{"en": "ja", "ja": "en"}[language]
			watch := &learningWatch{context: context.Background(), cfg: cfg.Correction, epoch: p.learningEpoch}
			p.mu.Unlock()
			p.learnRevision(watch, feedback)
			for path, before := range untouched {
				after, err := os.ReadFile(path)
				if err != nil || string(before) != string(after) {
					t.Fatal("learning changed an unrelated memory file")
				}
			}
			out, err := correction.Translate(context.Background(), cfg.Correction, raw, language, nil, nil, p.sharedMemory(true, ""), p.sharedMemory(true, language))
			if err != nil || out != revised || learned.Load() != 1 || translated.Load() != 1 {
				t.Fatalf("learning/reuse failed: %q %v %d %d", out, err, learned.Load(), translated.Load())
			}
			if !reflect.DeepEqual(p.sharedMemory(true, ""), []string{"-existing"}) {
				t.Fatal("ordinary correction consumed translation preferences")
			}
		})
	}
}

func TestInputMemoryPathRejectsUnknownGroups(t *testing.T) {
	for _, group := range []string{"fr", "EN", "../en"} {
		if path, err := InputMemoryPath(group); err == nil || path != "" {
			t.Fatalf("accepted invalid group %q", group)
		}
	}
}

func waitLearning(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("learning did not reach the expected state")
}

func TestCrossApplicationRevisionLearnsAndReusesMemoryWithoutChat(t *testing.T) {
	for _, correctFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary_input", true: "input_only_correction"}[correctFirst], func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			const note = "该平台语境中的 P01 应为 D01"
			var learned, corrections, withMemory atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct{ Messages []struct{ Content string } }
				if json.NewDecoder(r.Body).Decode(&request) != nil || len(request.Messages) != 2 {
					t.Error("invalid model request")
					w.WriteHeader(400)
					return
				}
				var data struct {
					Feedback *correction.Feedback
					Context  []correction.Message
					Memory   []string
				}
				if json.Unmarshal([]byte(request.Messages[1].Content), &data) != nil {
					t.Error("invalid payload")
				}
				if len(data.Context) != 0 {
					t.Error("generic input read chat history")
				}
				body := map[string]any{"corrected_text": "使用 P01。"}
				if data.Feedback != nil {
					learned.Add(1)
					if data.Feedback.Revised != "使用 D01。" || data.Feedback.Corrected != "使用 P01。" {
						t.Error("wrong revision")
					}
					body = map[string]any{"memory_updates": []string{note}}
				} else {
					corrections.Add(1)
					if len(data.Memory) == 1 && data.Memory[0] == note {
						withMemory.Add(1)
					}
				}
				content, _ := json.Marshal(body)
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(content)}}}})
			}))
			defer server.Close()
			cfg := config.Default()
			cfg.Correction = config.CorrectionConfig{Enabled: correctFirst, Context: false, Learning: true, Provider: "openai-compatible", BaseURL: server.URL, Model: "test", APIKey: "test-only", TimeoutMS: 1000}
			eng := engine.New(hotkey.NewMockProvider(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
			p := NewVoicePlugin()
			if err := eng.LoadPlugin(p); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(eng.Stop)
			var inputMu sync.Mutex
			value := ""
			var closed atomic.Int32
			p.correctionCapture = func() (*correction.Target, error) {
				t.Error("learning depended on a chat adapter")
				return nil, correction.ErrUnsupportedContext
			}
			p.correctionInputCapture = func() (*correction.Input, error) {
				return &correction.Input{Read: func() (string, error) { inputMu.Lock(); defer inputMu.Unlock(); return value, nil },
					Guard: func() error { return nil }, Close: func() { closed.Add(1) }}, nil
			}
			p.correctionClipboard = func(string) error { return nil }
			p.correctionDestinationCapture = func() (*correction.Destination, error) {
				return &correction.Destination{Guard: func() error { return nil }, Close: func() {}}, nil
			}
			p.correctionPaste = func(text string, guard func() error) error {
				if err := guard(); err != nil {
					return err
				}
				inputMu.Lock()
				value = text
				inputMu.Unlock()
				return nil
			}
			p.mu.Lock()
			p.sessionID = 1
			p.mu.Unlock()
			p.outputTranscript(&recordingSession{sessionID: 1, autoSubmit: true, correction: cfg.Correction}, "使用 P01。")
			waitLearning(t, func() bool {
				p.mu.Lock()
				w := p.learningWatch
				p.mu.Unlock()
				return w != nil && w.revision.Observed()
			})
			if learned.Load() != 0 {
				t.Fatal("unedited output generated memory")
			}
			inputMu.Lock()
			value = "中间修改，尚未完成。"
			inputMu.Unlock()
			time.Sleep(1700 * time.Millisecond)
			if learned.Load() != 0 {
				t.Fatal("idle pause created memory")
			}
			p.onLearningEnter(hotkey.Event{Combo: hotkey.Combo{Mods: hotkey.ModShift, Key: hotkey.KeyEnter}, Type: hotkey.KeyDown, Time: time.Now()})
			if learned.Load() != 0 {
				t.Fatal("Shift Enter created memory")
			}
			inputMu.Lock()
			value = "使用 D01。"
			inputMu.Unlock()
			time.Sleep(120 * time.Millisecond)
			at := time.Now()
			// Model an application clearing immediately before event dispatch.
			inputMu.Lock()
			value = ""
			inputMu.Unlock()
			p.onLearningEnter(hotkey.Event{Combo: confirmLearningCombo, Type: hotkey.KeyDown, Time: at})
			waitLearning(t, func() bool {
				notes, err := correction.LoadMemory(CorrectionMemoryPath())
				return err == nil && len(notes) == 1 && notes[0] == note
			})
			waitLearning(t, func() bool { return closed.Load() == 1 })
			if learned.Load() != 1 {
				t.Fatal("one revision should make one learning request")
			}
			next := *cfg
			next.Correction.Enabled = true
			if err := p.OnConfigReload(&next); err != nil {
				t.Fatal(err)
			}
			p.mu.Lock()
			p.sessionID = 2
			p.mu.Unlock()
			// Clipboard mode still reuses memory, but does not watch arbitrary
			// later manual pastes into another application.
			p.outputTranscript(&recordingSession{sessionID: 2, correction: next.Correction}, "使用 P01。")
			if withMemory.Load() != 1 {
				t.Fatal("shared memory was not used without chat context")
			}
			if correctFirst && corrections.Load() != 2 {
				t.Fatal("unexpected correction requests")
			}
		})
	}
}

func TestProgrammaticPasteCannotBecomePriorInputRevision(t *testing.T) {
	p := NewVoicePlugin()
	r := correction.NewRevision(&correction.Input{}, "P01", "P01", "")
	r.Observe("P01", time.Now())
	p.learningWatch = &learningWatch{revision: r, cancel: func() {}}
	p.correctionPaste = func(text string, guard func() error) error {
		if r.Observe(text, time.Now()) {
			t.Fatal("programmatic paste was accepted as a user edit")
		}
		return nil
	}
	if err := p.pasteTranscript("D01", nil, nil); err != nil {
		t.Fatal(err)
	}
	if r.Confirm(time.Now(), "D01") != nil || p.learningWatch != nil {
		t.Fatal("old revision survived the next paste")
	}
}

func TestLearningEnterRejectsModifiersOldEventsAndKeyUp(t *testing.T) {
	p := NewVoicePlugin()
	now := time.Now()
	w := &learningWatch{started: now, confirm: make(chan time.Time, 1)}
	p.learningWatch = w
	for _, mods := range []hotkey.Modifier{hotkey.ModShift, hotkey.ModCtrl, hotkey.ModAlt, hotkey.ModSuper} {
		p.onLearningEnter(hotkey.Event{Combo: hotkey.Combo{Key: hotkey.KeyEnter, Mods: mods}, Type: hotkey.KeyDown, Time: now})
	}
	p.onLearningEnter(hotkey.Event{Combo: confirmLearningCombo, Type: hotkey.KeyUp, Time: now})
	p.onLearningEnter(hotkey.Event{Combo: confirmLearningCombo, Type: hotkey.KeyDown, Time: now.Add(-time.Second)})
	if len(w.confirm) != 0 {
		t.Fatal("invalid confirmation accepted")
	}
	p.onLearningEnter(hotkey.Event{Combo: confirmLearningCombo, Type: hotkey.KeyDown, Time: now})
	if len(w.confirm) != 1 {
		t.Fatal("plain Enter missing")
	}
}

func TestAbandonedOrClearedRevisionNeverStartsLearning(t *testing.T) {
	for _, lostFocus := range []bool{false, true} {
		p := NewVoicePlugin()
		ctx, cancel := context.WithCancel(context.Background())
		r := correction.NewRevision(&correction.Input{}, "P01", "P01", "")
		r.Observe("P01", time.Now())
		r.Observe("D01", time.Now())
		w := &learningWatch{revision: r, cancel: cancel, context: ctx, confirm: make(chan time.Time, 1)}
		p.learningWatch = w
		closed := false
		input := &correction.Input{Read: func() (string, error) {
			if lostFocus {
				return "", errors.New("focus changed")
			}
			return "", nil
		}, Close: func() { closed = true }}
		// No model environment is installed: any accidental learning call is
		// a test failure instead of making a network request.
		p.watchRevision(ctx, w, input)
		if !closed || p.learningWatch != nil || r.Confirm(time.Now(), "D01") != nil {
			t.Fatal("abandoned revision survived")
		}
	}
}

func TestDisabledLearningRejectsAnAlreadyReturningModelResponse(t *testing.T) {
	for _, language := range []string{"", "en", "ja"} {
		t.Run(MemoryGroupName(language), func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(entered)
				<-release
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": `{"memory_updates":["过期回复不得写入"]}`}}}})
			}))
			defer server.Close()
			cfg := config.Default()
			cfg.Correction = config.CorrectionConfig{Learning: true, Provider: "openai-compatible", BaseURL: server.URL, Model: "test", APIKey: "test-only", TimeoutMS: 1000}
			eng := engine.New(hotkey.NewMockProvider(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
			p := NewVoicePlugin()
			if err := eng.LoadPlugin(p); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(eng.Stop)
			// This response deliberately ignores the ordinary cancellation context:
			// committing still has to check the generation under the commit lock.
			watch := &learningWatch{context: context.Background(), cfg: cfg.Correction, epoch: p.learningEpoch}
			go func() {
				defer close(done)
				p.learnRevision(watch, &correction.Feedback{Draft: "P01", Corrected: "P01", Revised: "D01", Language: language})
			}()
			<-entered
			next := *cfg
			next.Correction.Learning = false
			if err := p.OnConfigReload(&next); err != nil {
				t.Fatal(err)
			}
			close(release)
			<-done
			path, _ := InputMemoryPath(language)
			notes, err := correction.LoadMemory(path)
			if err != nil || len(notes) != 0 {
				t.Fatal("a disabled/stale response created memory")
			}
		})
	}
}
