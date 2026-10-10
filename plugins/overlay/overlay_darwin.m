//go:build darwin && cgo

#include "overlay_darwin.h"

#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>
#import <QuartzCore/QuartzCore.h>
#include <dispatch/dispatch.h>
#include <math.h>
#include <pthread.h>
#include <stdlib.h>
#include <string.h>

@class JTOverlayView;
typedef struct {
    NSPanel *panel;
    JTOverlayView *view;
    char position[32];
    CGFloat scale;
} jt_overlay_t;
static void jt_overlay_move(jt_overlay_t *overlay);

static NSColor *jt_rgb(CGFloat r, CGFloat g, CGFloat b, CGFloat a) {
    return [NSColor colorWithSRGBRed:r/255 green:g/255 blue:b/255 alpha:a];
}
static CGFloat jt_smooth(double x) {
    x=fmax(0,fmin(1,x)); return x*x*(3-2*x);
}
static void jt_text(NSString *text, NSRect rect, CGFloat size, NSColor *color, NSFontWeight weight) {
    [text drawInRect:rect withAttributes:@{NSFontAttributeName:[NSFont systemFontOfSize:size weight:weight],NSForegroundColorAttributeName:color}];
}

@interface JTOverlayView : NSView {
    NSString *label, *transcript, *caption, *originalTranscript;
    NSUInteger captionOffset;
    BOOL captionEllipsis, previousAI, reducedMotion, showing, hiding;
    CGFloat scale, audioLevel, targetLevel, appearFromAlpha;
    CFTimeInterval phaseAt, sizeAt, appearAt, hideAt, levelAt;
    NSSize fromSize, targetSize;
    NSTimer *timer;
    jt_overlay_t *owner;
}
- (void)setScaleValue:(CGFloat)newScale;
- (void)setLabel:(NSString *)newLabel text:(NSString *)text red:(CGFloat)r green:(CGFloat)g blue:(CGFloat)b;
- (void)attach:(jt_overlay_t *)overlay;
- (void)setAudioLevel:(CGFloat)level;
- (NSSize)preferredSize;
- (NSDictionary *)bodyAttributes;
- (void)beginShowing;
- (void)beginHiding;
- (void)stopAnimation;
@end

