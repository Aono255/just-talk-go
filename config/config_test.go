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
