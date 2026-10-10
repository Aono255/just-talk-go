package voice

import (
	"encoding/binary"
	"math"
)

// The existing 16-bit PCM stream drives the overlay; no additional audio capture is opened.
func pcmLevel(pcm []byte) float64 {
	samples := len(pcm) / 2
	if samples == 0 {
		return 0
	}
	var sum float64
	for i := 0; i < samples; i++ {
		value := float64(int16(binary.LittleEndian.Uint16(pcm[i*2:]))) / 32768
		sum += value * value
	}
	return math.Min(1, math.Sqrt(sum/float64(samples))*6)
}

func updateAudioLevel(sessionID uint64, pcm []byte) {
	level := pcmLevel(pcm)
	setTUIStatus(func(status *TUIVoiceStatus) {
		if status.SessionID == sessionID && status.Recording {
			status.AudioLevel = level
		}
	})
}
