//go:build darwin && cgo

#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#include <stdlib.h>
#include <string.h>
#include "context_darwin.h"

struct jt_correction_target {
    pid_t pid;
    AXUIElementRef app, editor, main, userAnchor;
    NSString *draft;
    NSArray *messages;
};

struct jt_input_target {
    pid_t pid;
    AXUIElementRef app, editor, window;
    NSString *before;
    CFRange selection;
};

static void fail(char **error, NSString *message) { *error = strdup([message UTF8String]); }

static id attribute(AXUIElementRef element, CFStringRef name) {
    CFTypeRef value = NULL;
    if (AXUIElementCopyAttributeValue(element, name, &value) != kAXErrorSuccess) return nil;
    return [(id)value autorelease];
}

static BOOL applicationRole(AXUIElementRef app, char **error) {
    // Chromium activates its standard accessibility interface when the app role
    // is queried. Reading focus first can otherwise return kAXErrorNoValue.
    CFTypeRef role=NULL;
    AXError status=AXUIElementCopyAttributeValue(app,kAXRoleAttribute,&role);
    if (status!=kAXErrorSuccess || !role) {
        if (role) CFRelease(role);
        fail(error,[NSString stringWithFormat:@"输入应用的辅助功能角色读取失败（AX %d），无法确认上屏目标",status==kAXErrorSuccess?kAXErrorNoValue:status]);
        return NO;
    }
    BOOL isApplication=CFGetTypeID(role)==CFStringGetTypeID() && CFEqual(role,kAXApplicationRole);
    CFRelease(role);
    if (!isApplication) { fail(error,@"输入应用的辅助功能角色无效，无法确认上屏目标"); return NO; }
    return YES;
}

static AXUIElementRef focusedWithBudget(AXUIElementRef app, char **error, NSTimeInterval budget) {
    if (!applicationRole(app,error)) return NULL;
    CFTypeRef value=NULL;
    AXError status;
    NSTimeInterval deadline=NSProcessInfo.processInfo.systemUptime+budget;
    for (;;) {
        status=AXUIElementCopyAttributeValue(app,kAXFocusedUIElementAttribute,&value);
        if (status!=kAXErrorNoValue || NSProcessInfo.processInfo.systemUptime>=deadline) break;
        if (value) { CFRelease(value); value=NULL; }
        // Only wait for the newly initialized focus; never change the target or
        // continue with a different window, missing context or another API.
        [NSThread sleepForTimeInterval:0.05];
    }
    if (status!=kAXErrorSuccess || !value) {
        if (value) CFRelease(value);
        fail(error,[NSString stringWithFormat:@"当前应用未提供可用输入焦点（AX %d），无法确认上屏目标",status==kAXErrorSuccess?kAXErrorNoValue:status]);
        return NULL;
    }
    if (CFGetTypeID(value)!=AXUIElementGetTypeID()) {
        CFRelease(value); fail(error,@"输入焦点返回类型无效，无法确认上屏目标"); return NULL;
    }
    return (AXUIElementRef)[(id)value autorelease];
}

static AXUIElementRef focused(AXUIElementRef app, char **error) {
    return focusedWithBudget(app,error,0.5);
}

// 仅由独立的单次助手进程调用；主进程不依赖 AppKit 的前台应用缓存。
int jt_correction_frontmost(pid_t *pid, char **bundle, char **error) {
    @autoreleasepool {
        NSRunningApplication *front=[[NSWorkspace sharedWorkspace] frontmostApplication];
        if (!front || front.processIdentifier<=0 || !front.bundleIdentifier.length) {
            fail(error,@"无法确认当前输入应用身份，未执行纠错或自动上屏"); return 0;
        }
        *pid=front.processIdentifier; *bundle=strdup(front.bundleIdentifier.UTF8String);
        return 1;
    }
}

// Generic input binding: no conversation traversal, app-name mapping or
// private accessibility switches. AXValue must be writable, excluding terminal
// history and other read-only text areas.
static BOOL inputFrontmost(AXUIElementRef app, char **error) {
    id front=attribute(app,kAXFrontmostAttribute);
    if (!front || CFGetTypeID((CFTypeRef)front)!=CFBooleanGetTypeID()) {
        fail(error,@"输入应用未提供前台状态，无法确认同一输入框"); return NO;
    }
    if (!CFBooleanGetValue((CFBooleanRef)front)) {
        fail(error,@"输入应用已切换，停止读取原输入框"); return NO;
    }
    return YES;
}

