package wacli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// A crash of the daemon (kill -9, a panic, an out-of-memory kill) does not
// reach the wacli it started: sync runs in its own process group so a stop
// can reach everything it spawned, and so it outlives its parent. That orphan
// keeps the store lock, and the restarted daemon could never start sync again.
// On start, and whenever the lock is held, the supervisor reclaims it: wacli
// records its pid in the lock file, and a wacli whose parent is gone is ours to
// stop. A wacli someone started from a terminal still has its shell as parent
// and is left alone.

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
var isOrphanedWacli = func(pid int) bool {
	out, err := exec.Command("ps", "-o", "ppid=,command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return false
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return false
	}
	ppid, _ := strconv.Atoi(fields[0])
	return ppid == 1 && strings.Contains(filepath.Base(fields[1]), "wacli")
}

// reclaimOrphan stops an orphaned wacli holding the store, and reports
// whether it did.
func (s *Supervisor) reclaimOrphan() bool {
	pid := lockHolder(s.cli.StoreDir)
	if pid <= 0 || pid == os.Getpid() || syscall.Kill(pid, 0) != nil || !isOrphanedWacli(pid) {
		return false
	}
	s.logger.Warn("stopping an orphaned wacli left by an earlier daemon", "pid", pid)
	_ = syscall.Kill(-pid, syscall.SIGINT)
	_ = syscall.Kill(pid, syscall.SIGINT)
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		if syscall.Kill(pid, 0) != nil {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = syscall.Kill(pid, syscall.SIGKILL)
	time.Sleep(500 * time.Millisecond)
	return true
}
