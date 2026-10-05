//go:build !windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

// Like wacli, the store lock is an flock, which the kernel releases however
// the holder dies.
func tryLock(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) }

func unlock(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }

func writePID(f *os.File) {
	_ = f.Truncate(0)
	_, _ = f.WriteAt([]byte(fmt.Sprintf("pid=%d\n", os.Getpid())), 0)
}
