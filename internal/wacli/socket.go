package wacli

import (
	"net"
	"path/filepath"
	"time"
)

// unixSocketReady reports whether the sync's send socket answers: a unix
// socket in the store, where wacli send hands its work to the sync.
func unixSocketReady(storeDir string) bool {
	conn, err := net.DialTimeout("unix", filepath.Join(storeDir, ".send.sock"), 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
