package tui

import (
	"os"
	"testing"

	"github.com/c/just-talk-go/config"
	tea "github.com/charmbracelet/bubbletea"
)

func TestSavePassesNewCredentialsToReload(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("config.toml", []byte("[voice]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	original := config.Default()
	original.Voice.AccessKey = "old-token"
	model := New(original)
	// Credentials now live under voice.engine_configs[doubao-stream]; the TUI
	// edits them through the engine-config modal.
	model.openEngineConfig()
	for i := range model.ecFields {
		if model.ecFields[i].key == "access_key" {
			model.ecFields[i].input.SetValue("new-token")
			model.commitEngineConfig(&model.ecFields[i])
		}
	}
	model.closeOverlay()
	var reloaded *config.Config
	model.OnSave = func(cfg *config.Config) error {
		reloaded = cfg
		return nil
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if reloaded == nil || reloaded.Voice.EngineConfigs["doubao-stream"]["access_key"] != "new-token" {
		t.Fatal("saving did not deliver the new credentials to the reload callback")
	}
	saved, err := config.Load("config.toml")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Voice.EngineConfigs["doubao-stream"]["access_key"] != "new-token" {
		t.Fatal("saving did not persist the new credentials")
	}
}

func overlayPositionField(t *testing.T, model *Model) int {
	t.Helper()
	for i := range model.fields {
		if model.fields[i].key == "overlay_position" {
			return i
		}
	}
	t.Fatal("TUI has no overlay position field")
	return -1
}

func loadTestConfig(t *testing.T, content string) *config.Config {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.WriteFile("config.toml", []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load("config.toml")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestOverlayPositionEditPersistsAcrossReload(t *testing.T) {
	model := New(loadTestConfig(t, "[overlay]\nposition = \"top-left\"\n"))
	var reloaded *config.Config
	model.OnSave = func(cfg *config.Config) error {
		reloaded = cfg
		return nil
	}
	model.cursor = overlayPositionField(t, model)
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyEnter},
		{Type: tea.KeyRunes, Runes: []rune{'j'}},
		{Type: tea.KeyRunes, Runes: []rune{'j'}},
		{Type: tea.KeyRunes, Runes: []rune{'j'}},
		{Type: tea.KeyEnter},
	} {
		model.handleKey(key)
	}
	if reloaded == nil || reloaded.Overlay.Position != "bottom-left" {
		t.Fatalf("reload callback got %+v, want overlay position bottom-left", reloaded)
	}
	saved, err := config.Load("config.toml")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Overlay.Position != "bottom-left" {
		t.Fatalf("saved overlay position = %q, want bottom-left", saved.Overlay.Position)
	}
	next := New(saved)
	f := next.fields[overlayPositionField(t, next)]
	if got := f.opts[f.optIdx]; got != "bottom-left" {
		t.Fatalf("reopened TUI shows overlay position %q, want bottom-left", got)
	}
}

func TestSaveKeepsOverlayPosition(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		want    string
	}{
		{name: "unset uses platform default", content: "[voice]\n", want: config.DefaultOverlayPosition()},
		{name: "explicit bottom-center", content: "[overlay]\nposition = \"bottom-center\"\n", want: "bottom-center"},
		{name: "explicit notch", content: "[overlay]\nposition = \"notch\"\n", want: "notch"},
		{name: "unknown value", content: "[overlay]\nposition = \"middle\"\n", want: "middle"},
		{name: "explicit empty", content: "[overlay]\nposition = \"\"\n", want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := New(loadTestConfig(t, test.content))
			f := model.fields[overlayPositionField(t, model)]
			if got := f.opts[f.optIdx]; got != test.want {
				t.Fatalf("TUI shows overlay position %q, want %q", got, test.want)
			}
			model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
			saved, err := config.Load("config.toml")
			if err != nil {
				t.Fatal(err)
			}
			if saved.Overlay.Position != test.want {
				t.Fatalf("saved overlay position = %q, want %q", saved.Overlay.Position, test.want)
			}
		})
	}
}
