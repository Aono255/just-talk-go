package config

import (
	"testing"

	"github.com/c/just-talk-go/hotkey"
)

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
	for _, input := range []string{"Fn+F5", "Fn+1"} {
		if _, err := ParseHotkey(input); err == nil {
			t.Fatalf("ParseHotkey(%q) succeeded, want Fn combination error", input)
		}
	}
}
