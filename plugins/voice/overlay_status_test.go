package voice

import (
	"encoding/binary"
	"math"
	"testing"
)

func constantPCM(sample int16, samples int) []byte {
	pcm := make([]byte, samples*2)
	for i := range samples {
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(sample))
	}
	return pcm
}

func TestPCMLevelBounds(t *testing.T) {
	for _, test := range []struct {
		name   string
		sample int16
		want   float64
	}{
		{name: "silence", sample: 0, want: 0},
		{name: "below floor", sample: 50, want: 0},   // about -56 dBFS
		{name: "mid range", sample: 1036, want: 0.5}, // about -30 dBFS
		{name: "full scale negative", sample: -32768, want: 1},
		{name: "above ceiling", sample: 16384, want: 1}, // about -6 dBFS
	} {
		got := pcmLevel(constantPCM(test.sample, 320))
		if math.Abs(got-test.want) > 0.01 {
			t.Errorf("%s: pcmLevel = %.3f, want %.3f", test.name, got, test.want)
		}
	}
	if got := pcmLevel([]byte{0x7f}); got != 0 {
		t.Errorf("pcmLevel of a partial sample = %.3f, want 0", got)
	}
}

func TestFailedOutputDoesNotPublishResult(t *testing.T) {
	before := OverlayStatus().LastOutput

	beginOverlayOutput()
	if !OverlayStatus().OutputPending {
		t.Fatal("output not pending while dispatch runs")
	}
	finishOverlayOutput("text", "")
	if st := OverlayStatus(); st.OutputPending || st.LastOutput != before {
		t.Fatalf("failed output changed status: %+v", st)
	}

	beginOverlayOutput()
	finishOverlayOutput("text", "pasted")
	st := OverlayStatus()
	want := VoiceOutput{Seq: before.Seq + 1, Text: "text", Detail: "pasted"}
	if st.OutputPending || st.LastOutput != want {
		t.Fatalf("status after output = %+v, want %+v", st, want)
	}
}

func TestPartialIgnoresStaleSession(t *testing.T) {
	resetOverlaySession(1)
	setOverlayPartial(1, "first")
	resetOverlaySession(2)
	if got := OverlayStatus().Partial; got != "" {
		t.Fatalf("partial after new session = %q, want empty", got)
	}
	setOverlayPartial(1, "late result from old session")
	setOverlayPartial(2, "second")
	if got := OverlayStatus().Partial; got != "second" {
		t.Fatalf("partial = %q, want %q", got, "second")
	}
}
