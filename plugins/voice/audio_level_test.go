package voice

import "testing"

func TestAudioLevelUsesPCMAndIgnoresFinishedOrOldSessions(t *testing.T) {
	for _, silent := range [][]byte{nil, {0}, {0, 0, 0, 0}} {
		if pcmLevel(silent) != 0 {
			t.Fatal("silence produced a signal")
		}
	}
	if level := pcmLevel([]byte{0, 0x80, 0xff, 0x7f, 0}); level != 1 {
		t.Fatalf("full scale level=%v", level)
	}
	saved := TUIStatus()
	defer setTUIStatus(func(s *TUIVoiceStatus) { *s = saved })
	setTUIStatus(func(s *TUIVoiceStatus) { *s = TUIVoiceStatus{SessionID: 2, Recording: true} })
	updateAudioLevel(1, []byte{0, 0x80})
	if TUIStatus().AudioLevel != 0 {
		t.Fatal("old audio changed the current meter")
	}
	updateAudioLevel(2, []byte{0, 0x80})
	if TUIStatus().AudioLevel != 1 {
		t.Fatal("current audio did not update the meter")
	}
	setTUIStatus(func(s *TUIVoiceStatus) { s.Recording = false; s.AudioLevel = 0 })
	updateAudioLevel(2, []byte{0, 0x80})
	if TUIStatus().AudioLevel != 0 {
		t.Fatal("finished audio changed the meter")
	}
}
