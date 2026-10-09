//go:build darwin && cgo

#include "overlay_darwin.h"

#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>
#include <dispatch/dispatch.h>
#include <math.h>
#include <pthread.h>
#include <stdlib.h>
#include <string.h>

@interface JTOverlayView : NSView {
	NSString *label;
	NSString *transcript;
	NSColor *dotColor;
	CGFloat scale;
}
- (void)setScaleValue:(CGFloat)newScale;
- (void)setLabel:(NSString *)newLabel text:(NSString *)text red:(CGFloat)r green:(CGFloat)g blue:(CGFloat)b;
- (NSSize)preferredSize;
- (NSDictionary *)bodyAttributes;
@end

@implementation JTOverlayView
- (instancetype)initWithFrame:(NSRect)frame {
	self = [super initWithFrame:frame];
	if (self) {
		label = [@"IDLE" retain];
		transcript = [@"" retain];
		dotColor = [[NSColor colorWithCalibratedRed:0.57 green:0.57 blue:0.57 alpha:1.0] retain];
		scale = 1.0;
	}
	return self;
}
- (void)dealloc {
	[label release];
	[transcript release];
	[dotColor release];
	[super dealloc];
}
- (BOOL)isOpaque { return NO; }
- (void)setScaleValue:(CGFloat)newScale {
	scale = newScale <= 0 ? 1.0 : newScale;
	[self setNeedsDisplay:YES];
}
- (void)setLabel:(NSString *)newLabel text:(NSString *)text red:(CGFloat)r green:(CGFloat)g blue:(CGFloat)b {
	[label release];
	label = [newLabel retain];
	[transcript release];
	transcript = [(text == nil ? @"" : text) retain];
	[dotColor release];
	dotColor = [[NSColor colorWithCalibratedRed:r green:g blue:b alpha:1.0] retain];
	[self setNeedsDisplay:YES];
}
- (NSDictionary *)bodyAttributes {
	NSMutableParagraphStyle *paragraph = [[[NSMutableParagraphStyle alloc] init] autorelease];
	[paragraph setLineBreakMode:NSLineBreakByWordWrapping];
	[paragraph setLineSpacing:2.0 * scale];
	return @{
		NSFontAttributeName: [NSFont systemFontOfSize:13.0 * scale],
		NSForegroundColorAttributeName: [NSColor colorWithCalibratedWhite:0.96 alpha:1.0],
		NSParagraphStyleAttributeName: paragraph,
	};
}
- (NSSize)preferredSize {
	if ([transcript length] == 0) return NSMakeSize(122.0 * scale, 42.0 * scale);
	NSRect textBounds = [transcript boundingRectWithSize:NSMakeSize(380.0 * scale, CGFLOAT_MAX)
		options:NSStringDrawingUsesLineFragmentOrigin | NSStringDrawingUsesFontLeading
		attributes:[self bodyAttributes]];
	return NSMakeSize(420.0 * scale, MIN(140.0 * scale, MAX(76.0 * scale, ceil(textBounds.size.height) + 52.0 * scale)));
}
- (void)drawRect:(NSRect)dirtyRect {
	(void)dirtyRect;
	NSRect bounds = [self bounds];
	BOOL hasText = [transcript length] > 0;
	CGFloat radius = hasText ? 18.0 * scale : NSHeight(bounds) / 2.0;
	NSBezierPath *pill = [NSBezierPath bezierPathWithRoundedRect:bounds xRadius:radius yRadius:radius];
	[[NSColor colorWithCalibratedRed:0.08 green:0.08 blue:0.08 alpha:0.94] setFill];
	[pill fill];
	if (hasText) {
		CGFloat dotSize = 8.0 * scale;
		NSBezierPath *dot = [NSBezierPath bezierPathWithOvalInRect:
			NSMakeRect(20.0 * scale, NSHeight(bounds) - 25.0 * scale, dotSize, dotSize)];
		[dotColor setFill];
		[dot fill];
		NSDictionary *header = @{
			NSFontAttributeName: [NSFont boldSystemFontOfSize:11.0 * scale],
			NSForegroundColorAttributeName: [NSColor colorWithCalibratedWhite:0.65 alpha:1.0],
		};
		[label drawAtPoint:NSMakePoint(36.0 * scale, NSHeight(bounds) - 29.0 * scale) withAttributes:header];
		[transcript drawWithRect:NSMakeRect(20.0 * scale, 14.0 * scale,
			NSWidth(bounds) - 40.0 * scale, NSHeight(bounds) - 52.0 * scale)
			options:NSStringDrawingUsesLineFragmentOrigin | NSStringDrawingUsesFontLeading | NSStringDrawingTruncatesLastVisibleLine
			attributes:[self bodyAttributes]];
		return;
	}

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

typedef struct {
	NSPanel *panel;
	JTOverlayView *view;
	char position[32];
	CGFloat scale;
} jt_overlay_t;

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

		CGFloat w = 122.0 * overlay->scale;
		CGFloat h = 42.0 * overlay->scale;
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
		[overlay->panel setIsVisible:YES];
		[overlay->panel setAlphaValue:1.0];
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
		[overlay->panel orderOut:nil];
		jt_overlay_pump();
	});
}

void jt_overlay_close(void *handle) {
	jt_overlay_t *overlay = (jt_overlay_t *)handle;
	if (overlay == NULL) return;
	jt_overlay_on_main_sync(^{
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
