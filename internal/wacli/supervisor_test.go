package wacli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// fakeWacli behaves like wacli where the supervisor depends on it: sync holds
// a lock and opens the delegate socket, exclusive commands fail while the lock
// is held, and delegated commands succeed while the socket is up.
const fakeWacli = `#!/bin/bash
STORE="$WACLI_STORE_DIR"
[ "$1" = "--json" ] && shift
[ "$1" = "--read-only" ] && shift
# Like wacli, the store lock is an flock, which the kernel releases however
# the holder dies.
locked() { python3 -c 'import fcntl,sys
f=open(sys.argv[1],"a")
try: fcntl.flock(f,fcntl.LOCK_EX|fcntl.LOCK_NB)
except OSError: sys.exit(0)
sys.exit(1)' "$STORE/LOCK"; }
case "$1 $2" in
"auth status")
  if [ -f "$STORE/AUTHED" ]; then a=true; else a=false; fi
  echo "{\"success\":true,\"data\":{\"authenticated\":$a,\"phone\":\"5511912345678\"},\"error\":null}" ;;
"auth --events")
  if locked; then echo "store is locked" >&2; exit 1; fi
  echo '{"event":"auth_starting","ts":1}' >&2
  echo '{"event":"qr_code","data":{"code":"2@first"},"ts":1}' >&2
  sleep 0.3
  echo '{"event":"qr_code","data":{"code":"2@second"},"ts":1}' >&2
  sleep 0.3
  touch "$STORE/AUTHED"
  echo '{"event":"connected","ts":1}' >&2
  echo '{"event":"history_sync","data":{"conversations":12},"ts":1}' >&2
  echo '{"event":"progress","data":{"messages_synced":340},"ts":1}' >&2
  if [ -f "$STORE/HANG" ]; then
    # A large account: the history keeps coming long after the link.
    exec python3 -c 'import signal,sys,time
signal.signal(signal.SIGINT, lambda *_: sys.exit(130))
while True: time.sleep(1)'
  fi
  sleep 0.3
  echo '{"event":"idle_exit","data":{"messages_synced":512},"ts":1}' >&2 ;;
"sync --follow")
  exec python3 -c '
import socket, os, sys, signal, fcntl
p = sys.argv[1]
def stop(*_):
    try: os.unlink(p)
    except OSError: pass
    sys.exit(0)
signal.signal(signal.SIGINT, stop); signal.signal(signal.SIGTERM, stop)
lock = open(os.path.join(os.path.dirname(p), "LOCK"), "a")
try: fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
except OSError:
    sys.stderr.write("store is locked\n"); sys.exit(1)
lock.truncate(0); lock.write("pid=%d\n" % os.getpid()); lock.flush()
try: os.unlink(p)
except OSError: pass
s = socket.socket(socket.AF_UNIX); s.bind(p); s.listen(8)
sys.stderr.write("{\"event\":\"connected\",\"ts\":1}\n"); sys.stderr.flush()
while True:
    c, _ = s.accept(); c.close()
' "$STORE/.send.sock" ;;
"chats archive")
  if locked; then echo '{"success":false,"data":null,"error":"store is locked by another process"}'; exit 1; fi
  echo "exclusive" >> "$STORE/calls.log"
  echo '{"success":true,"data":{"archived":true},"error":null}' ;;
"send text")
  if locked && [ ! -S "$STORE/.send.sock" ]; then echo '{"success":false,"data":null,"error":"store is locked"}'; exit 1; fi
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

	if err := os.WriteFile(filepath.Join(store, "AUTHED"), nil, 0o600); err != nil {
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

func fakeStore(t *testing.T) (string, *CLI) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is needed for the fake wacli")
	}
	store, err := os.MkdirTemp("/tmp", "wacli-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(store) })
	bin := filepath.Join(store, "wacli")
	if err := os.WriteFile(bin, []byte(fakeWacli), 0o755); err != nil {
		t.Fatal(err)
	}
	return store, &CLI{Bin: bin, StoreDir: store}
}

func TestPairingFromQRToRunningSync(t *testing.T) {
	_, cli := fakeStore(t)
	s := NewSupervisor(cli, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	waitState(t, s, "not_paired")
	if err := s.StartPairing(""); err != nil {
		t.Fatal(err)
	}
	if err := s.StartPairing(""); err != ErrPairingRunning {
		t.Fatalf("a second pairing must be refused while one runs, got %v", err)
	}

	seen := map[string]bool{}
	var qrs = map[string]bool{}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		p := s.Pairing()
		seen[p.State] = true
		if p.QR != "" {
			qrs[p.QR] = true
		}
		if p.State == "done" || p.State == "error" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	p := s.Pairing()
	if p.State != "done" {
		t.Fatalf("pairing ended as %q (%s), states seen %v", p.State, p.Error, seen)
	}
	if !seen["qr"] || !seen["syncing"] {
		t.Fatalf("expected to pass through qr and syncing, saw %v", seen)
	}
	if !qrs["2@first"] || !qrs["2@second"] {
		t.Fatalf("the QR code must follow wacli as it rotates, saw %v", qrs)
	}
	if p.Synced != 512 || p.Conversations != 12 {
		t.Fatalf("progress not tracked: %+v", p)
	}
	if p.QR != "" {
		t.Fatal("a finished pairing must not keep the QR code")
	}
	// And sync takes over as soon as pairing ends.
	waitState(t, s, "connected")
	account, err := s.Account(ctx)
	if err != nil || !account.Authenticated || account.Phone == "" {
		t.Fatalf("account after pairing: %+v, %v", account, err)
	}
}

// Switching from the QR code to a phone code cancels one pairing and starts
// the next at once. The cancelled one must not report its cancellation as the
// new pairing's error, which is what the panel used to show next to the code.
func TestSwitchingPairingDoesNotReportTheCancelledOne(t *testing.T) {
	_, cli := fakeStore(t)
	s := NewSupervisor(cli, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	waitState(t, s, "not_paired")
	if err := s.StartPairing(""); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for s.Pairing().State != "qr" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	s.CancelPairing()
	if err := s.StartPairing("5511912345678"); err != nil {
		t.Fatalf("a new pairing must start right after a cancel: %v", err)
	}
	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		p := s.Pairing()
		if p.State == "error" || p.State == "cancelled" {
			t.Fatalf("the new pairing reported %q (%s)", p.State, p.Error)
		}
		if p.State == "done" {
			if p.Phone != "5511912345678" {
				t.Fatalf("the finished pairing is not the new one: %+v", p)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the new pairing never finished: %+v", s.Pairing())
}

// A large account keeps sending history long after the phone has linked. The
// pairing must hand over to sync shortly after the link rather than hold the
// store, and every send, until the history is all in.
func TestPairingHandsOverToSyncAfterLinking(t *testing.T) {
	store, cli := fakeStore(t)
	if err := os.WriteFile(filepath.Join(store, "HANG"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	old := HandoverAfter
	HandoverAfter = 300 * time.Millisecond
	defer func() { HandoverAfter = old }()

	s := NewSupervisor(cli, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	waitState(t, s, "not_paired")
	if err := s.StartPairing(""); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for s.Pairing().State != "done" && time.Now().Before(deadline) {
		if st := s.Pairing().State; st == "error" {
			t.Fatalf("pairing failed: %s", s.Pairing().Error)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if st := s.Pairing().State; st != "done" {
		t.Fatalf("auth was not handed over; pairing is %q", st)
	}
	waitState(t, s, "connected")
}

// A daemon killed with SIGKILL leaves its sync running, holding the store.
// The next daemon must stop that orphan and take over.
func TestSupervisorReclaimsAnOrphanedSync(t *testing.T) {
	store, cli := fakeStore(t)
	if err := os.WriteFile(filepath.Join(store, "AUTHED"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// The previous daemon's sync, still running after its parent died.
	orphan := exec.Command(cli.Bin, "sync", "--follow")
	orphan.Env = append(os.Environ(), "WACLI_STORE_DIR="+store)
	orphan.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := orphan.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() { _ = orphan.Wait(); close(exited) }()
	defer func() { _ = orphan.Process.Kill() }()
	deadline := time.Now().Add(5 * time.Second)
	for lockHolder(store) != orphan.Process.Pid && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if lockHolder(store) != orphan.Process.Pid {
		t.Fatal("the orphan never took the lock")
	}

	old := isOrphanedWacli
	isOrphanedWacli = func(pid int) bool { return pid == orphan.Process.Pid }
	defer func() { isOrphanedWacli = old }()

	s := NewSupervisor(cli, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	select {
	case <-exited:
	case <-time.After(15 * time.Second):
		t.Fatal("the orphaned sync was not stopped")
	}
	waitState(t, s, "connected")
	if st := s.Status(); st.Restarts != 0 {
		t.Fatalf("taking over should not look like a crash loop: %+v", st)
	}
}
