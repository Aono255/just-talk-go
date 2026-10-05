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

@interface JTOverlayView : NSView {
	NSString *label;
	NSColor *dotColor;
	CGFloat scale;
}
- (void)setScaleValue:(CGFloat)newScale;
- (void)setLabel:(NSString *)newLabel red:(CGFloat)r green:(CGFloat)g blue:(CGFloat)b;
@end

@implementation JTOverlayView
- (instancetype)initWithFrame:(NSRect)frame {
	self = [super initWithFrame:frame];
	if (self) {
		label = [@"IDLE" retain];
		dotColor = [[NSColor colorWithCalibratedRed:0.57 green:0.57 blue:0.57 alpha:1.0] retain];
		scale = 1.0;
	}
	return self;
}
- (void)dealloc {
	[label release];
	[dotColor release];
	[super dealloc];
}
- (BOOL)isOpaque { return NO; }
- (void)setScaleValue:(CGFloat)newScale {
	scale = newScale <= 0 ? 1.0 : newScale;
	[self setNeedsDisplay:YES];
}
- (void)setLabel:(NSString *)newLabel red:(CGFloat)r green:(CGFloat)g blue:(CGFloat)b {
	[label release];
	label = [newLabel retain];
	[dotColor release];
	dotColor = [[NSColor colorWithCalibratedRed:r green:g blue:b alpha:1.0] retain];
	[self setNeedsDisplay:YES];
}
- (void)drawRect:(NSRect)dirtyRect {
	(void)dirtyRect;
	NSRect bounds = [self bounds];
	NSBezierPath *pill = [NSBezierPath bezierPathWithRoundedRect:bounds
		xRadius:NSHeight(bounds) / 2.0 yRadius:NSHeight(bounds) / 2.0];
	[[NSColor colorWithCalibratedRed:0.08 green:0.08 blue:0.08 alpha:0.94] setFill];
	[pill fill];

	CGFloat dotSize = 14.0 * scale;
	CGFloat dotX = 20.0 * scale;
	CGFloat dotY = (NSHeight(bounds) - dotSize) / 2.0;

	NSDictionary *attrs = @{
		NSFontAttributeName: [NSFont boldSystemFontOfSize:13.0 * scale],
		NSForegroundColorAttributeName: [NSColor colorWithCalibratedRed:0.96 green:0.96 blue:0.96 alpha:1.0],
	};
	NSSize textSize = [label sizeWithAttributes:attrs];
	CGFloat gap = 14.0 * scale;
	CGFloat contentW = dotSize + gap + textSize.width;
	dotX = (NSWidth(bounds) - contentW) / 2.0;
	if (dotX < 0) dotX = 0;
	NSBezierPath *dot = [NSBezierPath bezierPathWithOvalInRect:NSMakeRect(dotX, dotY, dotSize, dotSize)];
	[dotColor setFill];
	[dot fill];
	CGFloat textX = dotX + dotSize + gap;
	CGFloat textY = (NSHeight(bounds) - textSize.height) / 2.0;
	[label drawAtPoint:NSMakePoint(textX, textY) withAttributes:attrs];
}
@end

// ---------------------------------------------------------------------------
// Notch overlay: a black shape that grows out of the camera housing.

@interface JTNotchPanel : NSPanel
@end

@implementation JTNotchPanel
- (BOOL)canBecomeKeyWindow { return NO; }
- (BOOL)canBecomeMainWindow { return NO; }
- (NSRect)constrainFrameRect:(NSRect)frameRect toScreen:(NSScreen *)screen {
	(void)screen;
	return frameRect;
}
@end

typedef enum {
	JTNotchCollapsed,
	JTNotchEars,
	JTNotchText,
} JTNotchPhase;

typedef enum {
	JTAnimNone,
	JTAnimSpring,
	JTAnimEaseIn,
	JTAnimEaseOut,
} JTAnimCurve;

typedef struct {
	CGFloat w, h, bottomR, flareR;
} jt_notch_shape;

typedef enum {
	JTTextNormal,
	JTTextError,
	JTTextPlaceholder,
} JTTextStyle;

#define JT_BARS 5

static CGPathRef jt_notch_path(CGFloat cx, CGFloat top, jt_notch_shape s) {
	// Every shape uses the same element sequence so CA can interpolate paths.
	const CGFloat k = 0.5522847498;
	CGFloat l = cx - s.w / 2.0, r = cx + s.w / 2.0, b = top - s.h;
	CGFloat fr = s.flareR, br = s.bottomR;
	CGMutablePathRef p = CGPathCreateMutable();
	CGPathMoveToPoint(p, NULL, l - fr, top);
	CGPathAddCurveToPoint(p, NULL, l - fr + k * fr, top, l, top - fr + k * fr, l, top - fr);
	CGPathAddLineToPoint(p, NULL, l, b + br);
	CGPathAddCurveToPoint(p, NULL, l, b + br - k * br, l + br - k * br, b, l + br, b);
	CGPathAddLineToPoint(p, NULL, r - br, b);
	CGPathAddCurveToPoint(p, NULL, r - br + k * br, b, r, b + br - k * br, r, b + br);
	CGPathAddLineToPoint(p, NULL, r, top - fr);
	CGPathAddCurveToPoint(p, NULL, r, top - fr + k * fr, r + fr - k * fr, top, r + fr, top);
	CGPathCloseSubpath(p);
	return p;
}

static CAAnimation *jt_make_animation(NSString *key, id from, id to, JTAnimCurve curve, CFTimeInterval duration) {
	CABasicAnimation *anim;
	if (curve == JTAnimSpring) {
		CASpringAnimation *spring = [CASpringAnimation animationWithKeyPath:key];
		spring.mass = 1.0;
		spring.stiffness = 320.0;
		spring.damping = 26.0;
		spring.duration = spring.settlingDuration;
		anim = spring;
	} else {
		anim = [CABasicAnimation animationWithKeyPath:key];
		anim.duration = duration;
		anim.timingFunction = [CAMediaTimingFunction functionWithName:
			curve == JTAnimEaseIn ? kCAMediaTimingFunctionEaseIn : kCAMediaTimingFunctionEaseOut];
	}
	anim.fromValue = from;
	anim.toValue = to;
	return anim;
}

// jt_animate sets the model value and animates from whatever is on screen,
// so a new target smoothly interrupts an in-flight animation.
static void jt_animate(CALayer *layer, NSString *key, id to, JTAnimCurve curve, CFTimeInterval duration) {
	CALayer *pres = [layer presentationLayer];
	id from = [[(pres != nil ? pres : layer) valueForKey:key] retain];
	[CATransaction begin];
	[CATransaction setDisableActions:YES];
	[layer setValue:to forKey:key];
	[CATransaction commit];
	if (curve == JTAnimNone || from == nil) {
		[layer removeAnimationForKey:key];
	} else {
		[layer addAnimation:jt_make_animation(key, from, to, curve, duration) forKey:key];
	}
	[from release];
}

