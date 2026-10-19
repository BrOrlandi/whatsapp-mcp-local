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