@implementation JTOverlayView
- (instancetype)initWithFrame:(NSRect)frame {
    if ((self=[super initWithFrame:frame])) {
        label=[@"IDL" retain]; transcript=[@"" retain]; caption=[@"" retain];
        scale=1; fromSize=targetSize=NSMakeSize(158,52);
        reducedMotion=[[NSWorkspace sharedWorkspace] accessibilityDisplayShouldReduceMotion];
        phaseAt=sizeAt=CACurrentMediaTime();
    }
    return self;
}
- (void)dealloc {
    [self stopAnimation];
    [label release]; [transcript release]; [caption release]; [originalTranscript release];
    [super dealloc];
}
- (BOOL)isOpaque { return NO; }
- (BOOL)isFlipped { return YES; }
- (void)attach:(jt_overlay_t *)overlay { owner=overlay; }
- (void)setScaleValue:(CGFloat)newScale {
    scale=newScale<=0?1:newScale;
    fromSize=targetSize=NSMakeSize(158*scale,52*scale);
}
- (NSDictionary *)bodyAttributes {
    NSMutableParagraphStyle *paragraph=[[[NSMutableParagraphStyle alloc] init] autorelease];
    paragraph.lineBreakMode=NSLineBreakByWordWrapping;
    paragraph.lineSpacing=2*scale;
    return @{NSFontAttributeName:[NSFont systemFontOfSize:15*scale],
             NSForegroundColorAttributeName:jt_rgb(236,238,244,1),NSParagraphStyleAttributeName:paragraph};
}
- (void)updateCaption {
    NSUInteger offset=0;
    NSString *visible=transcript;
    BOOL truncated=NO;
    while (visible.length) {
        NSString *candidate=truncated?[@"…" stringByAppendingString:visible]:visible;
        NSRect measured=[candidate boundingRectWithSize:NSMakeSize(406*scale,CGFLOAT_MAX)
            options:NSStringDrawingUsesLineFragmentOrigin|NSStringDrawingUsesFontLeading attributes:[self bodyAttributes]];
        if (ceil(measured.size.height)<=39*scale) break;
        NSRange first=[visible rangeOfComposedCharacterSequenceAtIndex:0];
        offset+=NSMaxRange(first);
        visible=[visible substringFromIndex:NSMaxRange(first)];
        truncated=YES;
    }
    captionOffset=offset; captionEllipsis=truncated;
    [caption release]; caption=[(truncated?[@"…" stringByAppendingString:visible]:visible) copy];
}
- (void)setLabel:(NSString *)newLabel text:(NSString *)text red:(CGFloat)r green:(CGFloat)g blue:(CGFloat)b {
    (void)r; (void)g; (void)b;
    CFTimeInterval now=CACurrentMediaTime();
    BOOL changed=![label isEqualToString:newLabel];
    if (changed) {
        previousAI=[label isEqualToString:@"FIX"];
        phaseAt=now;
        if ([newLabel isEqualToString:@"FIX"]) {
            [originalTranscript release]; originalTranscript=[text copy];
        } else if ([newLabel isEqualToString:@"CON"] || [newLabel isEqualToString:@"REC"]) {
            [originalTranscript release]; originalTranscript=nil;
        }
        [label release]; label=[newLabel copy];
    }
    NSString *body=text?:@"";
    if (![transcript isEqualToString:body]) {
        [transcript release]; transcript=[body copy]; [self updateCaption];
    }
    reducedMotion=[[NSWorkspace sharedWorkspace] accessibilityDisplayShouldReduceMotion];
    BOOL compact=([label isEqualToString:@"CON"] || [label isEqualToString:@"IDL"]) && !transcript.length;
    NSSize next=NSMakeSize((compact?158:448)*scale,(compact?52:104)*scale);
    if (!NSEqualSizes(next,targetSize)) { fromSize=[self preferredSize]; targetSize=next; sizeAt=now; }
    [self setNeedsDisplay:YES];
}
- (NSSize)preferredSize {
    CGFloat p=reducedMotion?1:jt_smooth((CACurrentMediaTime()-sizeAt)/0.25);
    return NSMakeSize(fromSize.width+(targetSize.width-fromSize.width)*p,fromSize.height+(targetSize.height-fromSize.height)*p);
}
- (void)setAudioLevel:(CGFloat)level { targetLevel=fmax(0,fmin(1,level)); levelAt=CACurrentMediaTime(); }
- (void)startAnimation {
    if (!timer) {
        timer=[[NSTimer timerWithTimeInterval:1.0/30 target:self selector:@selector(tick:) userInfo:nil repeats:YES] retain];
        [[NSRunLoop mainRunLoop] addTimer:timer forMode:NSRunLoopCommonModes];
    }
}
- (void)stopAnimation { [timer invalidate]; [timer release]; timer=nil; }
- (void)beginShowing {
    if (!showing || hiding) {
        appearFromAlpha=showing?[owner->panel alphaValue]:0;
        appearAt=CACurrentMediaTime();
    }
    showing=YES; hiding=NO;
    [self startAnimation]; [self tick:nil];
}
- (void)beginHiding {
    if (!showing || hiding) return;
    hiding=YES; hideAt=CACurrentMediaTime(); [self startAnimation];
}
- (void)tick:(NSTimer *)sender {
    (void)sender;
    CFTimeInterval now=CACurrentMediaTime();
    if (now-levelAt>0.5) targetLevel=0;
    audioLevel+=(targetLevel-audioLevel)*0.3;
    if (!owner) return;
    CGFloat alpha=1;
    if (hiding) {
        alpha=reducedMotion?0:1-jt_smooth((now-hideAt)/0.18);
        if (alpha<=0) { [owner->panel orderOut:nil]; showing=hiding=NO; [self stopAnimation]; return; }
    } else {
        CGFloat p=reducedMotion?1:jt_smooth((now-appearAt)/0.18);
        alpha=appearFromAlpha+(1-appearFromAlpha)*p;
    }
    [owner->panel setAlphaValue:alpha];
    jt_overlay_move(owner);
    [self setNeedsDisplay:YES];
    BOOL busy=[@[@"CON",@"REC",@"STP",@"WAI",@"FIX"] containsObject:label];
    if (!hiding && now-fmax(fmax(phaseAt,sizeAt),appearAt)>0.35 && (!busy || reducedMotion)) [self stopAnimation];
}
- (NSString *)title {
    return [@{@"CON":@"准备中",@"REC":@"录音中",@"STP":@"收尾中",@"WAI":@"识别中",
              @"FIX":@"AI 整理中",@"DONE":@"已完成",@"ERR":@"处理失败",@"IDL":@"待机"} objectForKey:label]?:label;
}
- (NSColor *)accent {
    if ([label isEqualToString:@"REC"]) return jt_rgb(255,115,127,1);
    if ([label isEqualToString:@"FIX"]) return jt_rgb(183,161,255,1);
    if ([label isEqualToString:@"DONE"]) return jt_rgb(115,225,182,1);
    if ([label isEqualToString:@"ERR"]) return jt_rgb(255,177,111,1);
    if ([label isEqualToString:@"IDL"]) return jt_rgb(145,145,145,1);
    return jt_rgb(235,199,126,1);
}
- (void)drawIcon:(NSRect)rect time:(double)time {
    NSColor *color=[self accent];
    if ([label isEqualToString:@"REC"]) {
        for (int i=0;i<5;i++) {
            CGFloat p=reducedMotion?0.7:0.45+0.55*(sin(time*6+i*0.9)+1)/2;
            CGFloat h=(4+14*audioLevel*p)*scale;
            [color setFill]; [[NSBezierPath bezierPathWithRoundedRect:NSMakeRect(rect.origin.x+i*4*scale,NSMidY(rect)-h/2,2.5*scale,h) xRadius:1.25*scale yRadius:1.25*scale] fill];
        }
        return;
    }
    if ([@[@"CON",@"STP",@"WAI"] containsObject:label]) {
        for (int i=0;i<3;i++) {
            CGFloat p=reducedMotion?0.75:0.35+0.65*(sin(time*4-i*0.8)+1)/2;
            [[color colorWithAlphaComponent:p] setFill]; [[NSBezierPath bezierPathWithOvalInRect:NSMakeRect(rect.origin.x+i*7*scale,NSMidY(rect)-2*scale,4*scale,4*scale)] fill];
        }
        return;
    }
    NSString *name=[label isEqualToString:@"FIX"]?@"sparkles":([label isEqualToString:@"DONE"]?@"checkmark":([label isEqualToString:@"ERR"]?@"exclamationmark":@"mic"));
    NSImageSymbolConfiguration *configuration=[[NSImageSymbolConfiguration configurationWithPointSize:19*scale weight:NSFontWeightMedium]
        configurationByApplyingConfiguration:[NSImageSymbolConfiguration configurationWithPaletteColors:@[color]]];
    NSImage *image=[[NSImage imageWithSystemSymbolName:name accessibilityDescription:nil] imageWithSymbolConfiguration:configuration];
    CGFloat opacity=[label isEqualToString:@"FIX"]&&!reducedMotion?0.72+0.28*(sin(time*2.7)+1)/2:1;
    [image drawInRect:rect fromRect:NSZeroRect operation:NSCompositingOperationSourceOver fraction:opacity respectFlipped:YES hints:nil];
}
- (void)drawRim:(NSRect)rect radius:(CGFloat)radius time:(double)time opacity:(CGFloat)opacity {
    NSBezierPath *outline=[NSBezierPath bezierPathWithRoundedRect:rect xRadius:radius yRadius:radius];
    CGFloat breath=reducedMotion?0.45:0.42+0.18*sin(time*2.1);
    [jt_rgb(169,142,249,opacity*breath) setStroke]; outline.lineWidth=1.4*scale; [outline stroke];
    if (reducedMotion) return;
    NSBezierPath *rim=[NSBezierPath bezierPathWithRoundedRect:rect xRadius:radius yRadius:radius];
    [rim appendBezierPath:[NSBezierPath bezierPathWithRoundedRect:NSInsetRect(rect,2*scale,2*scale) xRadius:radius-2*scale yRadius:radius-2*scale]];
    rim.windingRule=NSWindingRuleEvenOdd;
    [NSGraphicsContext saveGraphicsState]; [rim addClip];
    NSPoint point=NSMakePoint(NSMidX(rect)+cos(time*1.7)*(rect.size.width/2-8*scale),NSMidY(rect)+sin(time*1.7)*(rect.size.height/2-4*scale));
    NSGradient *gradient=[[[NSGradient alloc] initWithColors:@[jt_rgb(212,197,255,opacity*0.8),jt_rgb(133,149,255,opacity*0.4),jt_rgb(133,149,255,0)]] autorelease];
    [gradient drawFromCenter:point radius:0 toCenter:point radius:86*scale options:NSGradientDrawsBeforeStartingLocation|NSGradientDrawsAfterEndingLocation];
    [NSGraphicsContext restoreGraphicsState];
}
- (NSRange)changedRange {
    if (!originalTranscript || !transcript.length || [originalTranscript isEqualToString:transcript]) return NSMakeRange(NSNotFound,0);
    NSUInteger prefix=0,common=MIN(originalTranscript.length,transcript.length);
    while (prefix<common && [originalTranscript characterAtIndex:prefix]==[transcript characterAtIndex:prefix]) prefix++;
    NSUInteger suffix=0;
    while (suffix<common-prefix && [originalTranscript characterAtIndex:originalTranscript.length-1-suffix]==[transcript characterAtIndex:transcript.length-1-suffix]) suffix++;
    NSRange range=NSMakeRange(prefix,transcript.length-prefix-suffix);
    return range.length?[transcript rangeOfComposedCharacterSequencesForRange:range]:NSMakeRange(NSNotFound,0);
}
- (void)drawRect:(NSRect)dirtyRect {
    (void)dirtyRect;
    NSRect rect=NSInsetRect(self.bounds,0.5*scale,0.5*scale);
    BOOL wide=rect.size.width>310*scale;
    CGFloat expansion=fmax(0,fmin(1,(rect.size.width/scale-158)/290));
    CGFloat radius=rect.size.height/2+(22*scale-rect.size.height/2)*expansion;
    NSBezierPath *pill=[NSBezierPath bezierPathWithRoundedRect:rect xRadius:radius yRadius:radius];
    double time=CACurrentMediaTime()-phaseAt;
    CGFloat entry=reducedMotion?1:jt_smooth(time/0.25);
    CGFloat emphasis=[label isEqualToString:@"FIX"]?entry:(previousAI?1-entry:0);
    [NSGraphicsContext saveGraphicsState]; [pill addClip];
    NSGradient *background=[[[NSGradient alloc] initWithColors:@[jt_rgb(32+11*emphasis,34+3*emphasis,44+33*emphasis,1),jt_rgb(22,24,31,1)]] autorelease];
    [background drawInRect:rect angle:0]; [NSGraphicsContext restoreGraphicsState];
    [jt_rgb(255,255,255,0.09+0.1*emphasis) setStroke]; pill.lineWidth=scale; [pill stroke];
    if (emphasis>0) [self drawRim:rect radius:radius time:time opacity:emphasis];
    NSRect icon=NSMakeRect(wide?21*scale:NSMidX(rect)-49*scale,17*scale,19*scale,19*scale);
    [self drawIcon:icon time:time];
    jt_text([self title],NSMakeRect(NSMaxX(icon)+10*scale,16*scale,rect.size.width-78*scale,24*scale),14*scale,
        [label isEqualToString:@"FIX"]?jt_rgb(210,198,255,1):jt_rgb(244,245,250,1),NSFontWeightSemibold);
    if (!wide) return;
    if ([label isEqualToString:@"FIX"]) {
        NSRect badge=NSMakeRect(rect.size.width-89*scale,15*scale,69*scale,23*scale);
        [jt_rgb(157,133,245,0.13) setFill]; [[NSBezierPath bezierPathWithRoundedRect:badge xRadius:7*scale yRadius:7*scale] fill];
        jt_text(@"上下文纠错",NSInsetRect(badge,8*scale,4*scale),10*scale,jt_rgb(194,177,255,1),NSFontWeightMedium);
    }
    NSMutableAttributedString *body=[[[NSMutableAttributedString alloc] initWithString:caption attributes:[self bodyAttributes]] autorelease];
    if ([label isEqualToString:@"FIX"]) [body addAttribute:NSForegroundColorAttributeName value:jt_rgb(187,181,207,1) range:NSMakeRange(0,body.length)];
    if ([label isEqualToString:@"DONE"]) {
        NSRange changed=[self changedRange];
        if (changed.location!=NSNotFound) {
            NSRange intersection=NSIntersectionRange(changed,NSMakeRange(captionOffset,transcript.length-captionOffset));
            if (intersection.length) {
                intersection.location=intersection.location-captionOffset+(captionEllipsis?1:0);
                [body addAttribute:NSForegroundColorAttributeName value:jt_rgb(142,240,193,1) range:intersection];
            }
        }
    }
    [NSGraphicsContext saveGraphicsState];
    if ([label isEqualToString:@"DONE"]) CGContextSetAlpha([[NSGraphicsContext currentContext] CGContext],entry);
    [body drawWithRect:NSMakeRect(21*scale,57*scale,406*scale,39*scale)
        options:NSStringDrawingUsesLineFragmentOrigin|NSStringDrawingUsesFontLeading context:nil];
    [NSGraphicsContext restoreGraphicsState];
}
@end


