package main

import (
	"os"
	"strings"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/appconfig"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/public"
)

func TestServerSetupKeepsTheServerMode(t *testing.T) {
	dir := t.TempDir()
	withPassword(t, "a throwaway server password")
	installed := 0
	old := installServiceFn
	installServiceFn = func(config) (string, error) { installed++; return "", nil }
	t.Cleanup(func() { installServiceFn = old })
	cfg := config{DataDir: dir, Port: freePort(t)}
	// Without a daemon answering, setup with --no-service keeps the settings
	// and touches nothing.
	out, err := run(t, serverCmd, cfg, "setup", "--url", "https://wa.example.com/", "--trusted-proxy", "192.0.2.5", "--no-service")
	if err != nil {
		t.Fatal(err)
	}
	if installed != 0 || !strings.Contains(out, "https://wa.example.com") || strings.Contains(out, "a throwaway") {
		t.Fatalf("setup: %q (installed %d)", out, installed)
	}
	c, _ := appconfig.Load(dir)
	if c.Server == nil || c.Server.PublicURL != "https://wa.example.com" || c.Server.TrustedProxy != "192.0.2.5" {
		t.Fatalf("config.json: %+v", c.Server)
	}
	if _, err := os.Stat(public.PasswordPath(dir)); err != nil {
		t.Fatal("no password kept")
	}
	// serve reads it, and a variable wins.
	pub, err := public.FromEnv(serverEnv(dir), dir)
	if err != nil || pub == nil || pub.Origin != "https://wa.example.com" || pub.Listen != public.DefaultListen {
		t.Fatalf("serve's view: %+v %v", pub, err)
	}
	t.Setenv(public.EnvListen, "127.0.0.1:48000")
	if pub, _ := public.FromEnv(serverEnv(dir), dir); pub.Listen != "127.0.0.1:48000" {
		t.Fatalf("the variable does not win: %s", pub.Listen)
	}
	// status says so; off turns it off.
	out, _ = run(t, serverCmd, cfg, "status", "--json")
	if !strings.Contains(out, `"enabled":true`) {
		t.Fatalf("status: %s", out)
	}
	if _, err := run(t, serverCmd, cfg, "off", "--no-service"); err != nil {
		t.Fatal(err)
	}
	if pub, _ := public.FromEnv(serverEnv(dir), dir); pub != nil {
		t.Fatal("still on after off")
	}
}

func TestServerSetupRefusals(t *testing.T) {
	withPassword(t, "short")
	cfg := config{DataDir: t.TempDir()}
	if _, err := run(t, serverCmd, cfg, "setup", "--url", "https://wa.example.com", "--no-service"); exitCode(err) != exitUsage {
		t.Errorf("a short password: %v", err)
	}
	withPassword(t, "a long enough password")
	if _, err := run(t, serverCmd, cfg, "setup", "--url", "https://wa.example.com/painel", "--no-service"); exitCode(err) != exitUsage {
		t.Errorf("a URL with a path: %v", err)
	}
	if c, _ := appconfig.Load(cfg.DataDir); c.Server != nil {
		t.Error("a refused setup was kept")
	}
	if _, err := run(t, serverCmd, cfg, "setup", "--no-service"); exitCode(err) != exitUsage {
		t.Errorf("no URL from a pipe: %v", err)
	}
	t.Setenv("WHATSAPP_MCP_CONTAINER", "1")
	if _, err := run(t, serverCmd, cfg, "setup", "--url", "https://wa.example.com"); exitCode(err) != exitConflict {
		t.Errorf("in Docker: %v", err)
	}
}

func TestHealthcheck(t *testing.T) {
	if _, err := run(t, healthcheckCmd, config{Port: freePort(t)}); exitCode(err) != exitNotRunning {
		t.Fatalf("nothing answering: %v", err)
	}
	cfg := startDaemon(t, map[string]string{"AUTHED": ""})
	if _, err := run(t, healthcheckCmd, cfg); err != nil {
		t.Fatalf("a daemon answering: %v", err)
	}
}

// Without --no-service, setup installs the service (stubbed here) and waits
// for the daemon to answer.
func TestServerSetupInstallsTheService(t *testing.T) {
	withPassword(t, "a throwaway server password")
	cfg := startDaemon(t, map[string]string{"AUTHED": ""})
	installed := 0
	old := installServiceFn
	installServiceFn = func(config) (string, error) { installed++; return "", nil }
	t.Cleanup(func() { installServiceFn = old })
	out, err := run(t, serverCmd, cfg, "setup", "--url", "https://wa.example.com")
	if err != nil || installed != 1 || !strings.Contains(out, "https://wa.example.com") {
		t.Fatalf("setup: %v, installed %d: %q", err, installed, out)
	}
}
