package platform

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// createNoWindow keeps a console program started by the app (which has no
// console of its own) from opening a black window.
const createNoWindow = 0x08000000

var (
	kernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	procAttachConsole            = kernel32.NewProc("AttachConsole")
	procFreeConsole              = kernel32.NewProc("FreeConsole")
	procGetConsoleWindow         = kernel32.NewProc("GetConsoleWindow")
	procGenerateConsoleCtrlEvent = kernel32.NewProc("GenerateConsoleCtrlEvent")
	procSetConsoleCtrlHandler    = kernel32.NewProc("SetConsoleCtrlHandler")

	jobOnce sync.Once
	job     windows.Handle
	jobErr  error

	// consoleMu serialises borrowing a child's console: a process has at most
	// one console attached at a time.
	consoleMu sync.Mutex
)

// Prepare runs a long-lived child in a console process group of its own, so
// CTRL_BREAK reaches it and not the app, without a window.
func Prepare(cmd *exec.Cmd) {
	Background(cmd)
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP
}

// Background keeps a console program started by the app from opening a
// window. The command line has a console, which the child simply shares.
func Background(cmd *exec.Cmd) {
	if hasConsole() {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}

// Started puts the child in the app's job object, which Windows closes when
// the app exits for any reason, killing every process still in it. This is
// what keeps a crash of the app from leaving wacli holding the store.
func Started(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return errors.New("process not started")
	}
	jobOnce.Do(func() { job, jobErr = newKillOnCloseJob() })
	if jobErr != nil {
		return jobErr
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.AssignProcessToJobObject(job, h)
}

func newKillOnCloseJob() (windows.Handle, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE},
	}
	if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(h)
		return 0, err
	}
	return h, nil
}

// Interrupt sends CTRL_BREAK to the child's process group. A program with a
// console (the command line) shares it with the child and sends it directly;
// the app, which has none, attaches to the child's hidden console for the
// moment it takes.
func Interrupt(pid int) error {
	consoleMu.Lock()
	defer consoleMu.Unlock()
	if hasConsole() {
		return ctrlBreak(pid)
	}
	if r, _, err := procAttachConsole.Call(uintptr(pid)); r == 0 {
		return err
	}
	defer procFreeConsole.Call()
	// While attached, a Ctrl+C on that console must not reach the app.
	procSetConsoleCtrlHandler.Call(0, 1)
	defer procSetConsoleCtrlHandler.Call(0, 0)
	return ctrlBreak(pid)
}

func hasConsole() bool {
	r, _, _ := procGetConsoleWindow.Call()
	return r != 0
}

func ctrlBreak(pid int) error {
	if r, _, err := procGenerateConsoleCtrlEvent.Call(windows.CTRL_BREAK_EVENT, uintptr(pid)); r == 0 {
		return err
	}
	return nil
}

// Kill terminates the process at once.
func Kill(pid int) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.TerminateProcess(h, 1)
}

// Alive reports whether a process with this pid is running.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == 259 // STILL_ACTIVE
}

// Orphaned reports whether pid is a program called name whose parent is no
// longer running. The job object makes this rare: it is for a wacli left by a
// crash of an older version, or of the command line.
func Orphaned(pid int, name string) bool {
	ppid, exe, ok := processEntry(pid)
	if !ok || !strings.Contains(strings.ToLower(exe), strings.ToLower(name)) {
		return false
	}
	return !Alive(ppid)
}

func processEntry(pid int) (ppid int, exe string, ok bool) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, "", false
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if int(e.ProcessID) == pid {
			return int(e.ParentProcessID), filepath.Base(windows.UTF16ToString(e.ExeFile[:])), true
		}
	}
	return 0, "", false
}

// Self is this process's pid.
func Self() int { return os.Getpid() }
