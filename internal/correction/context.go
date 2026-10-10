package correction

import (
	"errors"
	"io"
)

var ErrNotCodex = errors.New("当前应用不是 Codex")

// Target binds the captured conversation to the input field that will receive this transcript.
type Target struct {
	Messages            []Message
	AvailableUsers      int
	AvailableAssistants int
	Guard               func() error
	Close               func()
	SubmittedAfter      func(*Target) (string, bool)
	platform            any
}

func Capture() (*Target, error) { return capturePlatform() }

// RunFocusHelper writes application identity once, before configuration or plugins are loaded.
func RunFocusHelper(output io.Writer) error { return focusHelperPlatform(output) }
