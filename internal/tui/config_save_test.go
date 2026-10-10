package tui

import (
	"os"
	"reflect"
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
	original.Voice.Hotwords = []string{"Codex"}
	model := New(original)
	for i := range model.fields {
		if model.fields[i].key == "access_key" {
			model.fields[i].input.SetValue("new-token")
		}
		if model.fields[i].key == "correction_terms" {
			model.fields[i].input.SetValue("ClawOps, 两高一弱，G01, ClawOps")
		}
		if model.fields[i].key == "boosting_table_id" {
			model.fields[i].input.SetValue(" cloud-table-id ")
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
	if saved.Voice.BoostingTableID != "cloud-table-id" || reloaded.Voice.BoostingTableID != "cloud-table-id" {
		t.Fatal("cloud hotword table ID was not saved and reloaded")
	}
	expected := []string{"ClawOps", "两高一弱", "G01"}
	if !reflect.DeepEqual(saved.Correction.Terms, expected) || !reflect.DeepEqual(reloaded.Correction.Terms, expected) || !reflect.DeepEqual(saved.Voice.Hotwords, original.Voice.Hotwords) {
		t.Fatal("AI terms were not saved/reloaded independently from ASR hotwords")
	}
}

func TestCorrectionKeyIsMaskedAndConfigurationViewportFits(t *testing.T) {
	cfg := config.Default()
	cfg.Correction.APIKey = "never-show-this-secret"
	cfg.Correction.BaseURL = "https://example.invalid/v1"
	cfg.Correction.Terms = []string{"ClawOps", strings.Repeat("术语", 100)}
	model := New(cfg)
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	for _, key := range []string{"correction_api_key", "correction_terms", "boosting_table_id"} {
		for i, f := range model.fields {
			if f.key == key {
				model.cursor = i
			}
		}
		for _, editing := range []bool{false, true} {
			model.editing = editing
			view := model.View()
			if strings.Contains(view, cfg.Correction.APIKey) || !strings.Contains(view, model.fields[model.cursor].label) {
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
}
