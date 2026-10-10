package autotype

import (
	"log/slog"
)

// Paste inserts text into the currently focused input field.
func Paste(text string, logger *slog.Logger) error {
	return pastePlatform(text, logger, nil)
}

// PasteChecked verifies the captured input immediately before posting the paste shortcut.
func PasteChecked(text string, logger *slog.Logger, guard func() error) error {
	if err := guard(); err != nil {
		return err
	}
	return pastePlatform(text, logger, guard)
}
