package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// isTerminal reports whether f is a console, not a pipe, a file or NUL
// (which is a character device too).
func isTerminal(f *os.File) bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(f.Fd()), &mode) == nil
}

// ansiOK reports whether f is a console that shows ANSI colours and cursor
// codes, turning them on when the console allows: an old console prints the
// codes as text, and gets the plain drawing instead.
func ansiOK(f *os.File) bool {
	h := windows.Handle(f.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