static jt_overlay_t *helper_overlay = NULL;

static void jt_overlay_on_main_sync(void (^block)(void)) {
	if (pthread_main_np()) {
		block();
		return;
	}
	dispatch_sync(dispatch_get_main_queue(), block);
}

static void jt_overlay_pump(void) {
	NSEvent *event = nil;
	do {
		event = [NSApp nextEventMatchingMask:NSEventMaskAny
			untilDate:[NSDate distantPast]
			inMode:NSDefaultRunLoopMode
			dequeue:YES];
		if (event != nil) {
			[NSApp sendEvent:event];
		}
	} while (event != nil);
}

static void jt_overlay_move(jt_overlay_t *overlay) {
	NSScreen *screen = [NSScreen mainScreen];
	if (screen == nil) return;
	NSRect frame = [screen visibleFrame];
	NSSize size = [overlay->view preferredSize];
	CGFloat w = size.width;
	CGFloat h = size.height;
	CGFloat margin = 28.0 * overlay->scale;
	CGFloat x = NSMaxX(frame) - w - margin;
	CGFloat y = NSMaxY(frame) - h - margin;

	if (strcmp(overlay->position, "top-left") == 0) {
		x = NSMinX(frame) + margin; y = NSMaxY(frame) - h - margin;
	} else if (strcmp(overlay->position, "top-center") == 0) {
		x = NSMinX(frame) + (NSWidth(frame) - w) / 2.0; y = NSMaxY(frame) - h - margin;
	} else if (strcmp(overlay->position, "notch") == 0) {
		CGFloat safeTop = MIN(NSMaxY(frame), NSMaxY([screen frame]) - [screen safeAreaInsets].top);
		x = NSMinX(frame) + (NSWidth(frame) - w) / 2.0; y = safeTop - h - 8.0 * overlay->scale;
	} else if (strcmp(overlay->position, "bottom-left") == 0) {
		x = NSMinX(frame) + margin; y = NSMinY(frame) + margin;
	} else if (strcmp(overlay->position, "bottom-center") == 0) {
		x = NSMinX(frame) + (NSWidth(frame) - w) / 2.0; y = NSMinY(frame) + margin;
	} else if (strcmp(overlay->position, "bottom-right") == 0) {
		x = NSMaxX(frame) - w - margin; y = NSMinY(frame) + margin;
	}
	[overlay->panel setFrame:NSMakeRect(x, y, w, h) display:YES];
}

