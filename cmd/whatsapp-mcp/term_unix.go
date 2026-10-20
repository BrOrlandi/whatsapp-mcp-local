//go:build darwin || linux

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// isTerminal reports whether f is a terminal, not a pipe, a file or
// /dev/null (which is a character device too).
func isTerminal(f *os.File) bool {
	_, err := unix.IoctlGetTermios(int(f.Fd()), ioctlGetTermios)
	return err == nil
}

// ansiOK reports whether f shows ANSI colours and cursor codes: any terminal.
func ansiOK(f *os.File) bool { return isTerminal(f) }