static void jt_animate_path(CAShapeLayer *layer, CGPathRef to, JTAnimCurve curve, CFTimeInterval duration) {
	CAShapeLayer *pres = (CAShapeLayer *)[layer presentationLayer];
	CGPathRef from = CGPathRetain(pres != nil ? [pres path] : [layer path]);
	[CATransaction begin];
	[CATransaction setDisableActions:YES];
	[layer setPath:to];
	[CATransaction commit];
	if (curve == JTAnimNone || from == NULL) {
		[layer removeAnimationForKey:@"path"];
	} else {
		[layer addAnimation:jt_make_animation(@"path", (id)from, (id)to, curve, duration) forKey:@"path"];
	}
	CGPathRelease(from);
}

static void jt_set_background(CALayer *layer, CGFloat r, CGFloat g, CGFloat b, CGFloat a) {
	CGColorRef color = CGColorCreateGenericRGB(r, g, b, a);
	[layer setBackgroundColor:color];
	CGColorRelease(color);
}

static NSValue *jt_point(CGFloat x, CGFloat y) {
	return [NSValue valueWithPoint:NSMakePoint(x, y)];
}

@interface JTNotchOverlay : NSObject <CALayerDelegate> {
	CGFloat scale;
	CGFloat backingScale;
	NSRect screenFrame;
	CGFloat notchW, notchH, notchCX;

	// Metrics derived from scale and notch geometry, in panel coordinates.
	CGFloat cx, top;
	CGFloat earW, earPad, earsH, leftEarW, rightEarW;
	CGFloat textBodyW, padX, lineH;
	CGFloat barW, barGap, barMinH, barMaxH;

	JTNotchPanel *panel;
	CALayer *body;
	CAShapeLayer *maskShape;
	CALayer *leftEar;
	CALayer *dot;
	CAShapeLayer *check;
	CALayer *bars[JT_BARS];
	CATextLayer *rightLabel;
	CALayer *textLayer;
	CAGradientLayer *shimmer;
	NSFont *textFont;
	NSFont *labelFont;

	NSString *state;
	NSString *text;
	NSString *detail;
	NSString *displayText;
	JTTextStyle displayStyle;
	// Typewriter reveal: when a live partial only extends what is already on
	// screen, typeTarget holds the fitted destination while the timer reveals
	// one composed character per tick. Any rewrite cancels and swaps at once.
	NSString *typeTarget;
	NSTimer *typeTimer;
	int maxLines;
	BOOL sessionActive;
	BOOL textEnabled;
	CFTimeInterval recordStart;
	int shownSecs;

	// Local frame driver for the waveform and timer: CADisplayLink on macOS 14+,
	// otherwise a 60 Hz NSTimer. Level frames only move targetLevel.
	id frameLink;
	CFTimeInterval lastFrameTime;
	double targetLevel, smoothLevel, wavePhase;

	JTNotchPhase phase;
	BOOL visible;
	BOOL collapsing;
	NSUInteger collapseToken;
}
- (instancetype)initWithScale:(CGFloat)newScale;
- (BOOL)attachToScreen:(NSScreen *)screen;
- (void)showState:(NSString *)newState text:(NSString *)newText level:(double)level detail:(NSString *)newDetail;
- (void)setTextEnabled:(BOOL)on;
- (void)hide;
- (void)hideNow;
- (void)teardown;
@end

@implementation JTNotchOverlay

- (instancetype)initWithScale:(CGFloat)newScale {
	self = [super init];
	if (self) {
		scale = newScale <= 0 ? 1.0 : newScale;
		phase = JTNotchCollapsed;
	}
	return self;
}

- (void)dealloc {
	[self teardown];
	[super dealloc];
}

- (void)teardown {
	[self invalidateFrames];
	[self cancelTypewriter];
	collapseToken++;
	if (panel != nil) {
		[panel orderOut:nil];
		[panel close];
		[panel release];
		panel = nil;
	}
	[body release]; body = nil;
	[maskShape release]; maskShape = nil;
	[leftEar release]; leftEar = nil;
	[dot release]; dot = nil;
	[check release]; check = nil;
	for (int i = 0; i < JT_BARS; i++) {
		[bars[i] release];
		bars[i] = nil;
	}
	[rightLabel release]; rightLabel = nil;
	[textLayer setDelegate:nil];
	[textLayer release]; textLayer = nil;
	[shimmer release]; shimmer = nil;
	[textFont release]; textFont = nil;
	[labelFont release]; labelFont = nil;
	[state release]; state = nil;
	[text release]; text = nil;
	[detail release]; detail = nil;
	[displayText release]; displayText = nil;
	[typeTarget release]; typeTarget = nil;
	visible = NO;
}

#pragma mark Geometry

- (BOOL)attachToScreen:(NSScreen *)screen {
	if (screen == nil) return NO;
	if (@available(macOS 12.0, *)) {
		NSEdgeInsets insets = [screen safeAreaInsets];
		if (insets.top <= 0) return NO;
		NSRect frame = [screen frame];
		CGFloat leftW = NSWidth([screen auxiliaryTopLeftArea]);
		CGFloat rightW = NSWidth([screen auxiliaryTopRightArea]);
		CGFloat w = NSWidth(frame) - leftW - rightW;
		if (w <= 0) return NO;
		CGFloat centerX = NSMinX(frame) + leftW + w / 2.0;
		CGFloat backing = [screen backingScaleFactor];
		if (panel != nil && NSEqualRects(frame, screenFrame) && w == notchW && insets.top == notchH &&
			centerX == notchCX && backing == backingScale) {
			return YES;
		}
		screenFrame = frame;
		notchW = w;
		notchH = insets.top;
		notchCX = centerX;
		backingScale = backing;
		[self configure];
		return YES;
	}
	return NO;
}

