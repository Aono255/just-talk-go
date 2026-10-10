package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/c/just-talk-go/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestSavePassesNewCredentialsToReload(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("config.toml", []byte("[voice]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	original := config.Default()
	original.Voice.AccessKey = "old-token"
	model := New(original)
	for i := range model.fields {
		if model.fields[i].key == "access_key" {
			model.fields[i].input.SetValue("new-token")
		}
	}
	var reloaded *config.Config
	model.OnSave = func(cfg *config.Config) error {
		reloaded = cfg
		return nil
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if reloaded == nil || reloaded.Voice.AccessKey != "new-token" {
		t.Fatal("saving did not deliver the new credentials to the reload callback")
	}
	saved, err := config.Load("config.toml")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Voice.AccessKey != "new-token" {
		t.Fatal("saving did not persist the new credentials")
	}
}

func TestCorrectionKeyIsMaskedAndConfigurationViewportFits(t *testing.T) {
	cfg := config.Default()
	cfg.Correction.APIKey = "never-show-this-secret"
	cfg.Correction.BaseURL = "https://example.invalid/v1"
	model := New(cfg)
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	for i, f := range model.fields {
		if f.key == "correction_api_key" {
			model.cursor = i
		}
	}
	for _, editing := range []bool{false, true} {
		model.editing = editing
		view := model.View()
		if strings.Contains(view, cfg.Correction.APIKey) || !strings.Contains(view, "模型 API Key") {
			t.Fatal("API key leaked or editing row is invisible")
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > 80 {
				t.Fatalf("field wrapped outside terminal: %q", line)
			}
		}
		if strings.Count(view, "\n")+1 > 24 {
			t.Fatal("configuration pushes recording status outside a 24-row terminal")
		}
	}
}
