package voice

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c/just-talk-go/config"
	"github.com/c/just-talk-go/engine"
	"github.com/c/just-talk-go/hotkey"
	"github.com/c/just-talk-go/internal/correction"
)

func TestUnreadableInputStillCorrectsUsingBoundWindowButDoesNotLearn(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "same_window", true: "changed_window"}[changed], func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			var moved atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct{ Messages []struct{ Content string } }
				if json.NewDecoder(r.Body).Decode(&req) != nil {
					t.Error("invalid request")
				}
				var data struct{ Context []correction.Message }
				if len(req.Messages) != 2 || json.Unmarshal([]byte(req.Messages[1].Content), &data) != nil || len(data.Context) != 0 {
					t.Error("window-only correction acquired chat data")
				}
				moved.Store(changed)
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": `{"corrected_text":"使用 D01。"}`}}}})
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
			p.correctionInputCapture = func() (*correction.Input, error) { return nil, errors.New("AX -25212") }
			p.correctionDestinationCapture = func() (*correction.Destination, error) {
				return &correction.Destination{Guard: func() error {
					if moved.Load() {
						return errors.New("window changed")
					}
					return nil
				}, Close: func() {}}, nil
			}
			pasted := ""
			p.correctionClipboard = func(string) error { return nil }
			p.correctionPaste = func(text string, guard func() error) error {
				if err := guard(); err != nil {
					return err
				}
				pasted = text
				return nil
			}
			p.sessionID = 1
			p.outputTranscript(&recordingSession{sessionID: 1, autoSubmit: true, correction: cfg.Correction}, "使用 P01。")
			if (!changed && pasted != "使用 D01。") || (changed && pasted != "") {
				t.Fatal("window guard/output mismatch")
			}
			p.mu.Lock()
			watch := p.learningWatch
			p.mu.Unlock()
			if watch != nil {
				t.Fatal("missing input was used to learn revisions")
			}
		})
	}
}