void *jt_overlay_create(const char *position, double scale) {
	__block jt_overlay_t *overlay = NULL;
	double scaleCopy = scale;
	if (position == NULL || position[0] == '\0') position = "bottom-center";
	char *positionCopy = strdup(position);
	jt_overlay_on_main_sync(^{
		[NSApplication sharedApplication];
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
		[NSApp finishLaunching];

		overlay = calloc(1, sizeof(jt_overlay_t));
		if (overlay == NULL) return;
		overlay->scale = scaleCopy <= 0 ? 1.0 : scaleCopy;
		snprintf(overlay->position, sizeof(overlay->position), "%s", positionCopy == NULL ? "top-right" : positionCopy);

		CGFloat w = 158.0 * overlay->scale;
		CGFloat h = 52.0 * overlay->scale;
		overlay->panel = [[NSPanel alloc] initWithContentRect:NSMakeRect(0, 0, w, h)
			styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel
			backing:NSBackingStoreBuffered
			defer:NO];
		[overlay->panel setOpaque:NO];
		[overlay->panel setBackgroundColor:[NSColor clearColor]];
		[overlay->panel setHasShadow:YES];
		[overlay->panel setIgnoresMouseEvents:YES];
		[overlay->panel setCanHide:NO];
		[overlay->panel setHidesOnDeactivate:NO];
		[overlay->panel setReleasedWhenClosed:NO];
		[overlay->panel setLevel:NSStatusWindowLevel];
		[overlay->panel setAlphaValue:1.0];
		[overlay->panel setCollectionBehavior:
			NSWindowCollectionBehaviorCanJoinAllSpaces |
			NSWindowCollectionBehaviorStationary |
			NSWindowCollectionBehaviorFullScreenAuxiliary];

		overlay->view = [[JTOverlayView alloc] initWithFrame:NSMakeRect(0, 0, w, h)];
		[overlay->view setAutoresizingMask:NSViewWidthSizable | NSViewHeightSizable];
		[overlay->view setScaleValue:overlay->scale];
		[overlay->view attach:overlay];
		[overlay->panel setContentView:overlay->view];
		jt_overlay_move(overlay);
		[overlay->view display];
		[overlay->panel display];
		jt_overlay_pump();
	});
	if (positionCopy != NULL) free(positionCopy);
	return overlay;
}