- (void)configure {
	CGFloat s = scale;
	earW = round(64.0 * s);
	earPad = round(14.0 * s);
	earsH = fmax(notchH, round(24.0 * s));
	barW = 3.0 * s;
	barGap = 2.0 * s;
	barMinH = 3.0 * s;
	barMaxH = fmin(16.0 * s, earsH - 8.0);
	leftEarW = 11.0 * s + 7.0 * s + JT_BARS * barW + (JT_BARS - 1) * barGap;
	rightEarW = earW - earPad - 4.0 * s;
	padX = round(18.0 * s);
	textBodyW = round(fmax(notchW + 2.0 * earW, 420.0 * s));

	[textFont release];
	textFont = [[NSFont systemFontOfSize:13.0 * s] retain];
	[labelFont release];
	labelFont = [[NSFont monospacedDigitSystemFontOfSize:12.0 * s weight:NSFontWeightMedium] retain];
	NSSize probe = [@"测Ag" sizeWithAttributes:@{NSFontAttributeName: textFont}];
	lineH = ceil(probe.height);

	CGFloat flare = 6.0 * s;
	CGFloat shake = 10.0 * s;
	CGFloat panelW = ceil(textBodyW + 2.0 * flare + 2.0 * shake);
	CGFloat panelH = ceil(earsH + [self textAreaHeight:2] + 4.0);
	CGFloat x = round(notchCX - panelW / 2.0);
	CGFloat y = NSMaxY(screenFrame) - panelH;
	cx = notchCX - x;
	top = panelH;

	if (panel == nil) {
		[self createPanel];
	}
	[panel setFrame:NSMakeRect(x, y, panelW, panelH) display:NO];

	[CATransaction begin];
	[CATransaction setDisableActions:YES];
	CGRect bounds = CGRectMake(0, 0, panelW, panelH);
	[body setFrame:bounds];
	[maskShape setFrame:bounds];
	for (CALayer *layer in @[body, leftEar, dot, check, rightLabel, textLayer]) {
		[layer setContentsScale:backingScale];
	}
	for (int i = 0; i < JT_BARS; i++) [bars[i] setContentsScale:backingScale];

	[leftEar setBounds:CGRectMake(0, 0, leftEarW, earsH)];
	CGFloat iconD = 11.0 * s;
	CGFloat dotD = 8.0 * s;
	[dot setBounds:CGRectMake(0, 0, dotD, dotD)];
	[dot setCornerRadius:dotD / 2.0];
	[dot setPosition:CGPointMake(iconD / 2.0, earsH / 2.0)];
	[check setFrame:CGRectMake(0, (earsH - iconD) / 2.0, iconD, iconD)];
	CGMutablePathRef checkPath = CGPathCreateMutable();
	CGPathMoveToPoint(checkPath, NULL, iconD * 0.10, iconD * 0.52);
	CGPathAddLineToPoint(checkPath, NULL, iconD * 0.40, iconD * 0.20);
	CGPathAddLineToPoint(checkPath, NULL, iconD * 0.92, iconD * 0.84);
	[check setPath:checkPath];
	CGPathRelease(checkPath);
	[check setLineWidth:2.0 * s];
	for (int i = 0; i < JT_BARS; i++) {
		[bars[i] setBounds:CGRectMake(0, 0, barW, barMinH)];
		[bars[i] setCornerRadius:barW / 2.0];
		[bars[i] setPosition:CGPointMake(iconD + 7.0 * s + i * (barW + barGap) + barW / 2.0, earsH / 2.0)];
	}
	CGFloat labelH = ceil([@"00:00" sizeWithAttributes:@{NSFontAttributeName: labelFont}].height);
	[rightLabel setBounds:CGRectMake(0, 0, rightEarW, labelH)];
	[CATransaction commit];

	[self applyPhase:phase curve:JTAnimNone duration:0];
	[self renderRightLabel];
	[self updateText];
}

- (void)createPanel {
	panel = [[JTNotchPanel alloc] initWithContentRect:NSMakeRect(0, 0, 10, 10)
		styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel
		backing:NSBackingStoreBuffered
		defer:NO];
	[panel setOpaque:NO];
	[panel setBackgroundColor:[NSColor clearColor]];
	[panel setHasShadow:NO];
	[panel setIgnoresMouseEvents:YES];
	[panel setCanHide:NO];
	[panel setHidesOnDeactivate:NO];
	[panel setReleasedWhenClosed:NO];
	[panel setAnimationBehavior:NSWindowAnimationBehaviorNone];
	[panel setLevel:NSMainMenuWindowLevel + 2];
	[panel setCollectionBehavior:
		NSWindowCollectionBehaviorCanJoinAllSpaces |
		NSWindowCollectionBehaviorStationary |
		NSWindowCollectionBehaviorFullScreenAuxiliary |
		NSWindowCollectionBehaviorIgnoresCycle];

	NSView *view = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, 10, 10)];
	CALayer *root = [CALayer layer];
	[view setLayer:root];
	[view setWantsLayer:YES];
	[panel setContentView:view];
	[view release];

	NSDictionary *noActions = @{
		@"position": [NSNull null],
		@"bounds": [NSNull null],
		@"contents": [NSNull null],
		@"hidden": [NSNull null],
	};

	body = [[CALayer alloc] init];
	jt_set_background(body, 0, 0, 0, 1);
	[body setActions:noActions];
	maskShape = [[CAShapeLayer alloc] init];
	[body setMask:maskShape];
	[root addSublayer:body];

	leftEar = [[CALayer alloc] init];
	[leftEar setActions:noActions];
	[body addSublayer:leftEar];

	dot = [[CALayer alloc] init];
	[dot setActions:@{@"position": [NSNull null], @"bounds": [NSNull null], @"hidden": [NSNull null]}];
	[leftEar addSublayer:dot];

	check = [[CAShapeLayer alloc] init];
	[check setActions:noActions];
	[check setFillColor:NULL];
	CGColorRef green = CGColorCreateGenericRGB(0.20, 0.84, 0.35, 1.0);
	[check setStrokeColor:green];
	CGColorRelease(green);
	[check setLineCap:kCALineCapRound];
	[check setLineJoin:kCALineJoinRound];
	[check setHidden:YES];
	[leftEar addSublayer:check];

	for (int i = 0; i < JT_BARS; i++) {
		bars[i] = [[CALayer alloc] init];
		jt_set_background(bars[i], 1, 1, 1, 0.92);
		[bars[i] setActions:@{@"position": [NSNull null]}];
		[leftEar addSublayer:bars[i]];
	}

	rightLabel = [[CATextLayer alloc] init];
	[rightLabel setActions:noActions];
	[rightLabel setAlignmentMode:kCAAlignmentRight];
	[body addSublayer:rightLabel];

	textLayer = [[CALayer alloc] init];
	[textLayer setActions:noActions];
	[textLayer setDelegate:self];
	[textLayer setNeedsDisplayOnBoundsChange:YES];
	[body addSublayer:textLayer];

	if (@available(macOS 14.0, *)) {
		// Created paused with the panel: making a display link takes tens of
		// milliseconds, which would otherwise stall the first recording frame.
		CADisplayLink *link = [[panel contentView] displayLinkWithTarget:self selector:@selector(step:)];
		[link setPaused:YES];
		[link addToRunLoop:[NSRunLoop mainRunLoop] forMode:NSRunLoopCommonModes];
		frameLink = [link retain];
	}
}

