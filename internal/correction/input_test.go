package correction

import (
	"testing"
	"time"
)

func TestTranslatedRevisionCarriesOnlyItsSelectedOutputLanguage(t *testing.T) {
	now := time.Now()
	r := NewRevision(&Input{}, "请检查设备", "Please inspect device.", "en")
	r.Observe("Please inspect device.", now)
	r.Observe("Please check the device.", now.Add(time.Millisecond))
	f := r.Confirm(now.Add(2*time.Millisecond), "Please check the device.")
	if f == nil || f.Language != "en" || f.Revised != "Please check the device." {
		t.Fatal("translation feedback lost its language scope")
	}
}

func TestRevisionRequiresEnterAndUsesLatestPreEnterSnapshot(t *testing.T) {
	now := time.Now()
	input := &Input{Prefix: "原有🙂前缀", Suffix: "后缀"}
	r := NewRevision(input, "P01", "P01", "")
	r.Observe(input.Prefix+"D01"+input.Suffix, now)
	if r.Observed() {
		t.Fatal("edit before observed paste accepted")
	}
	r.Observe(input.Prefix+"P01"+input.Suffix, now)
	r.Observe(input.Prefix+"D01草稿"+input.Suffix, now.Add(time.Second))
	// An arbitrarily long pause cannot close the watch or create feedback.
	final := input.Prefix + "D01定稿" + input.Suffix
	if !r.Observe(final, now.Add(time.Minute)) {
		t.Fatal("pause ended observation")
	}
	// A later read must not replace the snapshot preceding Enter.
	r.Observe(input.Prefix+"下一条输入"+input.Suffix, now.Add(time.Minute+200*time.Millisecond))
	f := r.Confirm(now.Add(time.Minute+100*time.Millisecond), "")
	if f == nil || f.Revised != "D01定稿" || f.Corrected != "P01" {
		t.Fatalf("wrong sample: %+v", f)
	}
	if r.Observe(final, now) || r.Confirm(now, final) != nil {
		t.Fatal("duplicate confirmation")
	}
}

func TestRevisionRejectsUntouchedRevertedStaleAndCanceledSnapshots(t *testing.T) {
	for _, mode := range []string{"untouched", "reverted", "stale", "canceled", "new-input", "before-paste", "outside-span"} {
		t.Run(mode, func(t *testing.T) {
			now := time.Now()
			r := NewRevision(&Input{Prefix: "pre:", Suffix: ":post"}, "P01", "P01", "")
			if mode != "before-paste" {
				r.Observe("pre:P01:post", now)
			}
			if mode != "untouched" {
				r.Observe("pre:D01:post", now)
			}
			current := "pre:D01:post"
			if mode == "reverted" {
				r.Observe("pre:P01:post", now)
				current = "pre:P01:post"
			}
			if mode == "stale" {
				now = now.Add(time.Second)
			}
			if mode == "canceled" || mode == "outside-span" {
				r.Stop()
			}
			if mode == "new-input" {
				current = "unrelated"
			}
			if r.Confirm(now.Add(time.Millisecond), current) != nil {
				t.Fatal("invalid sample learned")
			}
		})
	}
}
