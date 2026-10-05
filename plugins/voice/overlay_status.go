package voice

import (
	"math"
	"sync"
	"sync/atomic"
)

const (
	levelFloorDB = -50.0
	levelCeilDB  = -10.0
)

// OverlayVoiceStatus is the live data the recording overlay renders next to
// TUIVoiceStatus. It is kept separate so the TUI snapshot stays unchanged.
type OverlayVoiceStatus struct {
	Partial       string  // latest ASR text of the current session
	Level         float64 // mic level 0..1 of the active recorder
	OutputPending bool    // final text claimed; clipboard / auto-submit still running
	LastOutput    VoiceOutput
}

// VoiceOutput describes the last successfully dispatched session output.
type VoiceOutput struct {
	Seq    uint64 // increments once per session whose text reached clipboard or input
	Text   string
	Detail string // "pasted" or "copied"
}

var (
	overlayMu        sync.Mutex
	overlaySessionID uint64
	overlayPartial   string
	overlayPending   int
	overlayOutput    VoiceOutput
	activeRecorder   atomic.Pointer[Recorder]
	// overlayUpdates wakes the overlay as soon as state, partial text or
	// output changes, so it does not wait for its next poll tick.
	overlayUpdates = make(chan struct{}, 1)
)

// OverlayUpdates signals (coalesced) that OverlayStatus or TUIStatus changed.
// Mic level is not signalled; the overlay polls it.
func OverlayUpdates() <-chan struct{} { return overlayUpdates }

func notifyOverlay() {
	select {
	case overlayUpdates <- struct{}{}:
	default:
	}
}

func OverlayStatus() OverlayVoiceStatus {
	var level float64
	if rec := activeRecorder.Load(); rec != nil {
		level = rec.Level()
	}
	overlayMu.Lock()
	defer overlayMu.Unlock()
	return OverlayVoiceStatus{
		Partial:       overlayPartial,
		Level:         level,
		OutputPending: overlayPending > 0,
		LastOutput:    overlayOutput,
	}
}

func resetOverlaySession(sessionID uint64) {
	overlayMu.Lock()
	overlaySessionID = sessionID
	overlayPartial = ""
	overlayMu.Unlock()
}

func setOverlayPartial(sessionID uint64, text string) {
	overlayMu.Lock()
	changed := sessionID == overlaySessionID && text != overlayPartial
	if changed {
		overlayPartial = text
	}
	overlayMu.Unlock()
	if changed {
		notifyOverlay()
	}
}

func beginOverlayOutput() {
	overlayMu.Lock()
	overlayPending++
	overlayMu.Unlock()
	notifyOverlay()
}

// finishOverlayOutput ends an output started by beginOverlayOutput. An empty
// detail means clipboard / auto-submit failed and no result is published.
func finishOverlayOutput(text, detail string) {
	overlayMu.Lock()
	if overlayPending > 0 {
		overlayPending--
	}
	if detail != "" {
		overlayOutput = VoiceOutput{Seq: overlayOutput.Seq + 1, Text: text, Detail: detail}
	}
	overlayMu.Unlock()
	notifyOverlay()
}

// pcmLevel maps the RMS of s16le samples to 0..1 over levelFloorDB..levelCeilDB dBFS.
func pcmLevel(pcm []byte) float64 {
	samples := len(pcm) / 2
	if samples == 0 {
		return 0
	}
	var sum float64
	for i := 0; i+1 < len(pcm); i += 2 {
		s := float64(int16(pcm[i]) | int16(pcm[i+1])<<8)
		sum += s * s
	}
	rms := math.Sqrt(sum / float64(samples))
	if rms <= 0 {
		return 0
	}
	db := 20 * math.Log10(rms/32768)
	level := (db - levelFloorDB) / (levelCeilDB - levelFloorDB)
	return math.Max(0, math.Min(1, level))
}
