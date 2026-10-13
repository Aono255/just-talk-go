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

func TestNativeContextMessagesStayBounded(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "feedback.m")
	program := `#include "context_darwin.m"
#include <assert.h>
int main(void) { @autoreleasepool {
    NSMutableArray *messages=[NSMutableArray array], *parts=[NSMutableArray array];
    for (int i=0;i<105;i++) {
        [parts addObject:[NSString stringWithFormat:@"message-%d",i]];
        flushMessage(messages,i%2?@"assistant":@"user",parts);
    }
    assert(messages.count==100);
    assert(userMessages(messages).count==6);
    assert([messages[0][@"text"] isEqual:@"message-5"]);
    jt_correction_target target={0};
    target.userAnchor=(AXUIElementRef)@{@"id":@"original"};
    target.messages=@[@{@"role":@"user",@"text":@"private original"}];
    NSArray *changed=@[@{@"role":@"user",@"text":@"private changed"}];
    for (int identity=0;identity<3;identity++) for (int content=0;content<2;content++) {
        AXUIElementRef anchor=identity==0?target.userAnchor:identity==1?(AXUIElementRef)@{@"id":@"recreated"}:NULL;
        char *error=NULL;
        BOOL matches=userBindingMatches(&target,anchor,content?changed:target.messages,&error);
        assert(matches==(identity==0 && content==0));
        if (!matches) {
            assert(error && !strstr(error,"private") && !strstr(error,"切换"));
            const char *reason=identity==2?"控件不可用":identity==0?"正文已变化，控件相同":content?"控件和正文均已变化":"控件已变化，正文相同";
            assert(strstr(error,reason)); free(error);
        }
    }
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

func TestNativeContextExcludesPeerClocksAndFollowingToolActivity(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "message-boundary.m")
	program := `#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#include <assert.h>
