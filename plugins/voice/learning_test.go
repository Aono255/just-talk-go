package voice

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c/just-talk-go/config"
	"github.com/c/just-talk-go/engine"
	"github.com/c/just-talk-go/hotkey"
	"github.com/c/just-talk-go/internal/correction"
)

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
			value = "使用 D01。"
			inputMu.Unlock()
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
	r := correction.NewRevision(&correction.Input{}, "P01", "P01")
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
	if r.Stop(true) != nil || p.learningWatch != nil {
		t.Fatal("old revision survived the next paste")
	}
}

func TestDisabledLearningRejectsAnAlreadyReturningModelResponse(t *testing.T) {
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
		p.learnRevision(watch, &correction.Feedback{Draft: "P01", Corrected: "P01", Revised: "D01"})
	}()
	<-entered
	next := *cfg
	next.Correction.Learning = false
	if err := p.OnConfigReload(&next); err != nil {
		t.Fatal(err)
	}
	close(release)
	<-done
	notes, err := correction.LoadMemory(CorrectionMemoryPath())
	if err != nil || len(notes) != 0 {
		t.Fatal("a disabled/stale response created memory")
	}
}
