//go:build darwin

package hotkey

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the production C callback without creating a global event tap or
// requiring Accessibility permission. cgo is not supported in Go test files,
// so compile its preamble with only the event API calls replaced by test doubles.
func TestDarwinEventTapTimeout(t *testing.T) {
	source, err := os.ReadFile("provider_darwin.go")
	if err != nil {
		t.Fatal(err)
	}
	preamble, _, found := strings.Cut(string(source), `import "C"`)
	if !found {
		t.Fatal("missing cgo preamble")
	}
	var c strings.Builder
	c.WriteString(darwinCallbackTestAPI)
	for _, line := range strings.Split(preamble, "\n") {
		if strings.HasPrefix(line, "//") && !strings.HasPrefix(line, "//go:") && !strings.Contains(line, "#cgo") {
			c.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "//"), " "))
			c.WriteByte('\n')
		}
	}
	c.WriteString(darwinCallbackTestMain)
	dir := t.TempDir()
	input := filepath.Join(dir, "callback.c")
	output := filepath.Join(dir, "callback-test")
	if err := os.WriteFile(input, []byte(c.String()), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("clang", "-std=c11", "-framework", "ApplicationServices", "-o", output, input)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile callback test: %v\n%s", err, out)
	}
	if out, err := exec.Command(output).CombinedOutput(); err != nil {
		t.Fatalf("callback regression: %v\n%s", err, out)
	}
}

const darwinCallbackTestAPI = `
#include <ApplicationServices/ApplicationServices.h>
#include <stdio.h>
#include <stdlib.h>

static int enable_calls, field_reads;
static CFMachPortRef enabled_tap;
static bool tap_enabled;
static void test_enable(CFMachPortRef tap, bool enabled) {
    enable_calls++;
    enabled_tap = tap;
    tap_enabled = enabled;
}
static int64_t test_keycode(CGEventRef event, CGEventField field) {
    field_reads++;
    return 0x3A;
}
static CGEventFlags test_flags(CGEventRef event) {
    field_reads++;
    return kCGEventFlagMaskAlternate | kCGEventFlagMaskCommand;
}
#define CGEventTapEnable test_enable
#define CGEventGetIntegerValueField test_keycode
#define CGEventGetFlags test_flags
#define CHECK(condition, message) do { \
    if (!(condition)) { fprintf(stderr, "%s\n", message); return 1; } \
} while (0)
`

const darwinCallbackTestMain = `
int main(void) {
    CFMachPortRef tap = (CFMachPortRef)(uintptr_t)1;
    CGEventRef event = (CGEventRef)(uintptr_t)2;
    darwin_enable_tap(tap, 1);
    enable_calls = field_reads = 0;
    CHECK(cg_event_cb(NULL, kCGEventTapDisabledByTimeout, NULL, NULL) == NULL,
          "timeout must preserve the event return value");
    CHECK(enable_calls == 1 && enabled_tap == tap && tap_enabled,
          "timeout must re-enable the active tap");
    CHECK(field_reads == 0, "timeout notification must not read keyboard fields");

    enable_calls = field_reads = 0;
    cg_event_cb(NULL, kCGEventTapDisabledByUserInput, NULL, NULL);
    CHECK(enable_calls == 0, "user-requested disable must be respected");
    CHECK(field_reads == 0, "user disable notification must not read keyboard fields");

    int fds[2];
    CHECK(pipe(fds) == 0, "create event bridge");
    bridge_init(fds[1]);
    CGEventType types[] = {kCGEventKeyDown, kCGEventKeyUp, kCGEventFlagsChanged};
    for (int i = 0; i < 3; i++) {
        field_reads = 0;
        CHECK(cg_event_cb(NULL, types[i], event, NULL) == event,
              "keyboard events must pass through");
        bridge_event_t got;
        CHECK(read(fds[0], &got, sizeof(got)) == sizeof(got), "bridge event missing");
        CHECK(got.keycode == 0x3A && got.flags == test_flags(event) && got.event_type == i,
              "keyboard event payload changed");
        CHECK(field_reads == 3, "keyboard fields must be read only for keyboard events");
    }
    close(fds[0]);
    close(fds[1]);
    bridge_init(-1);

    darwin_enable_tap(tap, 0);
    enable_calls = field_reads = 0;
    cg_event_cb(NULL, kCGEventTapDisabledByTimeout, NULL, NULL);
    CHECK(enable_calls == 0, "intentional shutdown must not re-enable the tap");
    CHECK(field_reads == 0, "shutdown notification must not read keyboard fields");
    return 0;
}
`
