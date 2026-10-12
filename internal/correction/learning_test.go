package correction

import (
	"context"
	"encoding/json"
	"github.com/c/just-talk-go/config"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestMemoryPersistenceIsBoundedPrivateAndDoesNotStoreFeedback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "correction-memory.json")
	notes, err := LoadMemory(path)
	if err != nil || len(notes) != 0 {
		t.Fatal("missing memory should be empty")
	}
	notes = MergeMemory(nil, []string{"p 零一在该平台语境中应为 D01", "偏好简洁书面表达", "", strings.Repeat("字", 121)})
	notes = MergeMemory(notes, []string{notes[0]})
	if len(notes) != 2 {
		t.Fatal("empty, duplicate or oversized notes were retained")
	}
	if err := SaveMemory(path, notes); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadMemory(path)
	if err != nil || !reflect.DeepEqual(notes, loaded) {
		t.Fatal("memory did not survive reload")
	}
	if err := SaveMemory(path, []string{"更新后的表达偏好"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
		t.Fatal("memory file should be private")
	}
	if err := os.WriteFile(path, []byte(`{"version":2,"notes":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMemory(path); err == nil {
		t.Fatal("unsupported memory format accepted")
	}
}

func TestConcurrentMemoryCommitsMergeInsteadOfOverwritingEarlierNotes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "correction-memory.json")
	var workers sync.WaitGroup
	for _, note := range []string{"术语纠正 A", "表达偏好 B", "术语纠正 C"} {
		workers.Add(1)
		go func(note string) {
			defer workers.Done()
			if _, err := Remember(path, []string{note}); err != nil {
				t.Error(err)
			}
		}(note)
	}
	workers.Wait()
	notes, err := LoadMemory(path)
	if err != nil || len(notes) != 3 {
		t.Fatal("a concurrent memory commit lost another request's note")
	}
}

func TestRevisionLearningIsIndependentOfContextAndCorrection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Messages []struct{ Content string } }
		if json.NewDecoder(r.Body).Decode(&request) != nil || len(request.Messages) != 2 {
			t.Error("invalid request")
			w.WriteHeader(400)
			return
		}
		var data map[string]json.RawMessage
		if json.Unmarshal([]byte(request.Messages[1].Content), &data) != nil {
			t.Error("invalid data")
		}
		if _, ok := data["context"]; ok {
			t.Error("learning read chat history")
		}
		var feedback Feedback
		if json.Unmarshal(data["feedback"], &feedback) != nil || feedback.Revised != "使用 D01。" {
			t.Error("missing actual input revision")
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": `{"memory_updates":["该平台语境中的 P01 应为 D01"]}`}}}})
	}))
	defer server.Close()
	cfg := config.CorrectionConfig{Learning: true, Provider: "openai-compatible", BaseURL: server.URL, Model: "test", APIKey: "test-only", TimeoutMS: 1000}
	notes, err := Learn(context.Background(), cfg, &Feedback{Draft: "使用 P01。", Corrected: "使用 P01。", Revised: "使用 D01。"}, nil, nil)
	if err != nil || len(notes) != 1 {
		t.Fatalf("independent learning failed: %v", err)
	}
	for _, feedback := range []*Feedback{nil, {Draft: "P01", Corrected: "D01", Revised: "D01"}} {
		if _, err := Learn(context.Background(), cfg, feedback, nil, nil); err == nil {
			t.Fatal("model-only output was treated as user revision")
		}
	}
}
