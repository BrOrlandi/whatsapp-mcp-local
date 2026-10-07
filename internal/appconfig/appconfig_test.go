package appconfig

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestLoadDefaultsWhenMissing(t *testing.T) {
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != DefaultPort || !c.CloseToTray || !c.Autostart {
		t.Fatalf("a missing config.json must give the defaults, got %+v", c)
	}
}

func TestResolveProxy(t *testing.T) {
	dir := t.TempDir()
	c := Defaults()
	c.WhatsAppProxy = "socks5://100.101.102.103:1080"
	if err := Save(dir, c); err != nil {
		t.Fatal(err)
	}
	// A service need not inherit a shell variable to keep its proxy choice.
	previous, present := os.LookupEnv("WHATSAPP_MCP_PROXY")
	os.Unsetenv("WHATSAPP_MCP_PROXY")
	t.Cleanup(func() {
		if present {
			os.Setenv("WHATSAPP_MCP_PROXY", previous)
		} else {
			os.Unsetenv("WHATSAPP_MCP_PROXY")
		}
	})
	if got, err := ResolveProxy(dir); err != nil || string(got) != c.WhatsAppProxy {
		t.Fatalf("lost saved proxy: %q, %v", got, err)
	}
	for _, value := range []string{"direct", "", "http://proxy.example:8080"} {
		t.Setenv("WHATSAPP_MCP_PROXY", value)
		if got, err := ResolveProxy(dir); err != nil || string(got) != value {
			t.Fatalf("environment must override config: %q, %v", got, err)
		}
	}
	t.Setenv("WHATSAPP_MCP_PROXY", "http://user:private-password@host:bad")
	if _, err := ResolveProxy(dir); err == nil || strings.Contains(err.Error(), "private-password") {
		t.Fatalf("invalid proxy must fail without exposing credentials: %v", err)
	}
}

func TestSaveProxyPreservesSettingsAndPrivateCredentials(t *testing.T) {
	dir := t.TempDir()
	want := Config{Port: 48901, CloseToTray: false, Autostart: false, Seen: []string{"first-run"}}
	if err := Save(dir, want); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		// A hand-written config may have started with a permissive mode.
		if err := os.Chmod(Path(dir), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	endpoint := "http://demo:p%40ss%25word@proxy.example:8080"
	if err := SaveProxy(dir, endpoint); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.WhatsAppProxy != endpoint || got.Port != want.Port || got.Autostart != want.Autostart || got.CloseToTray != want.CloseToTray || len(got.Seen) != 1 || got.Seen[0] != want.Seen[0] {
		t.Fatal("saving a service proxy must preserve credentials and other app settings")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(Path(dir))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("credential-bearing config is not owner-only: %o", info.Mode().Perm())
		}
	}
	before, err := os.ReadFile(Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveProxy(dir, "http://user:private-password@host:bad"); err == nil || strings.Contains(err.Error(), "private-password") {
		t.Fatal("invalid proxy must fail without revealing credentials")
	}
	after, err := os.ReadFile(Path(dir))
	if err != nil || string(after) != string(before) {
		t.Fatal("invalid proxy overwrote the previous config")
	}
	if err := SaveProxy(dir, ""); err != nil {
		t.Fatal(err)
	}
	if got, err := Load(dir); err != nil || got.WhatsAppProxy != "" {
		t.Fatal("an empty service proxy must clear the saved override")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	want := Config{Port: 47900, CloseToTray: false, Autostart: true, Seen: []string{"close"}}
	if err := Save(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Port != 47900 || got.CloseToTray || !got.Autostart || !got.HasSeen("close") || got.HasSeen("other") {
		t.Fatalf("round trip lost something: %+v", got)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("an atomic save must leave only config.json behind, found %d files", len(entries))
	}
}

func TestEnvironmentWinsOverConfig(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, Config{Port: 47900}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WHATSAPP_MCP_PORT", "")
	if got := ResolvePort(dir); got != 47900 {
		t.Fatalf("config.json's port = %d, want 47900", got)
	}
	t.Setenv("WHATSAPP_MCP_PORT", "48000")
	if got := ResolvePort(dir); got != 48000 {
		t.Fatalf("WHATSAPP_MCP_PORT must win, got %d", got)
	}
	if got := ResolvePort(t.TempDir()); got != 48000 {
		t.Fatalf("WHATSAPP_MCP_PORT must win without a config too, got %d", got)
	}
	t.Setenv("WHATSAPP_MCP_PORT", "")
	if got := ResolvePort(t.TempDir()); got != DefaultPort {
		t.Fatalf("without either, the default: got %d", got)
	}
}
