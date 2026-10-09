package voice

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/c/just-talk-go/config"
	"github.com/c/just-talk-go/engine"
	"github.com/c/just-talk-go/hotkey"
)

func TestTranscriptRejectsOldCanceledAndLateSessionResults(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	eng := engine.New(hotkey.NewMockProvider(), config.Default(), logger)
	p := NewVoicePlugin()
	if err := eng.LoadPlugin(p); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(eng.Stop)
	p.sessionID, p.recording = 2, true
	p.updateTranscript(1, "旧录音")
	if p.transcript != "" {
		t.Fatal("old recording overwrote current preview")
	}
	p.updateTranscript(2, "当前识别")
	if TUIStatus().Transcript != "当前识别" {
		t.Fatal("current recognition was not published")
	}
	p.recording = false
	p.finishingSessions = map[uint64]struct{}{2: {}}
	p.pendingDone = 1
	p.updateTranscript(2, "最终识别")
	p.recordingSessionFinished(2)
	if !TUIStatus().TranscriptUntil.After(time.Now()) {
		t.Fatal("final preview did not receive an expiry time")
	}
	p.updateTranscript(2, "收尾后的旧回调")
	if p.transcript != "最终识别" {
		t.Fatal("late callback changed completed preview")
	}
	p.finishingSessions[2] = struct{}{}
	p.canceledSessions = map[uint64]struct{}{2: {}}
	p.updateTranscript(2, "已取消的识别")
	if p.transcript != "最终识别" {
		t.Fatal("canceled recording changed preview")
	}
}
