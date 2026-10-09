package overlay

import (
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/c/just-talk-go/plugins/voice"
)

type captionBackend struct {
	label, text  string
	shows, hides int
}

func (b *captionBackend) Show(label, text string, color statusColor) error {
	b.label, b.text = label, text
	b.shows++
	return nil
}
func (b *captionBackend) Hide() error  { b.hides++; return nil }
func (b *captionBackend) Close() error { return nil }

func TestOverlayUpdatesCaptionAndExpiresFinalPreview(t *testing.T) {
	b := &captionBackend{}
	p := &Plugin{backend: b, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	s := voice.TUIVoiceStatus{State: "recording", Transcript: "开始"}
	p.sync(s)
	s.Transcript = "开始检查 Codex 的语音输入"
	p.sync(s)
	if b.shows != 2 || b.text != s.Transcript {
		t.Fatal("caption changes were lost while recording state stayed the same")
	}
	s.State = "idle"
	s.TranscriptUntil = time.Now().Add(time.Minute)
	p.sync(s)
	if b.label != "DONE" || b.text != s.Transcript || !p.lastVisible {
		t.Fatal("completed recognition did not remain visible")
	}
	s.TranscriptUntil = time.Now().Add(-time.Minute)
	p.sync(s)
	if b.hides != 1 || p.lastVisible {
		t.Fatal("completed preview did not expire")
	}
	s.State = "recording"
	s.Transcript = strings.Repeat("语音🎤", 60)
	p.sync(s)
	if !utf8.ValidString(b.text) || utf8.RuneCountInString(b.text) != 101 || !strings.HasSuffix(s.Transcript, b.text[3:]) {
		t.Fatal("long preview did not retain the latest 100 Unicode characters")
	}
}
