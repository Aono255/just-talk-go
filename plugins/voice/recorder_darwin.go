//go:build darwin && cgo

package voice

// #cgo LDFLAGS: -framework AVFoundation -framework CoreAudio -framework CoreMedia -framework Foundation
// #include "capture_darwin.h"
import "C"

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"
	"unsafe"
)

type darwinAudioReadCloser struct {
	rec       *C.jt_capture_t
	readMu    sync.Mutex
	closeOnce sync.Once
	closeErr  error
	closed    bool
	tail      bytes.Buffer
}

func (r *darwinAudioReadCloser) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	r.readMu.Lock()
	defer r.readMu.Unlock()
	if r.closed {
		return r.tail.Read(p)
	}
	var detail [512]C.char
	n := int(C.jt_capture_read(r.rec, unsafe.Pointer(&p[0]), C.size_t(len(p)), &detail[0], C.size_t(len(detail))))
	if n < 0 {
		return 0, fmt.Errorf("AVFoundation capture: %s", C.GoString(&detail[0]))
	}
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

func (r *darwinAudioReadCloser) Close() error {
	r.closeOnce.Do(func() {
		// Stop wakes a blocked native reader; destroy only after it has returned.
		C.jt_capture_stop(r.rec)
		r.readMu.Lock()
		defer r.readMu.Unlock()
		// Recorder.Stop drains after its stop callback, so preserve unread PCM.
		buf := make([]byte, 8192)
		var detail [512]C.char
		for {
			n := int(C.jt_capture_read(r.rec, unsafe.Pointer(&buf[0]), C.size_t(len(buf)), &detail[0], C.size_t(len(detail))))
			if n < 0 {
				r.closeErr = fmt.Errorf("AVFoundation capture: %s", C.GoString(&detail[0]))
				break
			}
			if n == 0 {
				break
			}
			r.tail.Write(buf[:n])
		}
		C.jt_capture_destroy(r.rec)
		r.closed = true
	})
	return r.closeErr
}

func startCaptureWithDevice(logger *slog.Logger, device string) (io.ReadCloser, string, func() error, error) {
	if device != "" && device != "default" {
		return nil, "", nil, fmt.Errorf("macOS 录音仅支持系统当前默认麦克风")
	}

	var rec *C.jt_capture_t
	var report C.jt_capture_start_report_t
	var detail [512]C.char
	started := time.Now()
	rc := int(C.jt_capture_start(&rec, &report, &detail[0], C.size_t(len(detail))))
	logger.Debug("native capture start completed", "backend", "avfoundation", "status", rc,
		"elapsed_ms", time.Since(started).Milliseconds(), "select_ms", int64(report.select_ms),
		"configure_ms", int64(report.configure_ms), "start_ms", int64(report.start_ms),
		"first_buffer_ms", int64(report.first_buffer_ms))
	if rc != 0 {
		return nil, "", nil, fmt.Errorf("AVFoundation start: %s", C.GoString(&detail[0]))
	}

	closer := &darwinAudioReadCloser{rec: rec}
	stop := func() error {
		err := closer.Close()
		if err != nil {
			logger.Error("native capture stop failed", "error", err)
		}
		return err
	}
	return closer, "avfoundation", stop, nil
}

func ListDevices() ([]string, error) {
	return []string{"default"}, nil
}
