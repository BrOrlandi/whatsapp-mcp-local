//go:build !linux

package main

// The macOS menu bar and the Windows notification area are always there.
func trayAvailable() bool { return true }
