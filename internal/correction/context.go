package correction

import (
	"errors"
	"fmt"
	"io"
)

// Only a non-matching application is unsupported. Failure to read a matched
// source must remain an error, so it cannot bypass the input binding guard.
var ErrUnsupportedContext = errors.New("当前应用没有已接入的上下文来源")

const CodexDesktopSourceID = "codex-desktop"

type Source struct {
	ID, Name string
}

type applicationIdentity struct {
	PID      int    `json:"pid"`
	BundleID string `json:"bundle_id"`
}

// Each adapter owns its reader, input guard and native resources.
// Shared memory and input revision learning do not depend on context adapters.
type contextAdapter struct {
	Source  Source
	Matches func(applicationIdentity) bool
	Capture func(applicationIdentity) (*Target, error)
}

// Target binds the captured conversation to the input field that will receive this transcript.
type Target struct {
	Source              Source
	Messages            []Message
	AvailableUsers      int
	AvailableAssistants int
	Guard               func() error
	Close               func()
}

func Capture() (*Target, error) { return capturePlatform() }

func captureForApplication(app applicationIdentity, adapters []contextAdapter) (*Target, error) {
	if app.PID <= 0 || app.BundleID == "" {
		return nil, errors.New("无法确认当前输入应用")
	}
	match := -1
	ids := make(map[string]bool, len(adapters))
	for i, adapter := range adapters {
		if adapter.Source.ID == "" || adapter.Source.Name == "" || ids[adapter.Source.ID] || adapter.Matches == nil || adapter.Capture == nil {
			return nil, errors.New("上下文适配器定义无效")
		}
		ids[adapter.Source.ID] = true
		if adapter.Matches(app) {
			if match >= 0 {
				return nil, errors.New("当前输入应用匹配多个上下文来源，未读取会话")
			}
			match = i
		}
	}
	if match < 0 {
		return nil, ErrUnsupportedContext
	}
	adapter := adapters[match]
	target, err := adapter.Capture(app)
	if err != nil || target == nil || target.Guard == nil || target.Close == nil || len(target.Messages) == 0 {
		if target != nil && target.Close != nil {
			target.Close()
		}
		if err != nil {
			return nil, fmt.Errorf("%s 上下文读取失败：%s", adapter.Source.Name, err)
		}
		return nil, errors.New("上下文来源未提供会话消息或完整的输入绑定")
	}
	target.Source = adapter.Source
	return target, nil
}

// RunFocusHelper writes application identity once, before configuration or plugins are loaded.
func RunFocusHelper(output io.Writer) error { return focusHelperPlatform(output) }
