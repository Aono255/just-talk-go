//go:build darwin && cgo

package overlay

// #cgo CFLAGS: -fblocks
// #cgo LDFLAGS: -framework AppKit -framework Foundation -framework QuartzCore
// #include <stdlib.h>
// #include "overlay_darwin.h"
import "C"

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"unsafe"
)

type helperCommand struct {
	Cmd    string  `json:"cmd"`             // config | show | hide | close
	Label  string  `json:"label,omitempty"` // CON/REC/STP/WAI/ERR/IDL (capsule mode)
	R      uint16  `json:"r,omitempty"`
	G      uint16  `json:"g,omitempty"`
	B      uint16  `json:"b,omitempty"`
	State  string  `json:"state,omitempty"`  // connecting|recording|stopping_delayed|stopping|done|error|idle
	Text   string  `json:"text,omitempty"`   // live partial ASR text, or final text when state=done
	Level  float64 `json:"level,omitempty"`  // mic level 0..1
	Detail string  `json:"detail,omitempty"` // done: "pasted" or "copied"; error: short error message
	// config: live text will be sent, so the notch opens its text area with
	// the session instead of on the first partial.
	ShowText bool `json:"show_text,omitempty"`
}

func RunHelper(position string, scale float64, input io.Reader) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	pos := C.CString(position)
	defer C.free(unsafe.Pointer(pos))

	C.jt_overlay_helper_init(pos, C.double(scale))
	go readHelperCommands(input)
	C.jt_overlay_helper_run_app()
	return nil
}

func readHelperCommands(input io.Reader) {
	scanner := bufio.NewScanner(input)
	// Lines carry the full live transcript; allow long sessions.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var cmd helperCommand
		if err := json.Unmarshal(scanner.Bytes(), &cmd); err != nil {
			fmt.Fprintf(os.Stderr, "overlay helper command parse error: %v\n", err)
			continue
		}
		switch cmd.Cmd {
		case "config":
			showText := C.int(0)
			if cmd.ShowText {
				showText = 1
			}
			C.jt_overlay_helper_configure(showText)
		case "show":
			state := C.CString(cmd.State)
			label := C.CString(cmd.Label)
			text := C.CString(cmd.Text)
			detail := C.CString(cmd.Detail)
			C.jt_overlay_helper_show(state, label, C.ushort(cmd.R), C.ushort(cmd.G), C.ushort(cmd.B), text, C.double(cmd.Level), detail)
			C.free(unsafe.Pointer(state))
			C.free(unsafe.Pointer(label))
			C.free(unsafe.Pointer(text))
			C.free(unsafe.Pointer(detail))
		case "hide":
			C.jt_overlay_helper_hide()
		case "close":
			C.jt_overlay_helper_close()
			return
		}
	}
	C.jt_overlay_helper_close()
}
