package appconfig

import (
	"os"
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
