package correction

import (
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var ErrUnsupportedInput = errors.New("当前输入框未提供可读取的可编辑文本")

// Input binds one editable field, independent of conversation/client adapters.
// Prefix and Suffix exclude pre-existing text around this dictation's selection.
type Input struct {
	Before, Prefix, Suffix string
	Role                   string
	Guard                  func() error
	Read                   func() (string, error)
	Close                  func()
}

func CaptureInput() (*Input, error) { return captureInputPlatform() }

// Destination intentionally binds only the app and window. It does not infer
// an input field, selection or chat when those are unavailable.
type Destination struct {
	Guard func() error
	Close func()
}

func CaptureDestination() (*Destination, error) { return captureDestinationPlatform() }

// Revision records only changes after observing this exact output in the bound
// field. Its lock makes stopping atomic with accepting a late native read.
type Revision struct {
	mu                sync.Mutex
	input             *Input
	original, output  string
	language          string
	observed, stopped bool
	samples           []revisionSample
}

type revisionSample struct {
	value string
	at    time.Time
}

func NewRevision(input *Input, original, output, language string) *Revision {
	return &Revision{input: input, original: original, output: output, language: language}
}

func (r *Revision) Observe(value string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return false
	}
	expected := r.input.Prefix + r.output + r.input.Suffix
	if !r.observed {
		r.observed = value == expected
		if !r.observed {
			return true
		}
	}
	if value == "" || !strings.HasPrefix(value, r.input.Prefix) || !strings.HasSuffix(value, r.input.Suffix) || len(value) < len(r.input.Prefix)+len(r.input.Suffix) {
		return false
	}
	r.samples = append(r.samples, revisionSample{value, now})
	if len(r.samples) > 16 {
		r.samples = r.samples[len(r.samples)-16:]
	}
	return true
}

// Confirm uses a recent snapshot taken before the actual Enter event, never an
// idle timer. current was freshly read from the same bound field; an empty
// value is allowed because the application may already have handled Enter.
func (r *Revision) Confirm(at time.Time, current string) *Feedback {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped || !r.observed {
		return nil
	}
	r.stopped = true
	for i := len(r.samples) - 1; i >= 0; i-- {
		sample := r.samples[i]
		if sample.at.After(at) {
			continue
		}
		if at.Sub(sample.at) > 500*time.Millisecond || (current != "" && current != sample.value) {
			return nil
		}
		text := sample.value[len(r.input.Prefix) : len(sample.value)-len(r.input.Suffix)]
		if strings.TrimSpace(text) == "" || text == r.output {
			return nil
		}
		for _, part := range []string{r.original, r.output, text} {
			if utf8.RuneCountInString(part) > 2000 {
				return nil
			}
		}
		return &Feedback{Draft: r.original, Corrected: r.output, Revised: text, Language: r.language}
	}
	return nil
}

func (r *Revision) Observed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.observed
}

func (r *Revision) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	r.samples = nil
}