static AXError readError=kAXErrorSuccess;
static AXError copyNode(AXUIElementRef element,CFArrayRef keys,AXCopyMultipleAttributeOptions options,CFArrayRef *out) {
    if (readError!=kAXErrorSuccess) return readError;
    *out=(CFArrayRef)CFRetain(element);return kAXErrorSuccess;
}
#define AXUIElementCopyMultipleAttributeValues copyNode
#include "context_darwin.m"
static NSMutableArray *n(NSString *role,NSString *text,NSArray *children) {
    return [NSMutableArray arrayWithArray:@[role,text,@"",@"",children]];
}
int main(void) { @autoreleasepool {
    id heading=n(@"AXHeading",@"You said:",@[]);
    id body=n(@"AXGroup",@"",@[n(@"AXStaticText",@"请在 12:34 检查 D01。",@[]),n(@"AXGroup",@"",@[n(@"AXStaticText",@"保留第二段与附件说明",@[])])]);
    id clock=n(@"AXStaticText",@"02:01",@[]);
    id tool=n(@"AXStaticText",@"运行了命令：private-tool-status",@[]);
    id assistant=n(@"AXGroup",@"",@[n(@"AXHeading",@"ChatGPT said:",@[]),n(@"AXGroup",@"",@[n(@"AXStaticText",@"助手正文",@[])])]);
    id latestHeading=n(@"AXHeading",@"你说：",@[]);
    id latestBody=n(@"AXStaticText",@"最后一条用户正文",@[]);
    id main=n(@"AXGroup",@"",@[heading,body,clock,n(@"AXGroup",@"",@[tool]),assistant,latestHeading,
        n(@"AXStaticText",@"16:40",@[]),n(@"AXGroup",@"",@[latestBody])]);
    id editor=n(@"AXTextArea",@"draft",@[]);
    char *error=NULL; AXUIElementRef anchor=NULL;
    NSArray *before=captureMessages((AXUIElementRef)main,(AXUIElementRef)editor,&anchor,&error);
    assert(before && !error && before.count==3 && anchor==(AXUIElementRef)latestHeading);
    assert([before[0][@"text"] isEqual:@"请在 12:34 检查 D01。\n保留第二段与附件说明"]);
    assert([before[1][@"text"] isEqual:@"助手正文"]);
    assert([before[2][@"text"] isEqual:@"最后一条用户正文"]);
    clock[1]=@"02:02";tool[1]=@"another tool status";
    NSArray *after=captureMessages((AXUIElementRef)main,(AXUIElementRef)editor,&anchor,&error);
    assert([userMessages(before) isEqual:userMessages(after)] && !error);
    jt_correction_target target={0};target.userAnchor=(AXUIElementRef)latestHeading;target.messages=userMessages(before);
    assert(userBindingMatches(&target,anchor,userMessages(after),&error));
    latestBody[1]=@"真正修改了用户消息";
    after=captureMessages((AXUIElementRef)main,(AXUIElementRef)editor,&anchor,&error);
    assert(!userBindingMatches(&target,anchor,userMessages(after),&error));
    assert(error && strstr(error,"正文已变化"));free(error);
    error=NULL;anchor=NULL;
    id emptyMain=n(@"AXGroup",@"",@[n(@"AXStaticText",@"新聊天",@[]),editor]);
    NSArray *empty=captureMessages((AXUIElementRef)emptyMain,(AXUIElementRef)editor,&anchor,&error);
    assert(empty && !empty.count && !anchor && !error);
    jt_correction_target emptyTarget={0};emptyTarget.messages=empty;
    assert(userBindingMatches(&emptyTarget,NULL,empty,&error));
    // Appearance of either role after capture invalidates an empty binding.
    assert(!userBindingMatches(&emptyTarget,NULL,@[@{@"role":@"assistant",@"text":@"new"}],&error));
    free(error);error=NULL;
    assert(!userBindingMatches(&emptyTarget,(AXUIElementRef)heading,before,&error));
    free(error);error=NULL;
    assert(!userBindingMatches(&target,NULL,empty,&error));
    free(error);error=NULL;
    // A role heading without a body is not accepted as a new chat.
    for (NSString *label in @[@"You said:",@"ChatGPT said:"]) {
        id incomplete=n(@"AXGroup",@"",@[n(@"AXHeading",label,@[]),editor]);
        assert(!captureMessages((AXUIElementRef)incomplete,(AXUIElementRef)editor,&anchor,&error));
        assert(error);free(error);error=NULL;
    }
    readError=kAXErrorCannotComplete;anchor=NULL;
    assert(!captureMessages((AXUIElementRef)emptyMain,(AXUIElementRef)editor,&anchor,&error));
    assert(error);free(error);
    // Empty chats own no user anchor, including at native resource cleanup.
    jt_correction_target *releaseTarget=calloc(1,sizeof(*releaseTarget));
    releaseTarget->app=(AXUIElementRef)CFRetain((CFTypeRef)@"app");
    releaseTarget->editor=(AXUIElementRef)CFRetain((CFTypeRef)@"editor");
    releaseTarget->main=(AXUIElementRef)CFRetain((CFTypeRef)@"main");
    jt_correction_release(releaseTarget);
} return 0; }`
	if err := os.WriteFile(source, []byte(program), 0600); err != nil {
		t.Fatal(err)
	}
	include, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "message-boundary")
	cmd := exec.Command("xcrun", "--sdk", "macosx", "clang", "-x", "objective-c", "-I", include, source, "-framework", "AppKit", "-framework", "ApplicationServices", "-o", binary)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile boundary fixture: %v\n%s", err, out)
	}
	if out, err := exec.Command(binary).CombinedOutput(); err != nil {
		t.Fatalf("boundary fixture: %v\n%s", err, out)
	}
}

func TestNativeFocusReadErrorsRemainExplicitBeforeBindingChecks(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "focus.m")
	program := `#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#include <assert.h>
