package correction

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c/just-talk-go/config"
)

func TestDeepSeekCorrectionDisablesThinkingAndRequestsJSON(t *testing.T) {
	var request map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"corrected_text":"Codex 纠错。"}`}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	cfg := config.Default().Correction
	cfg.Enabled = true
	cfg.BaseURL = server.URL
	cfg.APIKey = "test-key"
	_, err := Correct(context.Background(), cfg, "扣得克斯纠错。", []Message{{Role: "user", Text: "Codex"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(request["thinking"]) != `{"type":"disabled"}` || string(request["response_format"]) != `{"type":"json_object"}` || string(request["temperature"]) != "0" || string(request["model"]) != `"deepseek-flash"` {
		t.Fatalf("DeepSeek speed/format settings missing: %v", request)
	}
}

func TestDeepSeekHotwordIsAddedWithoutChangingUserSettings(t *testing.T) {
	words := []string{"Codex"}
	cfg := config.Default().Correction
	if result := Hotwords(words, cfg); len(result) != 1 {
		t.Fatal("disabled correction changed ASR")
	}
	cfg.Enabled = true
	result := Hotwords(words, cfg)
	if len(result) != 2 || result[1] != "DeepSeek" || len(words) != 1 {
		t.Fatal("provider term missing or preferences mutated")
	}
	if len(Hotwords([]string{"deepseek"}, cfg)) != 1 {
		t.Fatal("provider term duplicated")
	}
}
