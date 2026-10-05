//go:build !darwin

package main

// Windows and Linux pass --hidden to a program opened at login.
func launchedAtLogin() bool { return false }

func quitAskedByApp() bool { return false }

func inDock() bool { return true }

func menuBarOnly() bool { return true }

func showInDock(bool) {}

func activate() {}
