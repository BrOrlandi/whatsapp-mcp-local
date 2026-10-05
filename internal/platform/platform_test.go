package platform

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// The test binary doubles as a child process: with the variable set, it waits
// for an interrupt and records that it stopped cleanly.
func TestMain(m *testing.M) {
	if marker := os.Getenv("PLATFORM_TEST_CHILD"); marker != "" {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		_ = os.WriteFile(marker+".ready", nil, 0o600)
		select {
		case <-c:
			_ = os.WriteFile(marker, []byte("clean"), 0o600)
			os.Exit(0)
		case <-time.After(30 * time.Second):
			os.Exit(3)
		}
	}
	os.Exit(m.Run())
}

func startChild(t *testing.T) (*exec.Cmd, string, chan error) {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "stopped")
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "PLATFORM_TEST_CHILD="+marker)
	Prepare(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := Started(cmd); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker + ".ready"); err == nil {
			return cmd, marker, done
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the child never started")
	return nil, "", nil
}

func TestInterruptStopsTheChildCleanly(t *testing.T) {
	cmd, marker, done := startChild(t)
	if !Alive(cmd.Process.Pid) {
		t.Fatal("a running child must be alive")
	}
	if err := Interrupt(cmd.Process.Pid); err != nil {
		t.Fatalf("interrupt: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("the child did not exit cleanly: %v", err)
		}
	case <-time.After(10 * time.Second):
		_ = Kill(cmd.Process.Pid)
		t.Fatal("the child ignored the interrupt")
	}
	if body, _ := os.ReadFile(marker); string(body) != "clean" {
		t.Fatal("the child was not given the chance to stop on its own")
	}
	if Alive(cmd.Process.Pid) {
		t.Fatal("a reaped child must not be alive")
	}
}

func TestKillStopsTheChild(t *testing.T) {
	cmd, _, done := startChild(t)
	if err := Kill(cmd.Process.Pid); err != nil {
		t.Fatalf("kill: %v", err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the child survived a kill")
	}
}

func TestDataDirFollowsTheEnvironment(t *testing.T) {
	t.Setenv("WHATSAPP_MCP_DATA", "/somewhere/else")
	if dir, _ := DataDir(); dir != "/somewhere/else" {
		t.Fatalf("DataDir = %q", dir)
	}
	if dir, _ := LogDir(); dir != filepath.Join("/somewhere/else", "logs") {
		t.Fatalf("LogDir = %q", dir)
	}
	t.Setenv("WHATSAPP_MCP_DATA", "")
	if dir, err := DataDir(); err != nil || dir == "" {
		t.Fatalf("DataDir = %q, %v", dir, err)
	}
}
