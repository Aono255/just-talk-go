package voice

import (
	"io"
	"log/slog"
	"testing"

	"github.com/c/just-talk-go/config"
	"github.com/c/just-talk-go/engine"
	"github.com/c/just-talk-go/hotkey"
)

func TestConfigReloadUpdatesVoiceCredentials(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "enabled"
		if !enabled {
			name = "disabled"
		}
		t.Run(name, func(t *testing.T) {
			original := config.Default()
			original.Voice.AppKey = "old-app"
			original.Voice.AccessKey = "old-token"
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			eng := engine.New(hotkey.NewMockProvider(), original, logger)
			plugin := NewVoicePlugin()
			if err := eng.LoadPlugin(plugin); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(eng.Stop)

			next := *original
			next.Voice.Enabled = enabled
			next.Voice.AppKey = "new-app"
			next.Voice.AccessKey = "new-token"
			if err := eng.ReloadConfig(&next); err != nil {
				t.Fatal(err)
			}

			if plugin.cfg.Voice.AppKey != "new-app" || plugin.cfg.Voice.AccessKey != "new-token" {
				t.Fatal("voice plugin retained old credentials after reloading configuration")
			}
			if original.Voice.AppKey != "old-app" || original.Voice.AccessKey != "old-token" {
				t.Fatal("reload mutated the previous configuration snapshot")
			}
		})
	}
}
