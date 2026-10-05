// Command fakewacli behaves like wacli where the supervisor depends on it, on
// every system the app runs on: sync holds the store lock and opens the
// delegate socket, exclusive commands fail while the lock is held, delegated
// commands succeed while the socket is up, and auth walks through a pairing.
//
// Files in the store steer it: AUTHED marks the store as paired, HANG makes
// auth keep going after the link like a large account's history does, and
// calls.log records which kind of command ran.
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

var store = os.Getenv("WACLI_STORE_DIR")

func main() {
	args := os.Args[1:]
	for len(args) > 0 && (args[0] == "--json" || args[0] == "--read-only") {
		args = args[1:]
	}
	if len(args) == 1 && args[0] == "doctor" {
		reply(map[string]any{"authenticated": exists("AUTHED"), "session_revoked": false})
		return
	}
	if len(args) < 2 {
		fail("unknown")
	}
	switch args[0] + " " + args[1] {
	case "auth status":
		reply(map[string]any{"authenticated": exists("AUTHED"), "phone": "5511912345678"})
	case "auth --events":
		auth()
	case "sync --follow":
		syncFollow()
	case "chats archive":
		if locked() {
			fail("store is locked by another process")
		}
		record("exclusive")
		reply(map[string]any{"archived": true})
	case "send text":
		if locked() && !exists(".send.sock") {
			fail("store is locked")
		}
		record("delegated")
		reply(map[string]any{"sent": true, "id": "ABC"})
	default:
		fail("unknown")
	}
}

func reply(data any) {
	body, _ := json.Marshal(map[string]any{"success": true, "data": data, "error": nil})
	fmt.Println(string(body))
}

func fail(msg string) {
	body, _ := json.Marshal(map[string]any{"success": false, "data": nil, "error": msg})
	fmt.Println(string(body))
	os.Exit(1)
}

func event(name string, data map[string]any) {
	body, _ := json.Marshal(map[string]any{"event": name, "data": data, "ts": 1})
	fmt.Fprintln(os.Stderr, string(body))
}

func exists(name string) bool {
	_, err := os.Stat(filepath.Join(store, name))
	return err == nil
}

func record(kind string) {
	f, err := os.OpenFile(filepath.Join(store, "calls.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, kind)
}

// locked reports whether another process holds the store lock, the way an
// exclusive wacli command finds out.
func locked() bool {
	f, err := os.OpenFile(filepath.Join(store, "LOCK"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return true
	}
	defer f.Close()
	if err := tryLock(f); err != nil {
		return true
	}
	unlock(f)
	return false
}

func interrupted() <-chan os.Signal {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	return c
}

func auth() {
	if locked() {
		fmt.Fprintln(os.Stderr, "store is locked")
		os.Exit(1)
	}
	stop := interrupted()
	pause := func(d time.Duration) {
		select {
		case <-stop:
			os.Exit(130)
		case <-time.After(d):
		}
	}
	event("auth_starting", nil)
	event("qr_code", map[string]any{"code": "2@first"})
	pause(300 * time.Millisecond)
	event("qr_code", map[string]any{"code": "2@second"})
	pause(300 * time.Millisecond)
	_ = os.WriteFile(filepath.Join(store, "AUTHED"), nil, 0o600)
	event("connected", nil)
	event("history_sync", map[string]any{"conversations": 12})
	event("progress", map[string]any{"messages_synced": 340})
	if exists("HANG") {
		// A large account: the history keeps coming long after the link.
		<-stop
		os.Exit(130)
	}
	pause(300 * time.Millisecond)
	event("idle_exit", map[string]any{"messages_synced": 512})
}

func syncFollow() {
	stop := interrupted()
	lock, err := os.OpenFile(filepath.Join(store, "LOCK"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := tryLock(lock); err != nil {
		fmt.Fprintln(os.Stderr, "store is locked")
		os.Exit(1)
	}
	writePID(lock)
	sock := filepath.Join(store, ".send.sock")
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
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
	event("connected", nil)
	<-stop
	ln.Close()
	_ = os.Remove(sock)
	unlock(lock)
	lock.Close()
}
