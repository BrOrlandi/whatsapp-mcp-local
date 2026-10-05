package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// The lock is a byte range far past the pid, so the pid stays readable by
// other processes, the way a lock file is meant to be read on Windows.
const lockOffset = 1 << 30

func tryLock(f *os.File) error {
	ol := windows.Overlapped{Offset: lockOffset}
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &ol)
}

func unlock(f *os.File) {
	ol := windows.Overlapped{Offset: lockOffset}
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &ol)
}

func writePID(f *os.File) {
	_ = f.Truncate(0)
	_, _ = f.WriteAt([]byte(fmt.Sprintf("pid=%d\n", os.Getpid())), 0)
}
