package wacli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeWacli behaves like wacli where the supervisor depends on it: sync holds
// a lock and opens the delegate socket, exclusive commands fail while the lock
// is held, and delegated commands succeed while the socket is up.
const fakeWacli = `#!/bin/bash
STORE="$WACLI_STORE_DIR"
[ "$1" = "--json" ] && shift
case "$1 $2" in
"auth status")
  echo '{"success":true,"data":{"authenticated":true},"error":null}' ;;
"sync --follow")
  mkdir "$STORE/LOCK" 2>/dev/null || { echo "store is locked" >&2; exit 1; }
  exec python3 -c '
import socket, os, sys, signal
p = sys.argv[1]
s = socket.socket(socket.AF_UNIX); s.bind(p); s.listen(8)
def stop(*_):
    os.unlink(p); os.rmdir(os.path.join(os.path.dirname(p), "LOCK")); sys.exit(0)
signal.signal(signal.SIGINT, stop); signal.signal(signal.SIGTERM, stop)
sys.stderr.write("{\"event\":\"connected\",\"ts\":1}\n"); sys.stderr.flush()
while True:
    c, _ = s.accept(); c.close()
' "$STORE/.send.sock" ;;
"chats archive")
  if [ -d "$STORE/LOCK" ]; then echo '{"success":false,"data":null,"error":"store is locked by another process"}'; exit 1; fi
  echo "exclusive" >> "$STORE/calls.log"
  echo '{"success":true,"data":{"archived":true},"error":null}' ;;
"send text")
  if [ -d "$STORE/LOCK" ] && [ ! -S "$STORE/.send.sock" ]; then echo '{"success":false,"data":null,"error":"store is locked"}'; exit 1; fi
  echo "delegated" >> "$STORE/calls.log"
  echo '{"success":true,"data":{"sent":true,"id":"ABC"},"error":null}' ;;
*)
  echo '{"success":false,"data":null,"error":"unknown"}'; exit 1 ;;
esac
`

func waitState(t *testing.T, s *Supervisor, want string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if st := s.Status(); st.State == want && (want != "connected" || st.Delegate) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("supervisor never reached %q; status %+v", want, s.Status())
}

func TestSupervisorPausesSyncForExclusiveOperations(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is needed for the fake wacli")
	}
	// Unix socket paths are limited to ~104 bytes, too short for t.TempDir on macOS.
	store, err := os.MkdirTemp("/tmp", "wacli-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(store)
	bin := filepath.Join(store, "wacli")
	if err := os.WriteFile(bin, []byte(fakeWacli), 0o755); err != nil {
		t.Fatal(err)
	}

	cli := &CLI{Bin: bin, StoreDir: store}
	s := NewSupervisor(cli, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	waitState(t, s, "connected")

	// While sync holds the lock, a delegated send goes through its socket.
	if err := s.Delegated(ctx, func(ctx context.Context) error {
		_, err := cli.Run(ctx, "send", "text", "--to", "x", "--message", "hi")
		return err
	}); err != nil {
		t.Fatalf("delegated send failed: %v", err)
	}

	// An exclusive command would fail on the lock; the supervisor stops sync first.
	var result map[string]any
	if err := s.Exclusive(ctx, "test", func(ctx context.Context) error {
		if s.Status().State != "paused" {
			t.Errorf("sync was not paused during the exclusive operation")
		}
		return cli.Decode(ctx, &result, "chats", "archive", "--chat", "x")
	}); err != nil {
		t.Fatalf("exclusive operation failed: %v", err)
	}
	if result["archived"] != true {
		t.Fatalf("unexpected result %v", result)
	}

	// And sync comes back on its own.
	waitState(t, s, "connected")

	// Concurrent sends queue behind an exclusive operation instead of failing.
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- s.Exclusive(ctx, "test", func(ctx context.Context) error {
			time.Sleep(300 * time.Millisecond)
			_, err := cli.Run(ctx, "chats", "archive", "--chat", "x")
			return err
		})
	}()
	time.Sleep(50 * time.Millisecond)
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- s.Delegated(ctx, func(ctx context.Context) error {
				_, err := cli.Run(ctx, "send", "text", "--to", "x", "--message", "hi")
				return err
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("operation failed under contention: %v", err)
		}
	}

	log, _ := os.ReadFile(filepath.Join(store, "calls.log"))
	if got := strings.Count(string(log), "delegated"); got != 6 {
		t.Fatalf("expected 6 delegated sends, log has %d:\n%s", got, log)
	}
	if got := strings.Count(string(log), "exclusive"); got != 2 {
		t.Fatalf("expected 2 exclusive operations, log has %d:\n%s", got, log)
	}
	waitState(t, s, "connected")
	if st := s.Status(); st.Restarts != 0 {
		t.Fatalf("pauses must not count as crashes, status %+v", st)
	}
}
