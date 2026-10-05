package main

/*
#cgo CFLAGS: -x objective-c -mmacosx-version-min=11.0
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static int wamcpLaunchedAtLogin = 0;
static int wamcpPoweringOff = 0;

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
	// Logging out, restarting and shutting down are announced before the
	// apps are asked to quit.
	[[[NSWorkspace sharedWorkspace] notificationCenter] addObserverForName:NSWorkspaceWillPowerOffNotification
		object:nil queue:nil usingBlock:^(NSNotification *note) {
			wamcpPoweringOff = 1;
		}];
}

static int wamcpWasLaunchedAtLogin(void) { return wamcpLaunchedAtLogin; }

// The Dock and other apps ask an app to quit with a quit Apple event. The
// one a log out, a restart or a shut down sends carries the reason.
static int wamcpQuitAskedByApp(void) {
	if (wamcpPoweringOff) {
		return 0;
	}
	NSAppleEventDescriptor *event = [[NSAppleEventManager sharedAppleEventManager] currentAppleEvent];
	if (event == nil || [event eventClass] != kCoreEventClass || [event eventID] != kAEQuitApplication) {
		return 0;
	}
	return [event attributeDescriptorForKeyword:kAEQuitReason] == nil &&
		[event paramDescriptorForKeyword:kAEQuitReason] == nil;
}

static int wamcpActivationPolicy(void) {
	return (int)[NSApp activationPolicy];
}

static void wamcpShowInDock(int show) {
	[NSApp setActivationPolicy:show ? NSApplicationActivationPolicyRegular : NSApplicationActivationPolicyAccessory];
}

static void wamcpActivate(void) {
	[NSApp activateIgnoringOtherApps:YES];
}
*/
import "C"

// launchedAtLogin reports whether macOS opened the app as a login item. The
// login item cannot carry --hidden, so this is how the app knows to stay in
// the menu bar.
func launchedAtLogin() bool { return C.wamcpWasLaunchedAtLogin() == 1 }

// quitAskedByApp reports whether the request to quit came from the Dock or
// another app, rather than from logging out, shutting down or a signal,
// which the app must never refuse. It must run on the main thread, while the
// request is handled.
func quitAskedByApp() bool { return C.wamcpQuitAskedByApp() == 1 }

// inDock reports whether the app is in the Dock and the app switcher. It
// must run on the main thread.
func inDock() bool { return C.wamcpActivationPolicy() == C.NSApplicationActivationPolicyRegular }

// menuBarOnly reports whether the app is only in the menu bar, as Wails
// leaves it when it starts. It must run on the main thread.
func menuBarOnly() bool { return C.wamcpActivationPolicy() == C.NSApplicationActivationPolicyAccessory }

// showInDock puts the app in the Dock and the app switcher while its window
// is open, and takes it out when only the menu bar icon is left. The change
// completes some time after the call returns, and macOS says nothing when it
// has. It must run on the main thread.
func showInDock(show bool) {
	v := 0
	if show {
		v = 1
	}
	C.wamcpShowInDock(C.int(v))
}

// activate brings the app to the front. It must run on the main thread.
func activate() { C.wamcpActivate() }