- (CGFloat)textAreaHeight:(int)lines {
	if (lines <= 0) return 0;
	return 2.0 * scale + lines * lineH + 12.0 * scale;
}

- (jt_notch_shape)shapeForPhase:(JTNotchPhase)p {
	jt_notch_shape sh;
	CGFloat flare = 6.0 * scale;
	switch (p) {
	case JTNotchCollapsed:
		sh.w = notchW;
		sh.h = notchH;
		sh.flareR = 0;
		sh.bottomR = fmin(9.0, notchH / 2.0);
		break;
	case JTNotchEars:
		sh.w = notchW + 2.0 * earW;
		sh.h = earsH;
		sh.flareR = flare;
		sh.bottomR = fmin(12.0 * scale, earsH - flare);
		break;
	case JTNotchText:
	default:
		sh.w = textBodyW;
		sh.h = earsH + [self textAreaHeight:maxLines];
		sh.flareR = flare;
		sh.bottomR = 20.0 * scale;
		break;
	}
	return sh;
}

- (void)applyPhase:(JTNotchPhase)p curve:(JTAnimCurve)curve duration:(CFTimeInterval)duration {
	if (panel == nil) return;
	phase = p;
	jt_notch_shape sh = [self shapeForPhase:p];
	CGPathRef path = jt_notch_path(cx, top, sh);
	jt_animate_path(maskShape, path, curve, duration);
	CGPathRelease(path);

	CGFloat left = cx - sh.w / 2.0;
	CGFloat right = cx + sh.w / 2.0;
	CGFloat midY = top - earsH / 2.0;
	jt_animate(leftEar, @"position", jt_point(left + earPad + leftEarW / 2.0, midY), curve, duration);
	jt_animate(rightLabel, @"position", jt_point(right - earPad - rightEarW / 2.0, midY), curve, duration);

	if (p == JTNotchText) {
		CGFloat h = maxLines * lineH;
		CGRect frame = CGRectMake(cx - textBodyW / 2.0 + padX, top - earsH - 2.0 * scale - h, textBodyW - 2.0 * padX, h);
		if (!CGRectEqualToRect(frame, [textLayer frame])) {
			[CATransaction begin];
			[CATransaction setDisableActions:YES];
			[textLayer setFrame:frame];
			[shimmer setFrame:[textLayer bounds]];
			[CATransaction commit];
		}
	}
}

#pragma mark Text

- (NSDictionary *)textAttributes:(JTTextStyle)style {
	BOOL error = style == JTTextError;
	NSMutableParagraphStyle *para = [[[NSMutableParagraphStyle alloc] init] autorelease];
	[para setLineBreakMode:error ? NSLineBreakByTruncatingTail : NSLineBreakByWordWrapping];
	NSColor *color = error
		? [NSColor colorWithCalibratedRed:1.0 green:0.62 blue:0.58 alpha:1.0]
		: [NSColor colorWithCalibratedWhite:0.96 alpha:style == JTTextPlaceholder ? 0.45 : 1.0];
	return @{
		NSFontAttributeName: textFont,
		NSForegroundColorAttributeName: color,
		NSParagraphStyleAttributeName: para,
	};
}

- (int)linesForString:(NSString *)s width:(CGFloat)width attributes:(NSDictionary *)attrs {
	NSRect r = [s boundingRectWithSize:NSMakeSize(width, CGFLOAT_MAX)
		options:NSStringDrawingUsesLineFragmentOrigin
		attributes:attrs];
	return (int)floor(NSHeight(r) / lineH + 0.5);
}

// fitTail returns the newest part of s that fits in two lines, prefixed with
// an ellipsis when the head was dropped.
- (NSString *)fitTail:(NSString *)s lines:(int *)outLines {
	CGFloat width = textBodyW - 2.0 * padX;
	NSDictionary *attrs = [self textAttributes:JTTextNormal];
	int lines = [self linesForString:s width:width attributes:attrs];
	if (lines <= 2) {
		*outLines = lines < 1 ? 1 : lines;
		return s;
	}
	NSUInteger lo = 0, hi = [s length];
	while (hi - lo > 1) {
		NSUInteger mid = lo + (hi - lo) / 2;
		NSString *candidate = [@"…" stringByAppendingString:[s substringFromIndex:mid]];
		if ([self linesForString:candidate width:width attributes:attrs] <= 2) {
			hi = mid;
		} else {
			lo = mid;
		}
	}
	if (hi < [s length]) {
		NSRange r = [s rangeOfComposedCharacterSequenceAtIndex:hi];
		if (r.location < hi) hi = NSMaxRange(r);
	}
	*outLines = 2;
	return [@"…" stringByAppendingString:[s substringFromIndex:hi]];
}

#pragma mark Typewriter

- (void)cancelTypewriter {
	[typeTimer invalidate];
	[typeTimer release];
	typeTimer = nil;
	[typeTarget release];
	typeTarget = nil;
}

// stepTypewriter reveals the next chunk of typeTarget. One composed character
// per 30ms tick is the base rate; when faster partials leave a backlog the
// step grows so the display always converges on the newest target in roughly
// half a second instead of drifting further behind.
- (void)stepTypewriter:(NSTimer *)timer {
	(void)timer;
	NSUInteger total = [typeTarget length];
	NSUInteger shownLen = [displayText length];
	if (typeTarget == nil || displayText == nil || shownLen >= total ||
		![typeTarget hasPrefix:displayText]) {
		[self cancelTypewriter];
		return;
	}
	NSUInteger step = 1 + (total - shownLen) / 16;
	NSUInteger next = shownLen + step;
	if (next < total) {
		next = NSMaxRange([typeTarget rangeOfComposedCharacterSequenceAtIndex:next]);
	}
	if (next > total) next = total;
	[displayText release];
	displayText = [[typeTarget substringToIndex:next] copy];
	[textLayer setNeedsDisplay];
	if (next >= total) [self cancelTypewriter];
}

- (void)startTypewriter {
	if (typeTimer != nil) return;
	typeTimer = [[NSTimer timerWithTimeInterval:0.03 target:self
		selector:@selector(stepTypewriter:) userInfo:nil repeats:YES] retain];
	[[NSRunLoop mainRunLoop] addTimer:typeTimer forMode:NSRunLoopCommonModes];
}

