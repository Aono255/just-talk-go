package hotkey

import (
	"testing"
	"time"
)

func TestPlainEnterDoesNotMatchModifiedEnter(t *testing.T) {
	for mask := 0; mask < 16; mask++ {
		tracker := NewKeyStateTracker()
		combo := Combo{Key: KeyEnter}
		tracker.Watch(combo, make(chan Event, 4))
		for bit, key := range []KeyCode{KeyShift, KeyCtrl, KeyAlt, KeySuper} {
			if mask&(1<<bit) != 0 {
				tracker.KeyDown(key, time.Now())
			}
		}
		events := tracker.KeyDown(KeyEnter, time.Now())
		if (len(events) == 1) != (mask == 0) {
			t.Fatalf("modifier mask %d: unexpected Enter events %v", mask, events)
		}
	}
}

func TestStandardComboKeyUpFiresWhenModifierReleasesFirst(t *testing.T) {
	tracker := NewKeyStateTracker()
	ch := make(chan Event, 4)
	combo := Combo{Mods: ModAlt, Key: KeyR}
	tracker.Watch(combo, ch)

	events := tracker.KeyDown(KeyAlt, time.Now())
	if len(events) != 0 {
		t.Fatalf("Alt down emitted %d events, want 0", len(events))
	}

	events = tracker.KeyDown(KeyR, time.Now())
	if len(events) != 1 || events[0].Combo != combo || events[0].Type != KeyDown {
		t.Fatalf("R down emitted %#v, want one KeyDown for %s", events, combo)
	}

	events = tracker.KeyUp(KeyAlt, time.Now())
	if len(events) != 1 || events[0].Combo != combo || events[0].Type != KeyUp {
		t.Fatalf("Alt up emitted %#v, want one KeyUp for %s", events, combo)
	}

	events = tracker.KeyUp(KeyR, time.Now())
	if len(events) != 0 {
		t.Fatalf("R up emitted %d events, want 0", len(events))
	}
}

func TestStandardComboKeyDownFiresWhenModifierPressedAfterKey(t *testing.T) {
	tracker := NewKeyStateTracker()
	ch := make(chan Event, 4)
	combo := Combo{Mods: ModAlt, Key: KeyR}
	tracker.Watch(combo, ch)

	events := tracker.KeyDown(KeyR, time.Now())
	if len(events) != 0 {
		t.Fatalf("R down emitted %d events, want 0", len(events))
	}

	events = tracker.KeyDown(KeyAlt, time.Now())
	if len(events) != 1 || events[0].Combo != combo || events[0].Type != KeyDown {
		t.Fatalf("Alt down emitted %#v, want one KeyDown for %s", events, combo)
	}

	events = tracker.KeyUp(KeyR, time.Now())
	if len(events) != 1 || events[0].Combo != combo || events[0].Type != KeyUp {
		t.Fatalf("R up emitted %#v, want one KeyUp for %s", events, combo)
	}

	events = tracker.KeyUp(KeyAlt, time.Now())
	if len(events) != 0 {
		t.Fatalf("Alt up emitted %d events, want 0", len(events))
	}
}

func TestKeyOnlyComboEmitsOneEventPerTransition(t *testing.T) {
	tracker := NewKeyStateTracker()
	combo := Combo{Key: KeyF9}
	tracker.Watch(combo, make(chan Event, 4))

	events := tracker.KeyDown(KeyF9, time.Now())
	if len(events) != 1 || events[0].Combo != combo || events[0].Type != KeyDown {
		t.Fatalf("F9 down emitted %#v, want one KeyDown", events)
	}

	events = tracker.KeyUp(KeyF9, time.Now())
	if len(events) != 1 || events[0].Combo != combo || events[0].Type != KeyUp {
		t.Fatalf("F9 up emitted %#v, want one KeyUp", events)
	}
}
