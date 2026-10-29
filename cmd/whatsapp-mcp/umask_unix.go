//go:build darwin || linux

package main

import "syscall"

// privateFiles makes every file the daemon creates readable by its owner
// only, the SQLite journals included, which SQLite creates with the umask.
func privateFiles() { syscall.Umask(0o077) }
