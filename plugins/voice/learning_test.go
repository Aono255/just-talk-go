package voice

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/c/just-talk-go/config"
	"github.com/c/just-talk-go/engine"
	"github.com/c/just-talk-go/hotkey"
	"github.com/c/just-talk-go/internal/correction"
)

func TestLearningUsesSentFeedbackAndReusesLocalMemoryWithoutExtraRequests(t *testing.T) {
	for _, confirmed := range []bool{false, true} {
		t.Run(map[bool]string{false: "unconfirmed", true: "sent_in_same_chat"}[confirmed], func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			const note = "在该平台语境中，P01 应还原为 D01"
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				var request struct{ Messages []struct{ Content string } }
				if json.NewDecoder(r.Body).Decode(&request) != nil || len(request.Messages) != 2 {
					t.Error("invalid request")
					w.WriteHeader(400)
					return
				}
				var data struct {
					Memory   []string
					Feedback *correction.Feedback
				}
				if json.Unmarshal([]byte(request.Messages[1].Content), &data) != nil {
					t.Error("invalid correction data")
				}
				if call == 2 && confirmed {
					if data.Feedback == nil || data.Feedback.Submitted != "使用 D01。" || data.Feedback.Draft != "使用 P01。" {
						t.Error("actual sent edit was not included")
					}
				} else if data.Feedback != nil {
					t.Error("unconfirmed draft was treated as sent feedback")
				}
				if call == 3 && confirmed {
					if len(data.Memory) != 1 || data.Memory[0] != note {
						t.Error("learned reference did not reach the next request")
					}
				} else if len(data.Memory) != 0 {
					t.Error("memory created before confirmed feedback")
				}
				content, _ := json.Marshal(map[string]any{"corrected_text": "使用 P01。", "memory_updates": []string{note}})
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(content)}}}})
			}))
			defer server.Close()
			cfg := config.Default()
			cfg.Correction = config.CorrectionConfig{Enabled: true, Learning: true, Provider: "openai-compatible", BaseURL: server.URL, Model: "test", APIKey: "test-only-key", TimeoutMS: 1000}
			eng := engine.New(hotkey.NewMockProvider(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
			p := NewVoicePlugin()
			if err := eng.LoadPlugin(p); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(eng.Stop)
			var targets []*correction.Target
			closed := 0
			p.correctionCapture = func() (*correction.Target, error) {
				index := len(targets)
				target := &correction.Target{Messages: []correction.Message{{Role: "user", Text: "D01 平台配置"}}, Guard: func() error { return nil }, Close: func() { closed++ }}
				target.SubmittedAfter = func(previous *correction.Target) (string, bool) {
					return "使用 D01。", confirmed && index == 1 && previous == targets[0]
				}
				targets = append(targets, target)
				return target, nil
			}
			p.correctionClipboard = func(string) error { return nil }
			for i := uint64(1); i <= 3; i++ {
				p.sessionID = i
				p.outputTranscript(&recordingSession{sessionID: i, correction: cfg.Correction}, "使用 P01。")
			}
			if calls.Load() != 3 {
				t.Fatal("learning made an extra model request")
			}
			notes, err := correction.LoadMemory(CorrectionMemoryPath())
			if err != nil || (len(notes) == 1) != confirmed {
				t.Fatal("incorrect memory persistence")
			}
			p.Stop()
			if closed != 3 {
				t.Fatal("pending context was leaked or closed more than once")
			}
		})
	}
}

func TestStaleCorrectionCannotConsumeNewerPendingLearningTarget(t *testing.T) {
	closed := 0
	target := &correction.Target{Close: func() { closed++ }}
	p := NewVoicePlugin()
	p.sessionID = 3
	p.learningPending = &pendingLearning{target: target}
	if p.prepareLearning(2, &correction.Target{}, true) != nil || p.learningPending == nil || closed != 0 {
		t.Fatal("an older correction consumed or closed a newer pending sample")
	}
}
