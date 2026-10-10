//go:build darwin && cgo

package correction

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("JUST_TALK_TEST_FOCUS_HELPER") == "1" {
		if len(os.Args) != 2 || os.Args[1] != "--focus-helper" {
			os.Exit(2)
		}
		if os.Getenv("JUST_TALK_TEST_FOCUS_SLEEP") == "1" {
			time.Sleep(3 * time.Second)
		}
		if os.Getenv("JUST_TALK_TEST_FOCUS_FAIL") == "1" {
			fmt.Fprint(os.Stderr, "test-only-private-diagnostic")
			os.Exit(1)
		}
		fmt.Fprint(os.Stdout, os.Getenv("JUST_TALK_TEST_FOCUS_RESPONSE"))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestFreshApplicationQueryDoesNotReusePreviousApp(t *testing.T) {
	t.Setenv("JUST_TALK_TEST_FOCUS_HELPER", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		pid    int
		bundle string
	}{{100, "com.openai.codex"}, {200, "com.example.messaging"}, {100, "com.openai.codex"}} {
		t.Setenv("JUST_TALK_TEST_FOCUS_RESPONSE", fmt.Sprintf(`{"pid":%d,"bundle_id":%q}`, tc.pid, tc.bundle))
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		app, err := queryApplication(ctx, executable)
		cancel()
		if err != nil || app.PID != tc.pid || app.BundleID != tc.bundle {
			t.Fatalf("app=%+v error=%v", app, err)
		}
	}
}

func TestApplicationHelperRejectsInvalidResultsAndBoundsWaiting(t *testing.T) {
	t.Setenv("JUST_TALK_TEST_FOCUS_HELPER", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, response := range []string{
		"", `{}`, `null`, `{"pid":0,"bundle_id":"com.openai.codex"}`,
		`{"pid":-1,"bundle_id":"com.openai.codex"}`, `{"pid":2147483648,"bundle_id":"com.openai.codex"}`,
		`{"pid":100,"bundle_id":""}`, `{"pid":100,"bundle_id":"com.openai.codex","extra":true}`,
		`{"pid":100,"bundle_id":"com.openai.codex"} {}`,
		fmt.Sprintf(`{"pid":100,"bundle_id":%q}`, strings.Repeat("x", 257)), strings.Repeat("x", 4097),
	} {
		t.Setenv("JUST_TALK_TEST_FOCUS_RESPONSE", response)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := queryApplication(ctx, executable)
		cancel()
		if err == nil {
			t.Fatal("invalid application identity accepted")
		}
	}
	t.Setenv("JUST_TALK_TEST_FOCUS_FAIL", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_, err = queryApplication(ctx, executable)
	cancel()
	if err == nil || strings.Contains(err.Error(), "test-only-private-diagnostic") {
		t.Fatal("helper failure was hidden or leaked its stderr")
	}
	t.Setenv("JUST_TALK_TEST_FOCUS_FAIL", "0")
	t.Setenv("JUST_TALK_TEST_FOCUS_SLEEP", "1")
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	start := time.Now()
	_, err = queryApplication(ctx, executable)
	cancel()
	if err == nil || !strings.Contains(err.Error(), "超时") || time.Since(start) > time.Second {
		t.Fatalf("unbounded helper: elapsed=%s error=%v", time.Since(start), err)
	}
}
