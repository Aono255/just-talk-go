package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/c/just-talk-go/hotkey"
)

func TestLoadOverlayPosition(t *testing.T) {
	for _, test := range []struct {
		content string
		want    string
	}{
		{content: "[overlay]\nenabled = true\n", want: DefaultOverlayPosition()},
		{content: "[overlay]\nposition = \"bottom-center\"\n", want: "bottom-center"},
		{content: "[overlay]\nposition = \"notch\"\n", want: "notch"},
	} {
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte(test.content), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Overlay.Position != test.want {
			t.Fatalf("Load(%q).Overlay.Position = %q, want %q", test.content, cfg.Overlay.Position, test.want)
		}
	}
}

func TestParseHotkeyFnModifier(t *testing.T) {
	for _, input := range []string{"Fn", "Function"} {
		combo, err := ParseHotkey(input)
		if err != nil {
			t.Fatalf("ParseHotkey(%q): %v", input, err)
		}
		want := hotkey.Combo{Mods: hotkey.ModFn, Key: hotkey.KeyNone}
		if combo != want {
			t.Fatalf("ParseHotkey(%q) = %v, want %v", input, combo, want)
		}
	}
	for _, test := range []struct {
		input string
		key   hotkey.KeyCode
	}{
		{input: "Fn+F5", key: hotkey.KeyF5},
		{input: "Fn+A", key: hotkey.KeyA},
	} {
		combo, err := ParseHotkey(test.input)
		if err != nil {
			t.Fatalf("ParseHotkey(%q): %v", test.input, err)
		}
		want := hotkey.Combo{Mods: hotkey.ModFn, Key: test.key}
		if combo != want {
			t.Fatalf("ParseHotkey(%q) = %v, want %v", test.input, combo, want)
		}
	}
}
