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
	observed, stopped bool
	revised           string
	changedAt         time.Time
}

func NewRevision(input *Input, original, output string) *Revision {
	return &Revision{input: input, original: original, output: output}
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
		return true
	}
	if value == "" || !strings.HasPrefix(value, r.input.Prefix) || !strings.HasSuffix(value, r.input.Suffix) || len(value) < len(r.input.Prefix)+len(r.input.Suffix) {
		return false
	}
	text := value[len(r.input.Prefix) : len(value)-len(r.input.Suffix)]
	if text != r.revised {
		r.revised, r.changedAt = text, now
	}
	return true
}

func (r *Revision) Ready(now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.stopped && r.validLocked() && now.Sub(r.changedAt) >= 1500*time.Millisecond
}

func (r *Revision) Observed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.observed
}

func (r *Revision) validLocked() bool {
	if !r.observed || strings.TrimSpace(r.revised) == "" || r.revised == r.output {
		return false
	}
	for _, text := range []string{r.original, r.output, r.revised} {
		if utf8.RuneCountInString(text) > 2000 {
			return false
		}
	}
	return true
}

func (r *Revision) Stop(keep bool) *Feedback {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return nil
	}
	r.stopped = true
	if !keep || !r.validLocked() {
		return nil
	}
	return &Feedback{Draft: r.original, Corrected: r.output, Revised: r.revised}
}