static BOOL inputSelection(AXUIElementRef editor, NSString *value, CFRange *range, char **error) {
    id selected=attribute(editor,kAXSelectedTextRangeAttribute);
    if (!selected || CFGetTypeID((CFTypeRef)selected)!=AXValueGetTypeID() ||
        AXValueGetType((AXValueRef)selected)!=kAXValueCFRangeType ||
        !AXValueGetValue((AXValueRef)selected,kAXValueCFRangeType,range) ||
        range->location<0 || range->length<0 || (NSUInteger)range->location>value.length ||
        (NSUInteger)range->length>value.length-(NSUInteger)range->location) {
        fail(error,@"输入框未提供有效选区，无法绑定本次语音文字"); return NO;
    }
    return YES;
}

// Basic dictation output needs an application/window destination, independently
// of whether that application publishes editable text for revision learning.
int jt_window_capture(pid_t pid, jt_input_target **out, char **error) {
    @autoreleasepool {
        if (!AXIsProcessTrusted() || pid<=0) { fail(error,@"无法确认输入应用权限或身份"); return -1; }
        AXUIElementRef app=AXUIElementCreateApplication(pid);
        AXUIElementSetMessagingTimeout(app,0.2);
        if (!applicationRole(app,error)) { CFRelease(app); return -1; }
        id window=attribute(app,kAXFocusedWindowAttribute);
        pid_t windowPid=0;
        if (!window || CFGetTypeID((CFTypeRef)window)!=AXUIElementGetTypeID() ||
            AXUIElementGetPid((AXUIElementRef)window,&windowPid)!=kAXErrorSuccess || windowPid!=pid) {
            CFRelease(app); fail(error,@"无法确认当前前台窗口，结果未上屏"); return -1;
        }
        jt_input_target *target=calloc(1,sizeof(*target));
        if (!target) { CFRelease(app); fail(error,@"无法分配窗口绑定资源"); return -1; }
        target->pid=pid; target->app=app; target->window=(AXUIElementRef)CFRetain((CFTypeRef)window);
        *out=target; return 1;
    }
}

int jt_window_guard(jt_input_target *target, pid_t frontPid, char **error) {
    @autoreleasepool {
        if (!target || frontPid!=target->pid) { fail(error,@"纠错期间切换了应用，结果未上屏"); return 0; }
        id window=attribute(target->app,kAXFocusedWindowAttribute);
        if (!window || CFGetTypeID((CFTypeRef)window)!=AXUIElementGetTypeID() || !CFEqual((CFTypeRef)window,target->window)) {
            fail(error,@"纠错期间切换了窗口，结果未上屏"); return 0;
        }
        return 1;
    }
}

int jt_input_capture(pid_t pid, jt_input_target **out, char **data, char **error) {
    @autoreleasepool {
        if (!AXIsProcessTrusted()) { fail(error,@"请给启动 JustTalk 的终端辅助功能权限，才能读取输入框"); return -1; }
        if (pid<=0) { fail(error,@"输入应用 PID 无效"); return -1; }
        AXUIElementRef app=AXUIElementCreateApplication(pid);
        AXUIElementSetMessagingTimeout(app,0.2);
        // Query the standard role first, as required by Chromium clients.
        AXUIElementRef editor=focusedWithBudget(app,error,0);
        if (!editor || !inputFrontmost(app,error)) { CFRelease(app); return -1; }
        NSString *role=attribute(editor,kAXRoleAttribute);
        if (![@[(id)kAXTextAreaRole,(id)kAXTextFieldRole] containsObject:role] ||
            [attribute(editor,kAXSubroleAttribute) isEqual:(id)kAXSecureTextFieldSubrole]) {
            CFRelease(app); return 0;
        }
        Boolean writable=false;
        AXError writableStatus=AXUIElementIsAttributeSettable(editor,kAXValueAttribute,&writable);
        if (writableStatus!=kAXErrorSuccess && writableStatus!=kAXErrorAttributeUnsupported && writableStatus!=kAXErrorNotImplemented) {
            CFRelease(app); fail(error,[NSString stringWithFormat:@"输入框可编辑状态读取失败（AX %d）",writableStatus]); return -1;
        }
        if (writableStatus!=kAXErrorSuccess || !writable) {
            CFRelease(app); return 0;
        }
        pid_t editorPid=0;
        if (AXUIElementGetPid(editor,&editorPid)!=kAXErrorSuccess || editorPid!=pid) {
            CFRelease(app); fail(error,@"输入框不属于当前输入应用"); return -1;
        }
        NSString *value=attribute(editor,kAXValueAttribute);
        if (![value isKindOfClass:[NSString class]] || value.length>4096) {
            CFRelease(app); fail(error,@"输入框文本不可读取或超过 4096 字符"); return -1;
        }
        CFRange selection;
        if (!inputSelection(editor,value,&selection,error)) { CFRelease(app); return -1; }
        id window=attribute(editor,kAXWindowAttribute);
        if (!window || CFGetTypeID((CFTypeRef)window)!=AXUIElementGetTypeID()) {
            CFRelease(app); fail(error,@"无法确认输入框所属窗口"); return -1;
        }
        NSData *json=[NSJSONSerialization dataWithJSONObject:@{@"value":value,@"role":role,
            @"prefix":[value substringToIndex:selection.location],
            @"suffix":[value substringFromIndex:selection.location+selection.length]} options:0 error:nil];
        if (!json) { CFRelease(app); fail(error,@"输入框快照编码失败"); return -1; }
        jt_input_target *target=calloc(1,sizeof(*target));
        if (!target) { CFRelease(app); fail(error,@"无法分配输入框读取资源"); return -1; }
        target->pid=pid; target->app=app;
        target->editor=(AXUIElementRef)CFRetain(editor); target->window=(AXUIElementRef)CFRetain((CFTypeRef)window);
        target->before=[value copy]; target->selection=selection;
        *data=strdup([[[NSString alloc] initWithData:json encoding:NSUTF8StringEncoding] autorelease].UTF8String);
        *out=target; return 1;
    }
}

