package wacli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/platform"
)

// fakeWacli is testdata/fakewacli, built once for the system the tests run
// on: it behaves like wacli where the supervisor depends on it.
var fakeWacli string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakewacli-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fakeWacli = filepath.Join(dir, platform.ExeName("wacli"))
	if out, err := exec.Command("go", "build", "-o", fakeWacli, "./testdata/fakewacli").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building the fake wacli: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// shortTempDir is a store whose socket path fits the ~104 bytes Unix sockets
// allow, which t.TempDir on macOS exceeds.
func shortTempDir(t *testing.T) string {
	t.Helper()
	base := ""
	if runtime.GOOS != "windows" {
		base = "/tmp"
	}
	dir, err := os.MkdirTemp(base, "wacli-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

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
	store, cli := fakeStore(t)
	if err := os.WriteFile(filepath.Join(store, "AUTHED"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
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
	store := shortTempDir(t)
	return store, &CLI{Bin: fakeWacli, StoreDir: store}
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
	platform.Prepare(orphan)
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
