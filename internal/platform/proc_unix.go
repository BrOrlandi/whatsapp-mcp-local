//go:build !windows

package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Prepare runs a long-lived child in its own process group, so an interrupt
// reaches everything it started. On Linux the child is also told to stop
// when the app dies; macOS has no such thing, which is what orphan recovery
// is for.
func Prepare(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	setParentDeathSignal(cmd.SysProcAttr)
}

// Started has nothing to do on Unix.
func Started(*exec.Cmd) error { return nil }

// Background marks a short command as one that must not open a window. Only
// Windows opens one.
func Background(*exec.Cmd) {}

// Interrupt asks the process, and its group, to stop.
func Interrupt(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGINT); err != nil {
		return syscall.Kill(pid, syscall.SIGINT)
	}
	return nil
}

// Kill stops the process, and its group, at once.
func Kill(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		return syscall.Kill(pid, syscall.SIGKILL)
	}
	return nil
}

// Alive reports whether a process with this pid exists.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// Orphaned reports whether pid is a program called name whose parent has
// died: adopted by init (or launchd), or by the session's systemd, which
// Linux desktops make the subreaper of everything a login started. A program
// someone started from a terminal still has the shell as its parent.
func Orphaned(pid int, name string) bool {
	ppid, command := psInfo(pid)
	if ppid == 0 || !strings.Contains(filepath.Base(command), name) {
		return false
	}
	if ppid == 1 {
		return true
	}
	_, parent := psInfo(ppid)
	base := filepath.Base(parent)
	return base == "systemd" || base == "init"
}

func psInfo(pid int) (ppid int, command string) {
	out, err := exec.Command("ps", "-o", "ppid=,comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, ""
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return 0, ""
	}
	ppid, _ = strconv.Atoi(fields[0])
	return ppid, strings.Join(fields[1:], " ")
}

// Self is this process's pid.
func Self() int { return os.Getpid() }

// Detach starts a command in a session of its own, so it outlives this
// process and is not stopped with its group.
func Detach(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
}