// updateText recomputes the visible text and grows the text area if needed.
// It returns YES when the shape height changed.
- (BOOL)updateText {
	if (textFont == nil) return NO;
	NSString *source;
	BOOL error = [state isEqualToString:@"error"];
	if (error) {
		source = detail;
	} else {
		source = text;
	}
	// Only the newest words are ever visible; measuring a bounded tail keeps
	// each partial update cheap however long the session runs.
	const NSUInteger tailChars = 600;
	if (!error && [source length] > tailChars) {
		NSRange r = [source rangeOfComposedCharacterSequenceAtIndex:[source length] - tailChars];
		source = [@"…" stringByAppendingString:[source substringFromIndex:NSMaxRange(r)]];
	}
	source = [[source componentsSeparatedByCharactersInSet:[NSCharacterSet newlineCharacterSet]] componentsJoinedByString:@" "];
	source = [source stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];

	NSString *shown = nil;
	JTTextStyle style = error ? JTTextError : JTTextNormal;
	int lines = 0;
	if ([source length] > 0) {
		if (error) {
			shown = source;
			lines = 1;
		} else {
			shown = [self fitTail:source lines:&lines];
		}
	} else if (textEnabled && !error && ![state isEqualToString:@"done"]) {
		// The text area opens with the session, not with the first partial.
		shown = [state isEqualToString:@"connecting"] ? @"正在连接…"
			: [state isEqualToString:@"recording"] ? @"正在聆听…" : @"正在识别…";
		style = JTTextPlaceholder;
		lines = 1;
	}
	BOOL stopping = [state hasPrefix:@"stopping"];
	BOOL done = [state isEqualToString:@"done"];
	BOOL contentChanged = !(shown == displayText || [shown isEqualToString:displayText]) || style != displayStyle;
	// A partial that only extends the on-screen text is revealed by the
	// typewriter instead of swapped in whole: the target updates immediately
	// (no extra ASR latency) while ticks reveal the new tail. Anything that is
	// not a pure append — revisions, truncations, placeholder swaps, done,
	// stopping — cancels the animation and shows the newest text at once.
	BOOL typeable = contentChanged && shown != nil && displayText != nil &&
		displayStyle == JTTextNormal && style == JTTextNormal &&
		!error && !stopping && !done && [shown hasPrefix:displayText];
	if (typeable) {
		[typeTarget release];
		typeTarget = [shown copy];
		[self startTypewriter];
	} else {
		[self cancelTypewriter];
		if (contentChanged && displayText != nil && shown != nil && !error && [textLayer opacity] >= 1.0) {
			// Short cross-fade from the old text; the content itself is already
			// the newest partial. Reusing the key replaces an in-flight fade.
			CATransition *fade = [CATransition animation];
			fade.type = kCATransitionFade;
			fade.duration = 0.1;
			fade.timingFunction = [CAMediaTimingFunction functionWithName:kCAMediaTimingFunctionEaseOut];
			[textLayer addAnimation:fade forKey:@"textFade"];
		}
		[displayText release];
		displayText = [shown copy];
		displayStyle = style;
		if (contentChanged) [textLayer setNeedsDisplay];
	}

	if (lines > maxLines) {
		maxLines = lines;
		return YES;
	}
	return NO;
}

- (void)drawLayer:(CALayer *)layer inContext:(CGContextRef)ctx {
	if (layer != textLayer || displayText == nil || textFont == nil) return;
	CGRect bounds = [layer bounds];
	CGContextSaveGState(ctx);
	CGContextTranslateCTM(ctx, 0, bounds.size.height);
	CGContextScaleCTM(ctx, 1, -1);
	NSGraphicsContext *gc = [NSGraphicsContext graphicsContextWithCGContext:ctx flipped:YES];
	[NSGraphicsContext saveGraphicsState];
	[NSGraphicsContext setCurrentContext:gc];
	NSStringDrawingOptions opts = NSStringDrawingUsesLineFragmentOrigin | NSStringDrawingTruncatesLastVisibleLine;
	[displayText drawWithRect:NSMakeRect(0, 0, bounds.size.width, bounds.size.height)
		options:opts
		attributes:[self textAttributes:displayStyle]];
	[NSGraphicsContext restoreGraphicsState];
	CGContextRestoreGState(ctx);
}

#pragma mark Ears

- (void)renderRightLabel {
	if (rightLabel == nil || labelFont == nil) return;
	NSString *s;
	CGFloat alpha = 0.9;
	if ([state isEqualToString:@"done"]) {
		if ([detail isEqualToString:@"pasted"]) {
			s = @"已粘贴";
		} else if ([detail isEqualToString:@"copied"]) {
			s = @"已复制";
		} else {
			s = @"";
		}
		alpha = 1.0;
	} else {
		int secs = 0;
		if (recordStart > 0) {
			secs = (int)(CACurrentMediaTime() - recordStart);
		} else {
			alpha = 0.45;
		}
		shownSecs = secs;
		s = [NSString stringWithFormat:@"%02d:%02d", secs / 60, secs % 60];
	}
	NSAttributedString *as = [[NSAttributedString alloc] initWithString:s attributes:@{
		NSFontAttributeName: labelFont,
		NSForegroundColorAttributeName: [NSColor colorWithCalibratedWhite:1.0 alpha:alpha],
	}];
	[rightLabel setString:as];
	[as release];
}

- (void)renderBars {
	static const CGFloat weights[JT_BARS] = {0.55, 0.85, 1.0, 0.8, 0.6};
	static const double speeds[JT_BARS] = {5.3, 7.1, 4.4, 6.2, 8.0};
	// Speech mostly sits in the lower half of the dB range; lift it visually.
	double env = sqrt(fmax(0.0, smoothLevel));
	[CATransaction begin];
	[CATransaction setDisableActions:YES];
	for (int i = 0; i < JT_BARS; i++) {
		// A slow per-bar sine keeps the bars alive without frame-to-frame noise;
		// it scales with the envelope, so silence stays flat.
		double wobble = 0.8 + 0.2 * sin(wavePhase * speeds[i] + i * 1.7);
		CGFloat v = fmin(1.0, env * weights[i] * wobble);
		[bars[i] setBounds:CGRectMake(0, 0, barW, barMinH + (barMaxH - barMinH) * v)];
	}
	[CATransaction commit];
}

- (void)step:(id)sender {
	(void)sender;
	CFTimeInterval now = CACurrentMediaTime();
	double dt = fmin(0.1, fmax(0.0, now - lastFrameTime));
	lastFrameTime = now;
	// Fast attack, slower release: peaks register at once and decay smoothly
	// between the ~20 Hz level frames.
	double tau = targetLevel > smoothLevel ? 0.05 : 0.18;
	smoothLevel += (targetLevel - smoothLevel) * (1.0 - exp(-dt / tau));
	wavePhase += dt;
	[self renderBars];
	if (recordStart > 0 && (int)(now - recordStart) != shownSecs) [self renderRightLabel];
}

