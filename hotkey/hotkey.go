// Package hotkey provides cross-platform global hotkey registration and event handling.
//
// Supports four platforms: X11, Wayland, macOS, Windows.
// Each platform has its own provider implementation selected via build tags.
//
// Key features:
//   - Modifier-only hotkeys (e.g., pressing just Ctrl)
//   - Function-key-only hotkeys (e.g., pressing just F1)
//   - Full combinations (e.g., Ctrl+Shift+F)
//   - KeyDown, KeyUp, and KeyPress event types for push-to-talk scenarios
package hotkey

import "time"

// Modifier represents modifier key bitmask.
type Modifier uint32

const (
	ModNone  Modifier = 0
	ModCtrl  Modifier = 1 << iota // 1
	ModAlt                        // 2
	ModShift                      // 4
	ModSuper                      // 8 (Win / Cmd)
)

// String returns a human-readable representation of the modifier mask.
func (m Modifier) String() string {
	if m == ModNone {
		return "None"
	}
	s := ""
	if m&ModCtrl != 0 {
		s += "Ctrl+"
	}
	if m&ModAlt != 0 {
		s += "Alt+"
	}
	if m&ModShift != 0 {
		s += "Shift+"
	}
	if m&ModSuper != 0 {
		s += "Super+"
	}
	if s != "" {
		s = s[:len(s)-1] // trim trailing "+"
	}
	return s
}

// KeyCode is a unified key code across all platforms.
type KeyCode uint32

// Combo defines a hotkey combination.
//
// Special cases:
//
//	Combo{ModNone, KeyF1}        → F1 alone
//	Combo{ModCtrl, KeyNone}      → Ctrl alone (modifier-only)
//	Combo{ModCtrl, KeyF}         → Ctrl+F
//	Combo{ModCtrl|ModShift, KeyNone} → Ctrl+Shift
type Combo struct {
	Mods Modifier
	Key  KeyCode
}

// IsModifierOnly returns true if this combo is a modifier-only hotkey.
func (c Combo) IsModifierOnly() bool {
	return c.Key == KeyNone && c.Mods != ModNone
}

// IsKeyOnly returns true if this combo is a key-only (no modifiers) hotkey.
func (c Combo) IsKeyOnly() bool {
	return c.Key != KeyNone && c.Mods == ModNone
}

// String returns a human-readable representation of the combo.
func (c Combo) String() string {
	if c.IsModifierOnly() {
		return c.Mods.String()
	}
	if c.IsKeyOnly() {
		return c.Key.String()
	}
	return c.Mods.String() + "+" + c.Key.String()
}

// EventType describes the type of hotkey event.
type EventType int

const (
	KeyDown  EventType = iota // Key pressed down
	KeyUp                     // Key released
	KeyPress                  // Key pressed and quickly released (simulates a "click")
)

// String returns a human-readable event type name.
func (e EventType) String() string {
	switch e {
	case KeyDown:
		return "KeyDown"
	case KeyUp:
		return "KeyUp"
	case KeyPress:
		return "KeyPress"
	default:
		return "Unknown"
	}
}

// Event represents a hotkey event.
type Event struct {
	Combo Combo
	Type  EventType
	Time  time.Time
}
