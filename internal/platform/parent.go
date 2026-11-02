package platform

import (
	"os"
	"time"
)

// OnParentExit calls f once the process that started this one is gone. A
// program ended outright (as the panel ends Claude Desktop to restart it)
// does not always close the pipes of the programs it started, and those
// would stay behind, reading from a pipe nobody writes to.
func OnParentExit(f func()) {
	ppid := os.Getppid()
	if ppid <= 1 {
		return
	}
	go func() {
		if waitProcess(ppid) {
			f()
			return
		}
		// The parent could not be waited on: notice it is gone by the
		// parent changing, as it does when it exits.
		for os.Getppid() == ppid {
			time.Sleep(2 * time.Second)
		}
		f()
	}()
}
