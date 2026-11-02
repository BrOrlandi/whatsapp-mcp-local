package panel

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const desktopControl = true

// desktopProcesses are Claude Desktop's processes, told from Claude Code's
// claude.exe by where they run from: the Store package (WindowsApps\Claude_…)
// or the older installer (AnthropicClaude).
func desktopProcesses() map[uint32]string {
	out := map[uint32]string{}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return out
	}
	defer windows.CloseHandle(snap)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if !strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "claude.exe") {
			continue
		}
		if path := processPath(entry.ProcessID); isDesktopExe(path) {
			out[entry.ProcessID] = path
		}
	}
	return out
}

func isDesktopExe(path string) bool {
	p := strings.ToLower(path)
	return strings.Contains(p, `\windowsapps\claude_`) || strings.Contains(p, `\anthropicclaude\`)
}

func processPath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

func desktopRunning() bool { return len(desktopProcesses()) > 0 }

// quitDesktop ends Claude Desktop. Asking its window to close only hides it
// by the clock, so it is ended outright, which also keeps it from writing
// its configuration once more on the way out.
func quitDesktop(ctx context.Context) error {
	for pid := range desktopProcesses() {
		if h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid); err == nil {
			_ = windows.TerminateProcess(h, 0)
			windows.CloseHandle(h)
		}
	}
	return waitDesktopClosed(ctx, 10*time.Second)
}

// desktopLaunchTarget is how to open Claude Desktop again: the Store app's
// id, or the program of the older installer.
func desktopLaunchTarget() string {
	for _, path := range desktopProcesses() {
		if id := storeAppID(path); id != "" {
			return id
		}
		return path
	}
	// Not running: the package's folders name its publisher.
	local := os.Getenv("LOCALAPPDATA")
	for _, pattern := range []string{filepath.Join(local, "Claude-Data", "claude_*"), filepath.Join(local, "Packages", "Claude_*")} {
		if dirs, _ := filepath.Glob(pattern); len(dirs) > 0 {
			name := filepath.Base(dirs[0])
			return "Claude_" + name[strings.Index(name, "_")+1:] + "!Claude"
		}
	}
	if exe := filepath.Join(local, "AnthropicClaude", "claude.exe"); func() bool { _, err := os.Stat(exe); return err == nil }() {
		return exe
	}
	return ""
}

// storeAppID turns …\WindowsApps\Claude_4.4212.0.0_arm64__pzs8sxrjxfjjc\app\Claude.exe
// into Claude_pzs8sxrjxfjjc!Claude.
func storeAppID(path string) string {
	for _, part := range strings.Split(path, `\`) {
		if strings.HasPrefix(part, "Claude_") {
			if i := strings.LastIndex(part, "__"); i > 0 {
				return "Claude_" + part[i+2:] + "!Claude"
			}
		}
	}
	return ""
}

func openDesktop(target string) error {
	if target == "" {
		return errDesktopStillOpen
	}
	var cmd *exec.Cmd
	if strings.HasSuffix(target, "!Claude") {
		cmd = exec.Command("explorer.exe", `shell:AppsFolder\`+target)
	} else {
		cmd = exec.Command(target)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
