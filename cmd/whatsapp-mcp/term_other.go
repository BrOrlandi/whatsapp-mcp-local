//go:build !darwin && !linux && !windows

package main

import "os"

// isTerminal reports whether f is a terminal, not a pipe or a file.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// ansiOK reports whether f shows ANSI colours and cursor codes.
func ansiOK(f *os.File) bool { return isTerminal(f) }