func TestCorrectionRoutingKeepsOtherAppsUsableAndReportsCodexFocusErrors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		captureErr error
		wantError  bool
	}{
		{"other_app", correction.ErrUnsupportedContext, false},
		{"codex_without_draft", errors.New("请把光标放在 Codex 草稿框；未识别到同一聊天主区域"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			cfg := config.Default()
			cfg.Correction = config.CorrectionConfig{Provider: "openai-compatible", Enabled: true, Context: true, BaseURL: server.URL, Model: "small-test", APIKey: "test-key", TimeoutMS: 1000}
			eng := engine.New(hotkey.NewMockProvider(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
			p := NewVoicePlugin()
			if err := eng.LoadPlugin(p); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(eng.Stop)
			p.sessionID = 7
			p.correctionCapture = func() (*correction.Target, error) { return nil, tc.captureErr }
			p.correctionInputCapture = func() (*correction.Input, error) { return nil, correction.ErrUnsupportedInput }
			p.correctionDestinationCapture = func() (*correction.Destination, error) { return nil, correction.ErrUnsupportedInput }
			copied := make(chan string, 1)
			p.correctionClipboard = func(text string) error { copied <- text; return nil }
			p.outputTranscript(&recordingSession{sessionID: 7, correction: cfg.Correction}, "普通语音输入。")
			select {
			case text := <-copied:
				if text != "普通语音输入。" {
					t.Fatalf("raw transcript changed: %q", text)
				}
			case <-time.After(time.Second):
				t.Fatal("raw transcript was not copied")
			}
			if requests.Load() != 0 {
				t.Fatal("model was called without a Codex draft")
			}
			p.mu.Lock()
			hasError := p.lastError != ""
			p.mu.Unlock()
			if hasError != tc.wantError {
				t.Fatalf("error=%v want=%v", hasError, tc.wantError)
			}
		})
	}
}

func TestCorrectionPipelinePreservesRawTextAndCancelsWithoutPasting(t *testing.T) {
	for _, mode := range []string{"success", "changed", "error", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			entered := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				close(entered)
				if mode == "cancel" {
					select {
					case <-r.Context().Done():
					case <-time.After(time.Second):
					}
					return
				}
				if mode == "error" {
					w.WriteHeader(401)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"corrected_text":"适配 Codex，端口 5432。"}`}, "finish_reason": "stop"}}})
			}))
			defer server.Close()
			cfg := config.Default()
			cfg.Correction = config.CorrectionConfig{Provider: "openai-compatible", Enabled: true, Context: true, BaseURL: server.URL, Model: "small-test", APIKey: "test-key", TimeoutMS: 1000}
			eng := engine.New(hotkey.NewMockProvider(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
			p := NewVoicePlugin()
			if err := eng.LoadPlugin(p); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(eng.Stop)
			p.sessionID = 7
			p.pendingDone = 1
			p.finishingSessions = map[uint64]struct{}{7: {}}
			var guardCalls atomic.Int32
			p.correctionCapture = func() (*correction.Target, error) {
				return &correction.Target{
					Source:   correction.Source{ID: correction.CodexDesktopSourceID, Name: "Codex Desktop"},
					Messages: []correction.Message{{Role: "user", Text: "Codex 插件"}}, Close: func() {},
					Guard: func() error {
						if guardCalls.Add(1) > 1 && mode == "changed" {
							return errors.New("当前聊天已切换")
						}
						return nil
					}}, nil
			}
			copied := ""
			p.correctionClipboard = func(text string) error { copied = text; return nil }
			done := make(chan struct{})
			go func() {
				p.outputTranscript(&recordingSession{sessionID: 7, correction: cfg.Correction}, "适配扣得克斯，端口 5432。")
				close(done)
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("correction never started")
			}
			if mode == "cancel" {
				p.cancelRecording()
			}
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("correction prevented cancellation")
			}
			if mode == "success" {
				if copied != "适配 Codex，端口 5432。" || p.transcript != copied {
					t.Fatal("corrected result was not delivered to preview and clipboard")
				}
			} else if copied != "适配扣得克斯，端口 5432。" {
				t.Fatal("failed correction lost the raw transcript")
			}
			if (mode == "changed" || mode == "error") && !strings.Contains(p.lastError, "未上屏") {
				t.Fatal("failure was hidden")
			}
		})
	}
}

func TestCorrectionRejectsChangedInputBeforeModelRequest(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	var requests, closes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	cfg := config.Default()
	cfg.Correction = config.CorrectionConfig{Provider: "openai-compatible", Enabled: true, Context: true, BaseURL: server.URL, Model: "small-test", APIKey: "test-key", TimeoutMS: 1000}
	eng := engine.New(hotkey.NewMockProvider(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	p := NewVoicePlugin()
	if err := eng.LoadPlugin(p); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(eng.Stop)
	p.sessionID = 7
	p.correctionCapture = func() (*correction.Target, error) {
		return &correction.Target{
			Source:   correction.Source{ID: correction.CodexDesktopSourceID, Name: "Codex Desktop"},
			Messages: []correction.Message{{Role: "user", Text: "当前项目"}},
			Guard:    func() error { return errors.New("当前聊天已切换") },
			Close:    func() { closes.Add(1) },
		}, nil
	}
	var copied string
	p.correctionClipboard = func(text string) error { copied = text; return nil }
	p.outputTranscript(&recordingSession{sessionID: 7, correction: cfg.Correction}, "测试一下。")
	if requests.Load() != 0 || closes.Load() != 1 {
		t.Fatalf("requests=%d closes=%d; changed input must stop before the model call and release the binding", requests.Load(), closes.Load())
	}
	if copied != "测试一下。" || !strings.Contains(p.lastError, "未上屏") {
		t.Fatal("changed input must report failure and preserve the raw transcript")
	}
}
