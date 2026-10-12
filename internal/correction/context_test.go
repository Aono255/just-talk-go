package correction

import (
	"errors"
	"testing"
)

func TestContextAdaptersSelectBeforeReadingAndKeepBindingFailuresExplicit(t *testing.T) {
	for _, mode := range []string{"unique", "unsupported", "ambiguous", "invalid_binding", "reader_failure"} {
		t.Run(mode, func(t *testing.T) {
			reads, closed := 0, 0
			adapter := contextAdapter{
				Source:  Source{ID: "test-desktop", Name: "Test Desktop"},
				Matches: func(app applicationIdentity) bool { return app.BundleID == "test.desktop" },
				Capture: func(applicationIdentity) (*Target, error) {
					reads++
					target := &Target{Messages: []Message{{Role: "user", Text: "会话"}}, Guard: func() error { return nil }, Close: func() { closed++ }}
					if mode == "invalid_binding" {
						target.Guard = nil
					}
					if mode == "reader_failure" {
						return target, ErrUnsupportedContext
					}
					return target, nil
				},
			}
			adapters := []contextAdapter{adapter}
			app := applicationIdentity{PID: 123, BundleID: "test.desktop"}
			if mode == "unsupported" {
				app.BundleID = "test.terminal"
			}
			if mode == "ambiguous" {
				duplicate := adapter
				duplicate.Source.ID = "test-other"
				adapters = append(adapters, duplicate)
			}
			target, err := captureForApplication(app, adapters)
			if mode == "unique" {
				if err != nil || target.Source != adapter.Source || reads != 1 {
					t.Fatalf("unique source: %v", err)
				}
				target.Close()
			} else {
				if err == nil || target != nil {
					t.Fatal("invalid context was accepted")
				}
				if errors.Is(err, ErrUnsupportedContext) != (mode == "unsupported") {
					t.Fatal("matched source failure became ordinary voice input")
				}
				if (mode == "unsupported" || mode == "ambiguous") && reads != 0 {
					t.Fatal("context read before unique adapter selection")
				}
			}
			if reads != closed {
				t.Fatal("failed or successful binding was leaked")
			}
		})
	}
}

func TestSubmittedFeedbackRequiresTheSameContextSource(t *testing.T) {
	called := 0
	current := &Target{Source: Source{ID: "test-desktop"}, SubmittedAfter: func(*Target) (string, bool) { called++; return "已发送", true }}
	for _, source := range []string{"", "test-terminal", "test-desktop"} {
		text, ok := current.SubmittedTextAfter(&Target{Source: Source{ID: source}})
		if ok != (source == "test-desktop") || (ok && text != "已发送") {
			t.Fatal("cross-client feedback was accepted")
		}
	}
	if called != 1 {
		t.Fatal("native feedback reader received an unrelated client binding")
	}
}
