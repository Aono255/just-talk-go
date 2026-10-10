package correction

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

const memoryLimit = 32
const memoryNoteLimit = 120

// ponytail: one per-user memory file; add file locks if concurrent app instances are supported.
var memoryWriteMu sync.Mutex

// Feedback contains a confirmed sent message, not an unsubmitted draft.
type Feedback struct {
	Draft     string `json:"draft"`
	Corrected string `json:"corrected"`
	Submitted string `json:"submitted"`
}

// Learning carries local references in and proposed updates out of one request.
type Learning struct {
	Memory   []string
	Feedback *Feedback
	Updates  []string
	Error    error
}

func LoadMemory(path string) ([]string, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil || len(data) > 65536 {
		return nil, errors.New("自学习记忆读取失败或文件过大")
	}
	var value struct {
		Version int      `json:"version"`
		Notes   []string `json:"notes"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&value) != nil || decoder.Decode(&extra) != io.EOF || value.Version != 1 || len(value.Notes) > memoryLimit {
		return nil, errors.New("自学习记忆格式不正确，未覆盖原文件")
	}
	for _, note := range value.Notes {
		if strings.TrimSpace(note) == "" || utf8.RuneCountInString(note) > memoryNoteLimit {
			return nil, errors.New("自学习记忆条目为空或过长，未覆盖原文件")
		}
	}
	return value.Notes, nil
}

func MergeMemory(notes, updates []string) []string {
	var out []string
	for _, note := range append(append([]string(nil), notes...), updates...) {
		note = strings.TrimSpace(note)
		if note == "" || utf8.RuneCountInString(note) > memoryNoteLimit {
			continue
		}
		for i, previous := range out {
			if previous == note {
				out = append(out[:i], out[i+1:]...)
				break
			}
		}
		out = append(out, note)
		if len(out) > memoryLimit {
			out = out[1:]
		}
	}
	return out
}

func SaveMemory(path string, notes []string) error {
	data, err := json.MarshalIndent(struct {
		Version int      `json:"version"`
		Notes   []string `json:"notes"`
	}{1, MergeMemory(notes, nil)}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".correction-memory-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Remember merges with the latest file while holding the commit lock, so a late
// older request cannot overwrite notes already committed by a newer request.
func Remember(path string, updates []string) (int, error) {
	memoryWriteMu.Lock()
	defer memoryWriteMu.Unlock()
	notes, err := LoadMemory(path)
	if err != nil {
		return 0, err
	}
	notes = MergeMemory(notes, updates)
	return len(notes), SaveMemory(path, notes)
}