void jt_overlay_show(void *handle, const char *labelText, const char *bodyText, unsigned short r, unsigned short g, unsigned short b) {
	jt_overlay_t *overlay = (jt_overlay_t *)handle;
	if (overlay == NULL) return;
	char *labelCopy = strdup(labelText == NULL ? "" : labelText);
	char *bodyCopy = strdup(bodyText == NULL ? "" : bodyText);
	jt_overlay_on_main_sync(^{
		NSString *text = [NSString stringWithUTF8String:labelCopy == NULL ? "" : labelCopy];
		NSString *body = [NSString stringWithUTF8String:bodyCopy == NULL ? "" : bodyCopy];
		[overlay->view setLabel:text text:body red:((CGFloat)r / 65535.0) green:((CGFloat)g / 65535.0) blue:((CGFloat)b / 65535.0)];
		jt_overlay_move(overlay);
		[overlay->view beginShowing];
		[overlay->panel orderFrontRegardless];
		[overlay->view display];
		[overlay->panel display];
		jt_overlay_pump();
		if (getenv("JUST_TALK_OVERLAY_DEBUG") != NULL) {
			NSRect frame = [overlay->panel frame];
			fprintf(stderr, "overlay visible=%d position=%s frame=%.0f,%.0f %.0fx%.0f\n",
				[overlay->panel isVisible], overlay->position, frame.origin.x, frame.origin.y, frame.size.width, frame.size.height);
		}
	});
	if (labelCopy != NULL) free(labelCopy);
	if (bodyCopy != NULL) free(bodyCopy);
}

