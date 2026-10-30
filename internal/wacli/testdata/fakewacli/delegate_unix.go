//go:build !windows

package main

import (
	"net"
	"os"
	"path/filepath"
)

// serveDelegate opens the delegate socket, a unix socket in the store, as
// wacli's sync does, and accepts every connection until the function it
// returns closes it.
func serveDelegate() (func(), error) {
	sock := filepath.Join(store, ".send.sock")
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	return func() { ln.Close(); _ = os.Remove(sock) }, nil
}

func delegateUp() bool { return exists(".send.sock") }