- (void)startFrames {
	if (panel == nil) return;
	lastFrameTime = CACurrentMediaTime();
	if (@available(macOS 14.0, *)) {
		[(CADisplayLink *)frameLink setPaused:NO];
		return;
	}
	if (frameLink != nil) return;
	NSTimer *timer = [NSTimer timerWithTimeInterval:1.0 / 60.0 target:self selector:@selector(step:) userInfo:nil repeats:YES];
	[[NSRunLoop mainRunLoop] addTimer:timer forMode:NSRunLoopCommonModes];
	frameLink = [timer retain];
}

// stopFrames freezes the waveform and timer where they are.
- (void)stopFrames {
	if (frameLink == nil) return;
	if (@available(macOS 14.0, *)) {
		[(CADisplayLink *)frameLink setPaused:YES];
		return;
	}
	[self invalidateFrames];
}

- (void)invalidateFrames {
	if (frameLink == nil) return;
	[frameLink invalidate];
	[frameLink release];
	frameLink = nil;
}

- (void)setLevel:(double)level {
	targetLevel = fmin(1.0, fmax(0.0, level));
}

- (void)resetLevel {
	targetLevel = smoothLevel = 0;
	[self renderBars];
}

- (void)setShimmer:(BOOL)on {
	if (!on) {
		if (shimmer != nil) {
			[textLayer setMask:nil];
			[shimmer release];
			shimmer = nil;
		}
		return;
	}
	if (shimmer != nil) return;
	shimmer = [[CAGradientLayer alloc] init];
	[shimmer setActions:@{@"position": [NSNull null], @"bounds": [NSNull null]}];
	CGColorRef dim = CGColorCreateGenericRGB(1, 1, 1, 0.45);
	CGColorRef bright = CGColorCreateGenericRGB(1, 1, 1, 1);
	[shimmer setColors:@[(id)dim, (id)bright, (id)dim]];
	CGColorRelease(dim);
	CGColorRelease(bright);
	[shimmer setStartPoint:CGPointMake(0, 0.5)];
	[shimmer setEndPoint:CGPointMake(1, 0.5)];
	[shimmer setLocations:@[@0.0, @0.0, @0.25]];
	[shimmer setFrame:[textLayer bounds]];
	CABasicAnimation *sweep = [CABasicAnimation animationWithKeyPath:@"locations"];
	sweep.fromValue = @[@(-0.3), @(-0.15), @0.0];
	sweep.toValue = @[@1.0, @1.15, @1.3];
	sweep.duration = 1.3;
	sweep.repeatCount = HUGE_VALF;
	[shimmer addAnimation:sweep forKey:@"sweep"];
	[textLayer setMask:shimmer];
}

- (void)setDotRed:(CGFloat)r green:(CGFloat)g blue:(CGFloat)b {
	jt_set_background(dot, r, g, b, 1.0);
}

- (void)applyStateVisuals {
	BOOL connecting = [state isEqualToString:@"connecting"];
	BOOL recording = [state isEqualToString:@"recording"];
	BOOL stopping = [state hasPrefix:@"stopping"];
	BOOL done = [state isEqualToString:@"done"];
	BOOL error = [state isEqualToString:@"error"];

	[dot removeAnimationForKey:@"pulse"];
	[dot setHidden:done];
	[check setHidden:!done];
	if (connecting) {
		[self setDotRed:1.0 green:0.78 blue:0.25];
		CABasicAnimation *pulse = [CABasicAnimation animationWithKeyPath:@"opacity"];
		pulse.fromValue = @1.0;
		pulse.toValue = @0.3;
		pulse.duration = 0.7;
		pulse.autoreverses = YES;
		pulse.repeatCount = HUGE_VALF;
		pulse.timingFunction = [CAMediaTimingFunction functionWithName:kCAMediaTimingFunctionEaseInEaseOut];
		[dot addAnimation:pulse forKey:@"pulse"];
	} else if (recording || error) {
		[self setDotRed:1.0 green:0.27 blue:0.23];
	} else if (stopping) {
		[self setDotRed:1.0 green:0.58 blue:0.0];
	}
	if (done) {
		CABasicAnimation *draw = [CABasicAnimation animationWithKeyPath:@"strokeEnd"];
		draw.fromValue = @0.0;
		draw.toValue = @1.0;
		draw.duration = 0.25;
		draw.timingFunction = [CAMediaTimingFunction functionWithName:kCAMediaTimingFunctionEaseOut];
		[check addAnimation:draw forKey:@"draw"];
	}

	for (int i = 0; i < JT_BARS; i++) [bars[i] setOpacity:recording ? 1.0 : 0.45];
	if (connecting) [self resetLevel];

	if (recording) {
		if (recordStart <= 0) recordStart = CACurrentMediaTime();
		[self startFrames];
	} else {
		[self stopFrames];
	}
	[self renderRightLabel];
	[self setShimmer:stopping];

	if (error) {
		CAKeyframeAnimation *shake = [CAKeyframeAnimation animationWithKeyPath:@"position.x"];
		CGFloat a = 8.0 * scale;
		shake.values = @[@0, @(-a), @(a * 0.85), @(-a * 0.6), @(a * 0.35), @(-a * 0.15), @0];
		shake.additive = YES;
		shake.duration = 0.4;
		[body addAnimation:shake forKey:@"shake"];
	}
}

- (void)resetSession {
	[self stopFrames];
	[self cancelTypewriter];
	recordStart = 0;
	maxLines = 0;
	[text release];
	text = nil;
	[displayText release];
	displayText = nil;
	[self setShimmer:NO];
	[self resetLevel];
	[textLayer setNeedsDisplay];
}

#pragma mark Commands

// setTextEnabled tells whether live text will be sent, so the text area can
// open with the session instead of waiting for the first partial.
- (void)setTextEnabled:(BOOL)on {
	textEnabled = on;
}

- (void)expandFromCurrent {
	if (!visible) {
		[self applyPhase:JTNotchCollapsed curve:JTAnimNone duration:0];
		[CATransaction begin];
		[CATransaction setDisableActions:YES];
		[leftEar setOpacity:0];
		[rightLabel setOpacity:0];
		[textLayer setOpacity:0];
		[body removeAnimationForKey:@"shake"];
		[CATransaction commit];
		[panel orderFrontRegardless];
		visible = YES;
	}
	if (collapsing) {
		collapsing = NO;
		collapseToken++;
	}
	jt_animate(leftEar, @"opacity", @1.0, JTAnimEaseOut, 0.2);
	jt_animate(rightLabel, @"opacity", @1.0, JTAnimEaseOut, 0.2);
}