void jt_overlay_hide(void *handle) {
	jt_overlay_t *overlay = (jt_overlay_t *)handle;
	if (overlay == NULL) return;
	jt_overlay_on_main_sync(^{
		[overlay->view beginHiding];
		jt_overlay_pump();
	});
}

void jt_overlay_close(void *handle) {
	jt_overlay_t *overlay = (jt_overlay_t *)handle;
	if (overlay == NULL) return;
	jt_overlay_on_main_sync(^{
		[overlay->view stopAnimation];
		[overlay->view attach:NULL];
		[overlay->panel orderOut:nil];
		[overlay->view release];
		[overlay->panel close];
		[overlay->panel release];
		free(overlay);
		jt_overlay_pump();
	});
}

void jt_overlay_helper_init(const char *position, double scale) {
	[NSApplication sharedApplication];
	[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
	[NSApp finishLaunching];
	helper_overlay = (jt_overlay_t *)jt_overlay_create(position, scale);
}

void jt_overlay_helper_run_app(void) {
	[NSApp run];
}

void jt_overlay_run_helper(const char *position, double scale) {
	jt_overlay_helper_init(position, scale);
	jt_overlay_helper_run_app();
}

void jt_overlay_helper_show(const char *label, const char *text, unsigned short r, unsigned short g, unsigned short b) {
	if (helper_overlay == NULL) return;
	jt_overlay_show(helper_overlay, label, text, r, g, b);
}

void jt_overlay_helper_level(double level) {
	jt_overlay_on_main_sync(^{
		if (helper_overlay != NULL) [helper_overlay->view setAudioLevel:level];
	});
}

void jt_overlay_helper_hide(void) {
	if (helper_overlay == NULL) return;
	jt_overlay_hide(helper_overlay);
}

void jt_overlay_helper_close(void) {
	if (helper_overlay != NULL) {
		jt_overlay_close(helper_overlay);
		helper_overlay = NULL;
	}
	[NSApp terminate:nil];
}