static NSString *inputValue(jt_input_target *target, char **error) {
    if (!target || !inputFrontmost(target->app,error)) return nil;
    AXUIElementRef editor=focusedWithBudget(target->app,error,0);
    if (!editor) return nil;
    id window=attribute(editor,kAXWindowAttribute);
    if (!CFEqual(editor,target->editor) || !window || CFGetTypeID((CFTypeRef)window)!=AXUIElementGetTypeID() ||
        !CFEqual((CFTypeRef)window,target->window)) {
        fail(error,@"输入框或窗口已切换，停止读取本次语音修订"); return nil;
    }
    NSString *value=attribute(editor,kAXValueAttribute);
    if (![value isKindOfClass:[NSString class]] || value.length>4096) {
        fail(error,@"绑定输入框的文本读取失败或过长"); return nil;
    }
    return value;
}

int jt_input_read(jt_input_target *target, char **text, char **error) {
    @autoreleasepool {
        NSString *value=inputValue(target,error);
        if (!value) return -1;
        *text=strdup(value.UTF8String); return *text ? 1 : -1;
    }
}

int jt_input_guard(jt_input_target *target, char **error) {
    @autoreleasepool {
        NSString *value=inputValue(target,error);
        if (!value) return 0;
        CFRange selection;
        if (![value isEqual:target->before] || !inputSelection(target->editor,value,&selection,error) ||
            selection.location!=target->selection.location || selection.length!=target->selection.length) {
            if (!*error) fail(error,@"纠错期间输入文字或选区发生变化，结果未上屏"); return 0;
        }
        return 1;
    }
}

void jt_input_release(jt_input_target *target) {
    if (!target) return;
    CFRelease(target->app); if (target->editor) CFRelease(target->editor); CFRelease(target->window);
    [target->before release]; free(target);
}

static NSString *messageRole(NSString *heading) {
    if ([heading isEqualToString:@"You said:"] || [heading isEqualToString:@"你说："] || [heading isEqualToString:@"你說："]) return @"user";
    if ([heading isEqualToString:@"ChatGPT said:"] || [heading isEqualToString:@"ChatGPT 说："] || [heading isEqualToString:@"ChatGPT 說："]) return @"assistant";
    return nil;
}

static NSArray *node(AXUIElementRef element, char **error) {
    NSArray *keys = @[(id)kAXRoleAttribute, (id)kAXValueAttribute, (id)kAXTitleAttribute,
                     (id)kAXDescriptionAttribute, (id)kAXChildrenAttribute];
    CFArrayRef values = NULL;
    if (AXUIElementCopyMultipleAttributeValues(element, (CFArrayRef)keys, 0, &values) != kAXErrorSuccess || !values) {
        fail(error, @"Codex 的辅助功能结构读取失败，未使用不完整上下文"); return nil;
    }
    return [(NSArray *)values autorelease];
}

static NSString *stringValue(id value) { return [value isKindOfClass:[NSString class]] ? value : @""; }
static NSArray *children(NSArray *values) { return [values[4] isKindOfClass:[NSArray class]] ? values[4] : @[]; }

static NSString *headingRole(NSArray *values, int depth, char **error) {
    for (int i=1; i<4; i++) { NSString *role=messageRole(stringValue(values[i])); if (role) return role; }
    if (depth<2) for (id child in children(values)) {
        NSArray *descendant=node((AXUIElementRef)child,error); if (!descendant) return nil;
        NSString *role=headingRole(descendant,depth+1,error); if (role) return role;
    }
    return nil;
}

