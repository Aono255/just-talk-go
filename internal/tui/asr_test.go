package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/c/just-talk-go/config"
)

func TestASRSelectionSavesModelAndPreservesExistingResources(t *testing.T) {
	resources := config.ASRResources()
	ids := []string{"", "custom-existing-resource"}
	for _, resource := range resources {
		ids = append(ids, resource.ID)
	}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := os.WriteFile("config.toml", []byte("[voice]\n"), 0600); err != nil {
				t.Fatal(err)
			}
			cfg := config.Default()
			cfg.Voice.ResourceID = id
			cfg.Voice.AccessKey = "test-access-token"
			model := New(cfg)
			var reloaded *config.Config
			model.OnSave = func(next *config.Config) error { reloaded = next; return nil }
			model.save()
			saved, err := config.Load("config.toml")
			want := id
			if want == "" {
				want = config.DefaultASRResourceID
			}
			if err != nil || reloaded == nil || saved.Voice.ResourceID != want || reloaded.Voice.ResourceID != want || saved.Voice.AccessKey != cfg.Voice.AccessKey {
				t.Fatal("unrelated save changed the selected resource or credentials")
			}
			for i := range model.fields {
				field := &model.fields[i]
				if field.key == "resource_id" {
					field.optIdx = idxOf(field.opts, "volc.seedasr.sauc.concurrent")
				}
				if field.key == "correction_context" && field.label != "上下文修正" {
					t.Fatal("context correction is still client-specific in settings")
				}
			}
			model.save()
			saved, err = config.Load("config.toml")
			if err != nil || saved.Voice.ResourceID != "volc.seedasr.sauc.concurrent" || reloaded.Voice.ResourceID != saved.Voice.ResourceID {
				t.Fatal("selected 2.0 model did not reach configuration reload")
			}
		})
	}
}

func TestASRSelectorDisplaysHumanLabels(t *testing.T) {
	model := New(config.Default())
	f := asrResourceField("volc.seedasr.sauc.duration")
	label := model.renderField(-1, f)
	if !strings.Contains(label, "豆包 2.0（小时版）") || strings.Contains(label, "volc.") {
		t.Fatal("ASR resource ID leaked into the product selector")
	}
}