- (void)showState:(NSString *)newState text:(NSString *)newText level:(double)level detail:(NSString *)newDetail {
	if ([newState isEqualToString:@"idle"]) {
		[self hide];
		return;
	}
	BOOL stateChanged = ![newState isEqualToString:state];
	BOOL done = [newState isEqualToString:@"done"];
	if (!stateChanged && done && (collapsing || !visible)) {
		// A repeated done frame must not bring back a finished session.
		return;
	}
	BOOL live = [newState isEqualToString:@"connecting"] || [newState isEqualToString:@"recording"];
	if (live && !sessionActive) {
		[self resetSession];
		sessionActive = YES;
	}
	if (done || [newState isEqualToString:@"error"]) {
		sessionActive = NO;
	}
	BOOL textChanged = ![newText isEqualToString:text] || ![newDetail isEqualToString:detail];
	// Text that arrives proves it is enabled even without a config command.
	if ([newText length] > 0) textEnabled = YES;

	BOOL needsExpand = !visible || collapsing;
	if (stateChanged) collapseToken++;
	if (needsExpand) [self expandFromCurrent];

	[state release];
	state = [newState copy];
	[text release];
	text = [newText copy];
	[detail release];
	detail = [newDetail copy];

	if (stateChanged) [self applyStateVisuals];
	BOOL grew = NO;
	if (stateChanged || textChanged) grew = [self updateText];

	JTNotchPhase target = maxLines > 0 ? JTNotchText : JTNotchEars;
	if (needsExpand || grew || target != phase) {
		[self applyPhase:target curve:JTAnimSpring duration:0];
	}
	if (target == JTNotchText && [textLayer opacity] < 1.0) {
		jt_animate(textLayer, @"opacity", @1.0, JTAnimEaseOut, 0.15);
	}

	if ([state isEqualToString:@"recording"]) [self setLevel:level];

	if (done && stateChanged) {
		NSUInteger token = collapseToken;
		dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.6 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
			if (token == collapseToken) [self collapse];
		});
	}
}

- (void)collapse {
	if (!visible || collapsing) return;
	collapsing = YES;
	NSUInteger token = ++collapseToken;
	[self stopFrames];
	[self cancelTypewriter];
	[dot removeAnimationForKey:@"pulse"];
	[self setShimmer:NO];

	CFTimeInterval first = 0;
	if (phase == JTNotchText) {
		first = 0.18;
		jt_animate(textLayer, @"opacity", @0.0, JTAnimEaseIn, 0.12);
		[self applyPhase:JTNotchEars curve:JTAnimEaseIn duration:first];
	}
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(first * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
		if (token != collapseToken) return;
		jt_animate(leftEar, @"opacity", @0.0, JTAnimEaseIn, 0.12);
		jt_animate(rightLabel, @"opacity", @0.0, JTAnimEaseIn, 0.12);
		[self applyPhase:JTNotchCollapsed curve:JTAnimEaseIn duration:0.2];
		dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.22 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
			if (token != collapseToken) return;
			[panel orderOut:nil];
			visible = NO;
			collapsing = NO;
		});
	});
}

- (void)hide {
	sessionActive = NO;
	[state release];
	state = nil;
	[self stopFrames];
	[self collapse];
}

- (void)hideNow {
	collapseToken++;
	collapsing = NO;
	sessionActive = NO;
	[self cancelTypewriter];
	[state release];
	state = nil;
	[self stopFrames];
	if (panel != nil) [panel orderOut:nil];
	visible = NO;
	phase = JTNotchCollapsed;
}

@end

// ---------------------------------------------------------------------------

typedef struct {
	NSPanel *panel;
	JTOverlayView *view;
	JTNotchOverlay *notch;
	char position[32];
	CGFloat scale;
	// Notch mode pins one display per session; 0 means pick on the next show.
	CGDirectDisplayID sessionDisplay;
	BOOL lastConnecting;
} jt_overlay_t;

static jt_overlay_t *helper_overlay = NULL;

static void jt_overlay_on_main_sync(void (^block)(void)) {
	if (pthread_main_np()) {
		block();
		return;
	}
	dispatch_sync(dispatch_get_main_queue(), block);
}

static void jt_overlay_move(jt_overlay_t *overlay, NSScreen *screen, const char *position) {
	if (screen == nil) return;
	NSRect frame = [screen visibleFrame];
	CGFloat w = 122.0 * overlay->scale;
	CGFloat h = 42.0 * overlay->scale;
	CGFloat margin = 28.0 * overlay->scale;
	CGFloat x = NSMaxX(frame) - w - margin;
	CGFloat y = NSMaxY(frame) - h - margin;

	if (strcmp(position, "top-left") == 0) {
		x = NSMinX(frame) + margin; y = NSMaxY(frame) - h - margin;
	} else if (strcmp(position, "top-center") == 0) {
		x = NSMinX(frame) + (NSWidth(frame) - w) / 2.0; y = NSMaxY(frame) - h - margin;
	} else if (strcmp(position, "bottom-left") == 0) {
		x = NSMinX(frame) + margin; y = NSMinY(frame) + margin;
	} else if (strcmp(position, "bottom-center") == 0) {
		x = NSMinX(frame) + (NSWidth(frame) - w) / 2.0; y = NSMinY(frame) + margin;
	} else if (strcmp(position, "bottom-right") == 0) {
		x = NSMaxX(frame) - w - margin; y = NSMinY(frame) + margin;
	}
	[overlay->panel setFrame:NSMakeRect(x, y, w, h) display:YES];
}

static jt_overlay_t *jt_overlay_create(const char *position, double scale) {
	jt_overlay_t *overlay = calloc(1, sizeof(jt_overlay_t));
	if (overlay == NULL) return NULL;
	overlay->scale = scale <= 0 ? 1.0 : scale;
	snprintf(overlay->position, sizeof(overlay->position), "%s", position == NULL || position[0] == '\0' ? "bottom-center" : position);

	CGFloat w = 122.0 * overlay->scale;
	CGFloat h = 42.0 * overlay->scale;
	overlay->panel = [[NSPanel alloc] initWithContentRect:NSMakeRect(0, 0, w, h)
		styleMask:NSWindowStyleMaskBorderless
		backing:NSBackingStoreBuffered
		defer:NO];
	[overlay->panel setOpaque:NO];
	[overlay->panel setBackgroundColor:[NSColor clearColor]];
	[overlay->panel setHasShadow:YES];
	[overlay->panel setIgnoresMouseEvents:YES];
	[overlay->panel setCanHide:NO];
	[overlay->panel setHidesOnDeactivate:NO];
	[overlay->panel setReleasedWhenClosed:NO];
	[overlay->panel setLevel:NSFloatingWindowLevel];
	[overlay->panel setAlphaValue:1.0];
	[overlay->panel setCollectionBehavior:
		NSWindowCollectionBehaviorCanJoinAllSpaces |
		NSWindowCollectionBehaviorStationary |
		NSWindowCollectionBehaviorFullScreenAuxiliary];

	overlay->view = [[JTOverlayView alloc] initWithFrame:NSMakeRect(0, 0, w, h)];
	[overlay->view setScaleValue:overlay->scale];
	[overlay->panel setContentView:overlay->view];
	jt_overlay_move(overlay, [NSScreen mainScreen], overlay->position);

	if (strcmp(overlay->position, "notch") == 0) {
		overlay->notch = [[JTNotchOverlay alloc] initWithScale:overlay->scale];
		// Build the panel and layers now so the first hotkey press does not
		// pay for it; show re-attaches if the session picks another screen.
		[overlay->notch attachToScreen:[NSScreen mainScreen]];
	}
	return overlay;
}

