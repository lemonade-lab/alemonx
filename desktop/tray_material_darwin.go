//go:build darwin && !ios

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>

static void installTrayMaterial(void *handle) {
	NSWindow *window = (NSWindow *)handle;
	NSView *content = window.contentView;
	if (content == nil) return;
	window.appearance = nil;

	NSVisualEffectView *material = [[NSVisualEffectView alloc] initWithFrame:content.bounds];
	if (@available(macOS 10.14, *)) {
		material.material = NSVisualEffectMaterialPopover;
	}
	material.blendingMode = NSVisualEffectBlendingModeBehindWindow;
	material.state = NSVisualEffectStateActive;
	material.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;

	[content addSubview:material positioned:NSWindowBelow relativeTo:nil];
	[material release];
	[window invalidateShadow];
}
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

func prepareTrayMaterial(window *application.WebviewWindow) bool {
	handle := window.NativeWindow()
	if handle == nil {
		return false
	}
	C.installTrayMaterial(handle)
	return true
}
