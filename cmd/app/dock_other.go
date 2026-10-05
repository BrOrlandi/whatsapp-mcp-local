//go:build !darwin

package main

// Windows and Linux pass --hidden to a program opened at login.
func launchedAtLogin() bool { return false }

func showInDock(bool) {}
