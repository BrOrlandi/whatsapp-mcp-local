package wacli

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// sendSocketReady reports whether the sync's send pipe answers. Since wacli
// 0.23 it is a named pipe on Windows: its name is the sha256 of the store's
// absolute path, symbolic links resolved, in lower case
// (internal/delegate/transport_windows.go in wacli). Before, it was a unix
// socket in the store, as on the other systems.
func sendSocketReady(storeDir string) bool {
	return pipeReady(storeDir) || unixSocketReady(storeDir)
}

func pipeReady(storeDir string) bool {
	path, err := filepath.Abs(storeDir)
	if err != nil {
		return false
	}
	if path, err = filepath.EvalSymlinks(path); err != nil {
		return false
	}
	name := fmt.Sprintf(`\\.\pipe\wacli-send-%x`, sha256.Sum256([]byte(strings.ToLower(path))))
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		// Every instance taken means the sync is there, serving someone.
		return errors.Is(err, windows.ERROR_PIPE_BUSY)
	}
	_ = windows.CloseHandle(h)
	return true
}
