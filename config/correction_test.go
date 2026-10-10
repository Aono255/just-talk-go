package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExplicitConfigSaveKeepsPathAndPrivatePermissions(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("config.toml", []byte("[voice]\napp_key='unchanged'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "trial.toml")
	if err := os.WriteFile(path, []byte("[correction]\nmodel='small'\napi_key='local-only-key'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Correction.Enabled || cfg.Correction.TimeoutMS != 8000 {
		t.Fatal("correction should be disabled with an 8-second timeout")
	}
	cfg.Correction.Model = "new-model"
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.Correction.Model != "new-model" {
		t.Fatalf("wrong save target: %v", err)
	}
	other, err := Load("config.toml")
	if err != nil || other.Voice.AppKey != "unchanged" {
		t.Fatal("custom config save changed another user configuration")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("config containing API key is not private")
	}
}