static void flushMessage(NSMutableArray *messages, NSString *role, NSMutableArray *parts) {
    if (role && parts.count) {
        [messages addObject:@{@"role":role,@"text":[parts componentsJoinedByString:@"\n"]}];
        if (messages.count>100) [messages removeObjectAtIndex:0];
    }
    [parts removeAllObjects];
}

static BOOL scan(AXUIElementRef element, AXUIElementRef editor, int depth, NSUInteger *count,
                 NSTimeInterval deadline, NSMutableArray *messages, NSMutableArray *parts,
                 NSString **currentRole, int *headingDepth, AXUIElementRef *lastUserHeading, char **error) {
    if (depth>=60 || ++(*count)>18000 || [NSDate timeIntervalSinceReferenceDate]>deadline) {
        fail(error,@"当前聊天读取超过 2 秒或结构过大，未使用不完整上下文"); return NO;
    }
    if (CFEqual(element,editor)) return YES;
    NSArray *values=node(element,error); if (!values) return NO;
    NSString *role=stringValue(values[0]);
    if ([@[@"AXButton",@"AXTextField",@"AXTextArea",@"AXMenu",@"AXToolbar"] containsObject:role]) return YES;
    if ([role isEqualToString:@"AXHeading"]) {
        NSString *next=headingRole(values,0,error); if (*error) return NO;
        if (next) {
            flushMessage(messages,*currentRole,parts); *currentRole=next; *headingDepth=depth;
            if ([next isEqualToString:@"user"]) *lastUserHeading=element;
            return YES;
        }
    }
    NSString *text=stringValue(values[1]);
    // Codex exposes the message body inside groups below its role heading,
    // but elapsed clocks as peer static text in the transcript list. A flat
    // scan previously appended those clocks and later tool activity to the
    // preceding user message. End at that structural boundary, not by matching
    // clock-like text (a user's actual "12:34" must remain intact).
    if ([*currentRole isEqual:@"user"] && [role isEqual:@"AXStaticText"] && depth<=*headingDepth) {
        if (parts.count) { flushMessage(messages,*currentRole,parts); *currentRole=nil; }
        return YES;
    }
    if (*currentRole && [role isEqualToString:@"AXStaticText"] && text.length) [parts addObject:text];
    for (id child in children(values)) if (!scan((AXUIElementRef)child,editor,depth+1,count,deadline,messages,parts,currentRole,headingDepth,lastUserHeading,error)) return NO;
    return YES;
}

static NSArray *captureMessages(AXUIElementRef main, AXUIElementRef editor, AXUIElementRef *lastUserHeading,
                               char **error) {
    NSMutableArray *messages=[NSMutableArray array],*parts=[NSMutableArray array];
    NSUInteger count=0; NSString *role=nil; int headingDepth=0;
    if (!scan(main,editor,0,&count,[NSDate timeIntervalSinceReferenceDate]+2,messages,parts,&role,&headingDepth,lastUserHeading,error)) return nil;
    flushMessage(messages,role,parts);
    if (!messages.count) { fail(error,@"当前 Codex 聊天没有提供消息角色与正文，未执行无上下文纠错"); return nil; }
    return messages;
}

static NSArray *userMessages(NSArray *messages) {
    NSArray *users=[messages filteredArrayUsingPredicate:[NSPredicate predicateWithFormat:@"role == 'user'"]];
    return users.count>6 ? [users subarrayWithRange:NSMakeRange(users.count-6,6)] : users;
}

// AX object identity is a binding signal, not proof that the user switched
// chats. Keep the existing guard, but report its actual observations separately.
static BOOL userBindingMatches(jt_correction_target *target, AXUIElementRef anchor,
                               NSArray *users, char **error) {
    BOOL sameAnchor=anchor && CFEqual(anchor,target->userAnchor);
    BOOL sameText=[users isEqual:target->messages];
    if (sameAnchor && sameText) return YES;
    NSString *reason=!anchor ? @"用户消息控件不可用" :
        !sameAnchor && sameText ? @"用户消息控件已变化，正文相同" :
        sameAnchor ? @"用户消息正文已变化，控件相同" : @"用户消息控件和正文均已变化";
    fail(error,[NSString stringWithFormat:@"%@（消息数 %lu→%lu），无法确认同一聊天，结果未上屏",
                reason,(unsigned long)target->messages.count,(unsigned long)users.count]);
    return NO;
}

