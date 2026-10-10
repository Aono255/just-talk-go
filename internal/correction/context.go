package correction

import "errors"

var ErrNotCodex = errors.New("当前应用不是 Codex")

// Target binds the captured conversation to the input field that will receive this transcript.
type Target struct {
	Messages            []Message
	AvailableUsers      int
	AvailableAssistants int
	Guard               func() error
	Close               func()
}

func Capture() (*Target, error) { return capturePlatform() }