static CGDirectDisplayID jt_screen_display(NSScreen *screen) {
	return (CGDirectDisplayID)[[[screen deviceDescription] objectForKey:@"NSScreenNumber"] unsignedIntValue];
}

// jt_session_screen returns the display pinned for the current notch session.
// The first show of a session pins the screen under the mouse pointer: a
// separate helper process cannot see which screen another app's focused
// window is on, and NSEvent mouseLocation needs no extra permission. A pinned
// display that was disconnected is re-picked the same way.
static NSScreen *jt_session_screen(jt_overlay_t *overlay) {
	NSArray<NSScreen *> *screens = [NSScreen screens];
	if (overlay->sessionDisplay != 0) {
		for (NSScreen *screen in screens) {
			if (jt_screen_display(screen) == overlay->sessionDisplay) return screen;
		}
	}
	NSPoint mouse = [NSEvent mouseLocation];
	NSScreen *picked = nil;
	for (NSScreen *screen in screens) {
		if (NSMouseInRect(mouse, [screen frame], NO)) {
			picked = screen;
			break;
		}
	}
	if (picked == nil) picked = [NSScreen mainScreen];
	if (picked == nil && [screens count] > 0) picked = [screens objectAtIndex:0];
	overlay->sessionDisplay = picked == nil ? 0 : jt_screen_display(picked);
	return picked;
}

static void jt_overlay_show_capsule(jt_overlay_t *overlay, NSScreen *screen, const char *position, NSString *label, unsigned short r, unsigned short g, unsigned short b) {
	[overlay->view setLabel:label red:((CGFloat)r / 65535.0) green:((CGFloat)g / 65535.0) blue:((CGFloat)b / 65535.0)];
	jt_overlay_move(overlay, screen, position);
	[overlay->panel setIsVisible:YES];
	[overlay->panel setAlphaValue:1.0];
	[overlay->panel orderFrontRegardless];
	[overlay->view display];
}

static void jt_overlay_show(jt_overlay_t *overlay, NSString *state, NSString *label, unsigned short r, unsigned short g, unsigned short b, NSString *text, double level, NSString *detail) {
	if (overlay->notch == nil) {
		jt_overlay_show_capsule(overlay, [NSScreen mainScreen], overlay->position, label, r, g, b);
		return;
	}
	BOOL connecting = [state isEqualToString:@"connecting"];
	if ([state isEqualToString:@"idle"] || (connecting && !overlay->lastConnecting)) {
		overlay->sessionDisplay = 0;
	}
	overlay->lastConnecting = connecting;
	NSScreen *screen = jt_session_screen(overlay);
	if ([overlay->notch attachToScreen:screen]) {
		if ([overlay->panel isVisible]) [overlay->panel orderOut:nil];
		[overlay->notch showState:state text:text level:level detail:detail];
		return;
	}
	// No camera housing on the active screen: fall back to a top-center capsule.
	[overlay->notch hideNow];
	if ([state isEqualToString:@"idle"]) {
		[overlay->panel orderOut:nil];
		return;
	}
	jt_overlay_show_capsule(overlay, screen, "top-center", label, r, g, b);
}

static void jt_overlay_hide(jt_overlay_t *overlay) {
	overlay->sessionDisplay = 0;
	overlay->lastConnecting = NO;
	[overlay->panel orderOut:nil];
	[overlay->notch hide];
}

static void jt_overlay_close(jt_overlay_t *overlay) {
	[overlay->panel orderOut:nil];
	[overlay->notch teardown];
	[overlay->notch release];
	[overlay->view release];
	[overlay->panel close];
	[overlay->panel release];
	free(overlay);
}

static NSString *jt_string(const char *s) {
	NSString *str = s == NULL ? nil : [NSString stringWithUTF8String:s];
	return str == nil ? @"" : str;
}

void jt_overlay_helper_init(const char *position, double scale) {
	jt_overlay_on_main_sync(^{
		[NSApplication sharedApplication];
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
		[NSApp finishLaunching];
		helper_overlay = jt_overlay_create(position, scale);
	});
}

void jt_overlay_helper_run_app(void) {
	[NSApp run];
}

void jt_overlay_helper_configure(int showText) {
	jt_overlay_t *overlay = helper_overlay;
	if (overlay == NULL) return;
	dispatch_async(dispatch_get_main_queue(), ^{
		[overlay->notch setTextEnabled:showText != 0];
	});
}

void jt_overlay_helper_show(const char *state, const char *label, unsigned short r, unsigned short g, unsigned short b, const char *text, double level, const char *detail) {
	jt_overlay_t *overlay = helper_overlay;
	if (overlay == NULL) return;
	char *stateCopy = strdup(state == NULL ? "" : state);
	char *labelCopy = strdup(label == NULL ? "" : label);
	char *textCopy = strdup(text == NULL ? "" : text);
	char *detailCopy = strdup(detail == NULL ? "" : detail);
	// Async keeps the stdin reader free; the main queue preserves command order.
	dispatch_async(dispatch_get_main_queue(), ^{
		@autoreleasepool {
			jt_overlay_show(overlay, jt_string(stateCopy), jt_string(labelCopy), r, g, b, jt_string(textCopy), level, jt_string(detailCopy));
		}
		free(stateCopy);
		free(labelCopy);
		free(textCopy);
		free(detailCopy);
	});
}

void jt_overlay_helper_hide(void) {
	jt_overlay_t *overlay = helper_overlay;
	if (overlay == NULL) return;
	dispatch_async(dispatch_get_main_queue(), ^{
		@autoreleasepool {
			jt_overlay_hide(overlay);
		}
	});
}

void jt_overlay_helper_close(void) {
	jt_overlay_t *overlay = helper_overlay;
	helper_overlay = NULL;
	jt_overlay_on_main_sync(^{
		@autoreleasepool {
			if (overlay != NULL) jt_overlay_close(overlay);
			[NSApp terminate:nil];
		}
	});
}
