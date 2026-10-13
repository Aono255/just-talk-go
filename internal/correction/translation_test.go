package correction

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/c/just-talk-go/config"
)

func TestCombinedTranslationContextAndMemorySwitches(t *testing.T) {
	for _, mode := range []string{"combined", "no-context", "no-learning", "translation-only"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var request struct{ Messages []struct{ Content string } }
				json.NewDecoder(r.Body).Decode(&request)
				var data struct {
					Draft           string
					CorrectASR      bool `json:"correct_asr"`
					Context         []Message
					Normal          []string `json:"correction_memory"`
					Translation     []string `json:"translation_memory"`
					Hotwords, Terms []string
				}
				json.Unmarshal([]byte(request.Messages[1].Content), &data)
				if data.Draft != "p零一检查一下" || data.CorrectASR != (mode != "translation-only") {
					t.Error("ASR text or mode missing")
				}
				wantContext := mode != "translation-only" && mode != "no-context"
				if (len(data.Context) == 1) != wantContext {
					t.Error("chat context toggle violated")
				}
				wantNormal := mode != "translation-only" && mode != "no-learning"
				if (len(data.Normal) == 1) != wantNormal || (len(data.Translation) == 1) != (mode != "no-learning") {
					t.Error("memory switches violated")
				}
				if !reflect.DeepEqual(data.Terms, []string{"D01"}) {
					t.Error("shared glossary missing")
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": `{"translated_text":"Please check D01."}`}}}})
			}))
			defer server.Close()
			cfg := config.CorrectionConfig{Enabled: mode != "translation-only", Context: mode != "no-context", Learning: mode != "no-learning", Provider: "openai-compatible", BaseURL: server.URL, Model: "test", APIKey: "test-only", TimeoutMS: 1000, Terms: []string{"D01"}}
			out, err := Translate(context.Background(), cfg, "p零一检查一下", "en", []Message{{Role: "user", Text: "项目编号为 D01"}}, []string{"D01"}, []string{"编号 p 零一是 D01"}, []string{"英文请求使用 Please"})
			if err != nil || out != "Please check D01." || calls.Load() != 1 {
				t.Fatalf("one-call translation: %q %v calls=%d", out, err, calls.Load())
			}
		})
	}
}

func TestTranslationSettingsDefaultOffAndRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.toml")
	os.WriteFile(path, []byte("[voice]\npush_to_talk='Alt+Super'\n"), 0600)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Translation.Enabled || cfg.Translation.Hotkey != "Alt+Super+Shift" || cfg.Translation.Language != "en" {
		t.Fatal("old config defaults changed")
	}
	cfg.Translation.Enabled = true
	cfg.Translation.Language = "ja"
	cfg.Correction.APIKey = "test-only"
	if err := ValidateTranslation(cfg, "darwin"); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	saved, err := config.Load(path)
	if err != nil || saved.Translation != cfg.Translation || saved.Correction.APIKey != "test-only" {
		t.Fatal("translation/model settings lost")
	}
	for _, key := range []string{"Alt+Super", "Alt+Shift+Ctrl", "F9"} {
		cfg.Translation.Hotkey = key
		if ValidateTranslation(cfg, "darwin") == nil {
			t.Fatal("invalid overlapping chord accepted")
		}
	}
	cfg.Translation.Hotkey = "Alt+Super+Shift"
	cfg.Translation.Language = "zh"
	if ValidateTranslation(cfg, "darwin") == nil {
		t.Fatal("unsupported language")
	}
	if _, err := Translate(context.Background(), cfg.Correction, "test", "zh", nil, nil, nil, nil); err == nil {
		t.Fatal("unsupported language reached model")
	}
}
