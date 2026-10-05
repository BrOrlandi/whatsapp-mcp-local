package wacli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/platform"
)

// A crash of the daemon (kill -9, a panic, an out-of-memory kill) does not
// always reach the wacli it started: on macOS sync runs in its own process
// group so a stop can reach everything it spawned, and so it outlives its
// parent. (Windows kills it with the app's job object, and Linux with a
// parent-death signal, but an older version or a stuck process can still
// leave one behind.) That orphan keeps the store lock, and the restarted
// daemon could never start sync again. On start, and whenever the lock is
// held, the supervisor reclaims it: wacli records its pid in the lock file,
// and a wacli whose parent is gone is ours to stop. A wacli someone started
// from a terminal still has its shell as parent and is left alone.

// lockHolder reads the pid wacli wrote into the store's lock file.
func lockHolder(storeDir string) int {
	raw, err := os.ReadFile(filepath.Join(storeDir, "LOCK"))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "pid="); ok {
			pid, _ := strconv.Atoi(v)
			return pid
		}
	}
	return 0
}

// isOrphanedWacli reports whether pid is a wacli whose parent has died. It is
// a variable so tests can stand in for the process table.
var isOrphanedWacli = func(pid int) bool { return platform.Orphaned(pid, "wacli") }

// reclaimOrphan stops an orphaned wacli holding the store, and reports
// whether it did.
func (s *Supervisor) reclaimOrphan() bool {
	pid := lockHolder(s.cli.StoreDir)
	if pid <= 0 || pid == platform.Self() || !platform.Alive(pid) || !isOrphanedWacli(pid) {
		return false
	}
	s.logger.Warn("stopping an orphaned wacli left by an earlier daemon", "pid", pid)
	_ = platform.Interrupt(pid)
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		if !platform.Alive(pid) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	_ = platform.Kill(pid)
	time.Sleep(500 * time.Millisecond)
	return true
}
