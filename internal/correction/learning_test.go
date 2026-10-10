package correction

import (
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

func TestLearningUpdatesRequireConfirmedFeedbackAndNeverBlockTextRepairs(t *testing.T) {
	response := `{"corrected_text":"使用 D01。","memory_updates":["在该平台语境中，P01 应还原为 D01"]}`
	learning := &Learning{}
	text, err := CheckResult("使用 P01。", response, nil, learning)
	if err != nil || text != "使用 D01。" || len(learning.Updates) != 0 {
		t.Fatal("unsubmitted output created a memory or content repair was blocked")
	}
	learning.Feedback = &Feedback{Draft: "P01", Corrected: "P01", Submitted: "D01"}
	text, err = CheckResult("使用 P01。", response, nil, learning)
	if err != nil || text != "使用 D01。" || len(learning.Updates) != 1 {
		t.Fatal("confirmed feedback did not produce a proposed memory")
	}
	learning.Feedback = nil
	if _, err := CheckResult("使用 P01。", response, nil, learning); err != nil || len(learning.Updates) != 0 {
		t.Fatal("previous updates survived without confirmed feedback")
	}
	learning.Feedback = &Feedback{Submitted: "D01"}
	text, err = CheckResult("P01", `{"corrected_text":"D01","memory_updates":42}`, nil, learning)
	if err != nil || text != "D01" || learning.Error == nil || len(learning.Updates) != 0 {
		t.Fatal("invalid optional learning response blocked correction or was hidden")
	}
}