static AXUIElementRef mainAncestor(AXUIElementRef editor) {
    AXUIElementRef current=editor;
    for (int i=0;i<40;i++) {
        if ([attribute(current,kAXSubroleAttribute) isEqual:@"AXLandmarkMain"]) return current;
        id parent=attribute(current,kAXParentAttribute);
        if (!parent || CFGetTypeID((CFTypeRef)parent)!=AXUIElementGetTypeID()) break;
        current=(AXUIElementRef)parent;
    }
    return NULL;
}

int jt_correction_capture(pid_t pid, jt_correction_target **out, char **data, char **error) {
    @autoreleasepool {
        if (!AXIsProcessTrusted()) { fail(error,@"请给启动 JustTalk 的终端辅助功能权限，才能确认当前输入应用"); return -1; }
        if (pid<=0) { fail(error,@"当前输入应用 PID 无效"); return -1; }
        NSRunningApplication *front=[NSRunningApplication runningApplicationWithProcessIdentifier:pid];
        if (!front.bundleIdentifier.length) {
            fail(error,@"无法确认当前输入应用身份，未执行纠错或自动上屏"); return -1;
        }
        if (![front.bundleIdentifier isEqualToString:@"com.openai.codex"]) return 0;
        AXUIElementRef system=AXUIElementCreateSystemWide();
        AXUIElementSetMessagingTimeout(system,0.2); CFRelease(system);
        AXUIElementRef app=AXUIElementCreateApplication(pid);
        AXUIElementSetMessagingTimeout(app,0.2);
        AXUIElementRef editor=focused(app,error);
        if (!editor) { CFRelease(app); return -1; }
        AXUIElementRef main=mainAncestor(editor);
        NSString *draft=attribute(editor,kAXValueAttribute);
        if (![attribute(editor,kAXRoleAttribute) isEqual:@"AXTextArea"] || ![draft isKindOfClass:[NSString class]] || !main) {
            CFRelease(app); fail(error,@"请把光标放在 Codex 草稿框；未识别到同一聊天主区域"); return -1;
        }
        AXUIElementRef userAnchor=NULL;
        NSArray *messages=captureMessages(main,editor,&userAnchor,error);
        if (!messages) { CFRelease(app); return -1; }
        if (!userAnchor) { CFRelease(app); fail(error,@"当前聊天没有提供用户消息锚点，无法绑定自动上屏目标"); return -1; }
        NSData *json=[NSJSONSerialization dataWithJSONObject:messages options:0 error:nil];
        if (!json) { CFRelease(app); fail(error,@"当前聊天无法转成文本上下文"); return -1; }
        jt_correction_target *target=calloc(1,sizeof(*target));
        if (!target) { CFRelease(app); fail(error,@"无法分配聊天读取资源"); return -1; }
        target->pid=pid; target->app=app;
        target->editor=(AXUIElementRef)CFRetain(editor); target->main=(AXUIElementRef)CFRetain(main);
        target->userAnchor=(AXUIElementRef)CFRetain(userAnchor);
        target->draft=[draft copy]; target->messages=[userMessages(messages) copy];
        *data=strdup([[[NSString alloc] initWithData:json encoding:NSUTF8StringEncoding] autorelease].UTF8String);
        *out=target; return 1;
    }
}

int jt_correction_guard(jt_correction_target *target, pid_t frontPid, char **error) {
    @autoreleasepool {
        if (frontPid!=target->pid) { fail(error,@"纠错期间切换了应用，结果未上屏"); return 0; }
        AXUIElementRef editor=focused(target->app,error);
        if (!editor) return 0;
        if (!CFEqual(editor,target->editor)) {
            fail(error,@"纠错期间切换了应用或输入框，结果未上屏"); return 0;
        }
        if (![attribute(editor,kAXValueAttribute) isEqual:target->draft]) {
            fail(error,@"纠错期间草稿发生变化，结果未上屏"); return 0;
        }
        AXUIElementRef main=mainAncestor(editor);
        if (!main || !CFEqual(main,target->main)) { fail(error,@"纠错期间切换了聊天，结果未上屏"); return 0; }
        AXUIElementRef userAnchor=NULL;
        NSArray *messages=captureMessages(main,editor,&userAnchor,error);
        if (!messages) return 0;
        // 助手流式回复可继续增长；绑定用户消息的 AX 身份与正文，避免写入切换后的聊天。
        return userBindingMatches(target,userAnchor,userMessages(messages),error) ? 1 : 0;
    }
}

void jt_correction_release(jt_correction_target *target) {
    if (!target) return;
    CFRelease(target->app); CFRelease(target->editor); CFRelease(target->main); CFRelease(target->userAnchor);
    [target->draft release]; [target->messages release]; free(target);
}
