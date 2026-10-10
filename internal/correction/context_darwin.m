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

static void fail(char **error, NSString *message) { *error = strdup([message UTF8String]); }

static id attribute(AXUIElementRef element, CFStringRef name) {
    CFTypeRef value = NULL;
    if (AXUIElementCopyAttributeValue(element, name, &value) != kAXErrorSuccess) return nil;
    return [(id)value autorelease];
}

static AXUIElementRef focused(AXUIElementRef app) {
    id value = attribute(app, kAXFocusedUIElementAttribute);
    return value && CFGetTypeID((CFTypeRef)value) == AXUIElementGetTypeID() ? (AXUIElementRef)value : NULL;
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
                 NSString **currentRole, AXUIElementRef *lastUserHeading, char **error) {
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
            flushMessage(messages,*currentRole,parts); *currentRole=next;
            if ([next isEqualToString:@"user"]) *lastUserHeading=element;
            return YES;
        }
    }
    NSString *text=stringValue(values[1]);
    if (*currentRole && [role isEqualToString:@"AXStaticText"] && text.length) [parts addObject:text];
    for (id child in children(values)) if (!scan((AXUIElementRef)child,editor,depth+1,count,deadline,messages,parts,currentRole,lastUserHeading,error)) return NO;
    return YES;
}

static NSArray *captureMessages(AXUIElementRef main, AXUIElementRef editor, AXUIElementRef *lastUserHeading, char **error) {
    NSMutableArray *messages=[NSMutableArray array],*parts=[NSMutableArray array];
    NSUInteger count=0; NSString *role=nil;
    if (!scan(main,editor,0,&count,[NSDate timeIntervalSinceReferenceDate]+2,messages,parts,&role,lastUserHeading,error)) return nil;
    flushMessage(messages,role,parts);
    if (!messages.count) { fail(error,@"当前 Codex 聊天没有提供消息角色与正文，未执行无上下文纠错"); return nil; }
    return messages;
}

static NSArray *userMessages(NSArray *messages) {
    NSArray *users=[messages filteredArrayUsingPredicate:[NSPredicate predicateWithFormat:@"role == 'user'"]];
    return users.count>6 ? [users subarrayWithRange:NSMakeRange(users.count-6,6)] : users;
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
        AXUIElementRef editor=focused(app),main=editor?mainAncestor(editor):NULL;
        NSString *draft=editor?attribute(editor,kAXValueAttribute):nil;
        if (!editor || ![attribute(editor,kAXRoleAttribute) isEqual:@"AXTextArea"] || ![draft isKindOfClass:[NSString class]] || !main) {
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
        AXUIElementRef editor=focused(target->app);
        if (!editor || !CFEqual(editor,target->editor)) {
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
        if (!userAnchor || !CFEqual(userAnchor,target->userAnchor) || ![userMessages(messages) isEqual:target->messages]) {
            fail(error,@"纠错期间切换聊天或用户消息发生变化，结果未上屏"); return 0;
        }
        return 1;
    }
}

void jt_correction_release(jt_correction_target *target) {
    if (!target) return;
    CFRelease(target->app); CFRelease(target->editor); CFRelease(target->main); CFRelease(target->userAnchor);
    [target->draft release]; [target->messages release]; free(target);
}