static AXError copyStatus;
static CFTypeRef copyValue;
static AXError roleStatus;
static CFTypeRef roleValue;
static int roleReads, focusReads, delayedReads;
static AXError copyFocus(AXUIElementRef app, CFStringRef name, CFTypeRef *value) {
    if (CFEqual(name,kAXRoleAttribute)) {
        ++roleReads; *value=roleValue?CFRetain(roleValue):NULL; return roleStatus;
    }
    assert(CFEqual(name,kAXFocusedUIElementAttribute));
    assert(roleReads>0);
    ++focusReads;
    if (delayedReads>0) { --delayedReads; *value=NULL; return kAXErrorNoValue; }
    *value=copyValue?CFRetain(copyValue):NULL;
    return copyStatus;
}
static CFTypeID fixtureElementType(void) { return CFDictionaryGetTypeID(); }
#define AXUIElementCopyAttributeValue copyFocus
#define AXUIElementGetTypeID fixtureElementType
#include "context_darwin.m"
int main(void) { @autoreleasepool {
    jt_correction_target target={0}; target.pid=123;
    target.app=(AXUIElementRef)@{@"id":@"fixture-app"};
    roleStatus=kAXErrorSuccess; roleValue=kAXApplicationRole;
    for (int i=0;i<3;i++) {
        copyStatus=i==0?kAXErrorNoValue:i==1?kAXErrorCannotComplete:kAXErrorSuccess;
        copyValue=NULL;
        char *error=NULL;
        NSTimeInterval started=NSProcessInfo.processInfo.systemUptime;
        assert(jt_correction_guard(&target,123,&error)==0);
        assert(NSProcessInfo.processInfo.systemUptime-started<1.2);
        assert(error && strstr(error,"AX "));
        assert(strstr(error,i==1?"-25204":"-25212"));
        assert(!strstr(error,"切换")); free(error);
    }
    copyStatus=kAXErrorSuccess; copyValue=(CFTypeRef)@"invalid-type";
    char *error=NULL;
    assert(focused(target.app,&error)==NULL);
    assert(error && strstr(error,"返回类型无效")); free(error); error=NULL;
    copyValue=(CFTypeRef)@{@"id":@"fixture-editor"};
    assert(focused(target.app,&error)==(AXUIElementRef)copyValue);
    assert(error==NULL);
    roleReads=focusReads=0; delayedReads=2;
    assert(focused(target.app,&error)==(AXUIElementRef)copyValue);
    assert(error==NULL && roleReads==1 && focusReads==3);
    for (int i=0;i<3;i++) {
        roleReads=focusReads=0;
        roleStatus=i==0?kAXErrorCannotComplete:kAXErrorSuccess;
        roleValue=i==1?(CFTypeRef)kAXWindowRole:i==2?NULL:(CFTypeRef)kAXApplicationRole;
        assert(focused(target.app,&error)==NULL);
        assert(error && roleReads==1 && focusReads==0); free(error); error=NULL;
    }
} return 0; }`
	if err := os.WriteFile(source, []byte(program), 0600); err != nil {
		t.Fatal(err)
	}
	include, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "focus")
	cmd := exec.Command("xcrun", "--sdk", "macosx", "clang", "-x", "objective-c", "-I", include, source, "-framework", "AppKit", "-framework", "ApplicationServices", "-o", binary)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile native focus fixture: %v\n%s", err, output)
	}
	if output, err := exec.Command(binary).CombinedOutput(); err != nil {
		t.Fatalf("native focus failure handling: %v\n%s", err, output)
	}
}

func TestNativeGenericInputBindsEditableSelectionWithoutReadingHistory(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "input.m")
	program := `#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#include <assert.h>
