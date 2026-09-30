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
