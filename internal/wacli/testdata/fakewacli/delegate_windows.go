package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// pipeName is the named pipe wacli's sync serves on Windows (since 0.23):
// the sha256 of the store's absolute path, links resolved, in lower case.
func pipeName() (*uint16, error) {
	path, err := filepath.Abs(store)
	if err != nil {
		return nil, err
	}
	if path, err = filepath.EvalSymlinks(path); err != nil {
		return nil, err
	}
	return windows.UTF16PtrFromString(fmt.Sprintf(`\\.\pipe\wacli-send-%x`, sha256.Sum256([]byte(strings.ToLower(path)))))
}

func newInstance(name *uint16) (windows.Handle, error) {
	return windows.CreateNamedPipe(name, windows.PIPE_ACCESS_DUPLEX, windows.PIPE_TYPE_BYTE|windows.PIPE_WAIT,
		windows.PIPE_UNLIMITED_INSTANCES, 4096, 4096, 0, nil)
}

// serveDelegate opens the delegate pipe and accepts every connection,
// keeping one instance waiting at all times. The process ending closes it.
func serveDelegate() (func(), error) {
	name, err := pipeName()
	if err != nil {
		return nil, err
	}
	h, err := newInstance(name)
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			if err := windows.ConnectNamedPipe(h, nil); err != nil && !errors.Is(err, windows.ERROR_PIPE_CONNECTED) {
				return
			}
			next, err := newInstance(name)
			_ = windows.DisconnectNamedPipe(h)
			_ = windows.CloseHandle(h)
			if err != nil {
				return
			}
			h = next
		}
	}()
	return func() {}, nil
}

func delegateUp() bool {
	name, err := pipeName()
	if err != nil {
		return false
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return errors.Is(err, windows.ERROR_PIPE_BUSY)
	}
	_ = windows.CloseHandle(h)
	return true
}