static id appNode, editorNode, windowNode, focusNode;
static NSString *fieldValue, *fieldSubrole;
static BOOL frontmost=YES, writable=YES;
static AXError writableStatus=kAXErrorSuccess;
static CFRange selected;
static int valueReads;
static AXError copyInput(AXUIElementRef element, CFStringRef name, CFTypeRef *out) {
    id value=nil;
    if (CFEqual(name,kAXRoleAttribute)) value=CFEqual(element,(CFTypeRef)appNode)?(id)kAXApplicationRole:(id)kAXTextAreaRole;
    else if (CFEqual(name,kAXFocusedUIElementAttribute)) value=focusNode;
    else if (CFEqual(name,kAXFrontmostAttribute)) value=frontmost?@YES:@NO;
    else if (CFEqual(name,kAXWindowAttribute) || CFEqual(name,kAXFocusedWindowAttribute)) value=windowNode;
    else if (CFEqual(name,kAXSubroleAttribute)) value=fieldSubrole;
    else if (CFEqual(name,kAXValueAttribute)) { value=fieldValue; valueReads++; }
    else if (CFEqual(name,kAXSelectedTextRangeAttribute)) { *out=AXValueCreate(kAXValueCFRangeType,&selected); return kAXErrorSuccess; }
    else { assert(0 && "must not traverse chat or unrelated elements"); }
    *out=value?CFRetain((CFTypeRef)value):NULL;
    return value?kAXErrorSuccess:kAXErrorNoValue;
}
static bool trusted(void) { return true; }
static AXUIElementRef createApp(pid_t pid) { return (AXUIElementRef)CFRetain((CFTypeRef)appNode); }
static AXError timeout(AXUIElementRef app,float seconds) { return kAXErrorSuccess; }
static AXError getPid(AXUIElementRef element,pid_t *pid) { *pid=123;return kAXErrorSuccess; }
static AXError settable(AXUIElementRef element,CFStringRef name,Boolean *out) { *out=writable;return writableStatus; }
static CFTypeID elementType(void) { return CFDictionaryGetTypeID(); }
#define AXIsProcessTrusted trusted
#define AXUIElementCreateApplication createApp
#define AXUIElementSetMessagingTimeout timeout
#define AXUIElementGetPid getPid
#define AXUIElementIsAttributeSettable settable
#define AXUIElementCopyAttributeValue copyInput
#define AXUIElementGetTypeID elementType
#include "context_darwin.m"
int main(void) { @autoreleasepool {
 appNode=@{@"id":@"app"};editorNode=@{@"id":@"editor"};windowNode=@{@"id":@"window"};focusNode=editorNode;
 fieldValue=@"前🙂旧后";fieldSubrole=@"";selected=CFRangeMake(3,1);
 jt_input_target *input=NULL;char *data=NULL,*error=NULL,*text=NULL;
 assert(jt_input_capture(123,&input,&data,&error)==1 && input && !error);
 NSDictionary *snapshot=[NSJSONSerialization JSONObjectWithData:[[NSString stringWithUTF8String:data] dataUsingEncoding:NSUTF8StringEncoding] options:0 error:nil];
 assert([snapshot[@"prefix"] isEqual:@"前🙂"] && [snapshot[@"suffix"] isEqual:@"后"]);free(data);
 assert(jt_input_guard(input,&error)==1 && !error);
 selected=CFRangeMake(4,0);
 assert(jt_input_guard(input,&error)==0 && error);free(error);error=NULL;
 fieldValue=@"前🙂D01后";
 assert(jt_input_read(input,&text,&error)==1 && !error);assert(strcmp(text,"前🙂D01后")==0);free(text);
 frontmost=NO;valueReads=0;
 assert(jt_input_read(input,&text,&error)==-1 && valueReads==0);free(error);error=NULL;
 frontmost=YES;focusNode=@{@"id":@"other-editor"};valueReads=0;
 assert(jt_input_read(input,&text,&error)==-1 && valueReads==0);free(error);error=NULL;
 jt_input_release(input);input=NULL;focusNode=editorNode;
 writable=NO;valueReads=0;
 assert(jt_input_capture(123,&input,&data,&error)==0 && !input && valueReads==0);
 writable=YES;writableStatus=kAXErrorCannotComplete;
 assert(jt_input_capture(123,&input,&data,&error)==-1 && error && strstr(error,"-25204"));free(error);error=NULL;
 writableStatus=kAXErrorSuccess;fieldSubrole=(id)kAXSecureTextFieldSubrole;valueReads=0;
 assert(jt_input_capture(123,&input,&data,&error)==0 && !input && valueReads==0);
 // A window-only destination remains usable when the application exposes no
 // focused editor. It never acquires text and still rejects app/window changes.
 focusNode=nil;valueReads=0;
 assert(jt_window_capture(123,&input,&error)==1 && input && valueReads==0);
 assert(jt_window_guard(input,123,&error)==1 && !error);
 assert(jt_window_guard(input,456,&error)==0 && error);free(error);error=NULL;
 windowNode=@{@"id":@"other-window"};
 assert(jt_window_guard(input,123,&error)==0 && error);free(error);error=NULL;
 jt_input_release(input);
 } return 0; }
`
	if err := os.WriteFile(source, []byte(program), 0600); err != nil {
		t.Fatal(err)
	}
	include, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "input")
	command := exec.Command("xcrun", "--sdk", "macosx", "clang", "-x", "objective-c", "-I", include, source, "-framework", "AppKit", "-framework", "ApplicationServices", "-o", binary)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("compile input fixture: %v\n%s", err, output)
	}
	if output, err := exec.Command(binary).CombinedOutput(); err != nil {
		t.Fatalf("input binding fixture: %v\n%s", err, output)
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
