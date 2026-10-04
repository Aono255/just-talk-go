package voice

import (
	"testing"

	"github.com/c/just-talk-go/hotkey"
)

func TestValidateVoiceHotkeyFnRules(t *testing.T) {
	cases := []struct {
		name  string
		combo hotkey.Combo
		valid bool
	}{
		{name: "Fn with letter", combo: hotkey.Combo{Mods: hotkey.ModFn, Key: hotkey.KeyA}, valid: true},
		{name: "Fn with function key", combo: hotkey.Combo{Mods: hotkey.ModFn, Key: hotkey.KeyF5}},
		{name: "plain letter", combo: hotkey.Combo{Key: hotkey.KeyA}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := validateVoiceHotkey(test.combo)
			if test.valid && err != nil {
				t.Fatalf("validateVoiceHotkey(%s): %v", test.combo, err)
			}
			if !test.valid && err == nil {
				t.Fatalf("validateVoiceHotkey(%s) succeeded, want error", test.combo)
			}
		})
	}
}
