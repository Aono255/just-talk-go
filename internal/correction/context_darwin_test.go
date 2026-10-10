//go:build darwin && cgo

package correction

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

func TestNativeFeedbackRequiresOriginalChatAnchorAndSelectsFirstSentMessage(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "feedback.m")
	program := `#include "context_darwin.m"
#include <assert.h>
int main(void) { @autoreleasepool {
    jt_correction_target previous={0}, current={0};
    previous.pid=current.pid=100;
    previous.main=current.main=(AXUIElementRef)@"same-main";
    previous.editor=current.editor=(AXUIElementRef)@"same-editor";
    previous.userAnchor=(AXUIElementRef)@"original-anchor";
    previous.messages=@[@{@"role":@"user",@"text":@"old-user-message"}];
    current.users=@[previous.messages[0],@{@"role":@"user",@"text":@"user-edited D01"},@{@"role":@"user",@"text":@"later-unrelated-message"}];
    current.userAnchors=@[@"original-anchor",@"sent-anchor",@"later-anchor"];
    char *text=NULL;
    assert(jt_correction_submitted_after(&current,&previous,&text)==1);
    assert(strcmp(text,"user-edited D01")==0); free(text); text=NULL;
    current.pid=200;
    assert(jt_correction_submitted_after(&current,&previous,&text)==0);
    current.pid=100; current.main=(AXUIElementRef)@"different-main";
    assert(jt_correction_submitted_after(&current,&previous,&text)==0);
    current.main=previous.main; current.editor=(AXUIElementRef)@"different-editor";
    assert(jt_correction_submitted_after(&current,&previous,&text)==0);
    current.editor=previous.editor;
    current.userAnchors=@[@"replacement-anchor",@"sent-anchor",@"later-anchor"];
    assert(jt_correction_submitted_after(&current,&previous,&text)==0);
    current.userAnchors=@[@"original-anchor",@"sent-anchor",@"later-anchor"];
    current.users=@[@{@"role":@"user",@"text":@"edited-old-message"},@{@"role":@"user",@"text":@"user-edited D01"},@{@"role":@"user",@"text":@"later-unrelated-message"}];
    assert(jt_correction_submitted_after(&current,&previous,&text)==0);
    current.users=previous.messages; current.userAnchors=@[@"original-anchor"];
    assert(jt_correction_submitted_after(&current,&previous,&text)==0);
    NSMutableArray *messages=[NSMutableArray array], *anchors=[NSMutableArray array], *parts=[NSMutableArray array];
    for (int i=0;i<105;i++) {
        [parts addObject:[NSString stringWithFormat:@"message-%d",i]];
        flushMessage(messages,i%2?@"assistant":@"user",parts,(AXUIElementRef)[NSString stringWithFormat:@"anchor-%d",i],anchors);
    }
    assert(messages.count==100);
    NSArray *users=[messages filteredArrayUsingPredicate:[NSPredicate predicateWithFormat:@"role == 'user'"]];
    assert(users.count==anchors.count);
    assert([anchors[0] isEqual:@"anchor-6"]);
} return 0; }`
	if err := os.WriteFile(source, []byte(program), 0600); err != nil {
		t.Fatal(err)
	}
	include, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "feedback")
	cmd := exec.Command("xcrun", "--sdk", "macosx", "clang", "-x", "objective-c", "-I", include, source, "-framework", "AppKit", "-framework", "ApplicationServices", "-o", binary)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile native feedback fixture: %v\n%s", err, output)
	}
	if output, err := exec.Command(binary).CombinedOutput(); err != nil {
		t.Fatalf("native feedback binding failed: %v\n%s", err, output)
	}
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
