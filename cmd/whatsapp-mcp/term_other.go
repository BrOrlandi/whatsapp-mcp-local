//go:build !darwin && !linux

package main

import "os"

// isTerminal reports whether f is a terminal, not a pipe or a file.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
