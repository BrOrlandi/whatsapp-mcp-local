package main

/*
#cgo CFLAGS: -x objective-c -mmacosx-version-min=11.0
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static int wamcpLaunchedAtLogin = 0;

// The system opens a login item with an "open application" Apple event that
// says so. The event is only there while the app finishes launching, so it is
// read then, as the notification is posted, and kept.
__attribute__((constructor)) static void wamcpWatchLaunch(void) {
	[[NSNotificationCenter defaultCenter] addObserverForName:NSApplicationDidFinishLaunchingNotification
		object:nil queue:nil usingBlock:^(NSNotification *note) {
			NSAppleEventDescriptor *event = [[NSAppleEventManager sharedAppleEventManager] currentAppleEvent];
			if (event != nil && [event eventID] == kAEOpenApplication &&
				[[event paramDescriptorForKeyword:keyAEPropData] enumCodeValue] == keyAELaunchedAsLogInItem) {
				wamcpLaunchedAtLogin = 1;
			}
		}];
}

static int wamcpWasLaunchedAtLogin(void) { return wamcpLaunchedAtLogin; }

static void wamcpShowInDock(int show) {
	if (show) {
		[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
		[NSApp activateIgnoringOtherApps:YES];
	} else {
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
	}
}
*/
import "C"

// launchedAtLogin reports whether macOS opened the app as a login item. The
// login item cannot carry --hidden, so this is how the app knows to stay in
// the menu bar.
func launchedAtLogin() bool { return C.wamcpWasLaunchedAtLogin() == 1 }

// showInDock puts the app in the Dock and the app switcher while its window
// is open, and takes it out when only the menu bar icon is left. It must run
// on the main thread.
func showInDock(show bool) {
	v := 0
	if show {
		v = 1
	}
	C.wamcpShowInDock(C.int(v))
}
