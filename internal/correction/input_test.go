package correction

import (
	"testing"
	"time"
)

func TestRevisionOnlyLearnsObservedVoiceSpanAndStopsAtomically(t *testing.T) {
	now := time.Now()
	input := &Input{Prefix: "原有🙂前缀", Suffix: "后缀"}
	r := NewRevision(input, "P01", "P01")
	r.Observe(input.Prefix+"D01"+input.Suffix, now)
	if r.Observed() || r.Ready(now.Add(time.Minute)) {
		t.Fatal("edit before observed paste was accepted")
	}
	r.Observe(input.Prefix+"P01"+input.Suffix, now)
	r.Observe(input.Prefix+"D01"+input.Suffix, now)
	if r.Ready(now) || !r.Ready(now.Add(2*time.Second)) {
		t.Fatal("revision stabilization is incorrect")
	}
	f := r.Stop(true)
	if f == nil || f.Revised != "D01" || f.Corrected != "P01" {
		t.Fatal("pre-existing input leaked into feedback")
	}
	if r.Observe(input.Prefix+"unrelated later paste"+input.Suffix, now) || r.Stop(true) != nil {
		t.Fatal("a stopped input accepted a late read or duplicate sample")
	}
}

func TestUntouchedRevertedAndCanceledInputsNeverBecomeMemory(t *testing.T) {
	for _, mode := range []string{"untouched", "reverted", "disabled", "outside-span"} {
		t.Run(mode, func(t *testing.T) {
			r := NewRevision(&Input{Prefix: "before:", Suffix: ":after"}, "P01", "P01")
			now := time.Now()
			r.Observe("before:P01:after", now)
			if mode == "reverted" || mode == "disabled" {
				r.Observe("before:D01:after", now)
			}
			if mode == "reverted" {
				r.Observe("before:P01:after", now)
			}
			if mode == "outside-span" && r.Observe("some unrelated input", now) {
				t.Fatal("outside-span text accepted")
			}
			if r.Stop(mode != "disabled") != nil {
				t.Fatal("unconfirmed feedback produced")
			}
		})
	}
}
