//go:build darwin && !ios

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
static void skillhubAppearance(void *handle, int mode) {
 NSWindow *window = (NSWindow *)handle;
 window.appearance = mode == 0 ? nil : [NSAppearance appearanceNamed:mode == 2 ? NSAppearanceNameDarkAqua : NSAppearanceNameAqua];
 BOOL dark = [[window.effectiveAppearance bestMatchFromAppearancesWithNames:@[NSAppearanceNameAqua, NSAppearanceNameDarkAqua]] isEqualToString:NSAppearanceNameDarkAqua];
 window.titlebarAppearsTransparent = YES;
 window.backgroundColor = dark ? [NSColor colorWithRed:16.0/255 green:23.0/255 blue:34.0/255 alpha:1] : [NSColor whiteColor];
}
*/
import "C"
import (
	"errors"
	"unsafe"
)

func applyWindowAppearance(handle unsafe.Pointer, mode string, _ bool) error {
	if handle == nil {
		return errors.New("native window is unavailable")
	}
	value := 0
	if mode == "light" {
		value = 1
	} else if mode == "dark" {
		value = 2
	}
	C.skillhubAppearance(handle, C.int(value))
	return nil
}
