package main

import (
	"github.com/godbus/dbus/v5"
)

// trayAvailable reports whether the desktop shows tray icons: KDE, XFCE and
// most others run a StatusNotifierWatcher; plain GNOME does only with the
// AppIndicator extension. Without one, hiding the window would leave the app
// running with no way back to it.
func trayAvailable() bool {
	conn, err := dbus.SessionBus()
	if err != nil {
		return false
	}
	var has bool
	err = conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, "org.kde.StatusNotifierWatcher").Store(&has)
	return err == nil && has
}
