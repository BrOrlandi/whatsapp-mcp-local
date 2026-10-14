package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/appconfig"
)

// The command line's language: the environment, then config.json; a data
// folder in use from before the languages, with no config.json, stays in
// Portuguese, and a new one follows the system.
func TestLanguage(t *testing.T) {
	t.Setenv("WHATSAPP_MCP_LANG", "")
	fresh := t.TempDir()
	if got := language(fresh); got != "" {
		t.Errorf("a new data folder: %q, want the system's", got)
	}
	// Its first run marks it, so it keeps following the system once the
	// daemon has written state.db.
	if err := appconfig.MarkNew(fresh); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fresh, "state.db"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := language(fresh); got != "" {
		t.Errorf("a new data folder after its first run: %q, want the system's", got)
	}

	before := t.TempDir()
	if err := os.WriteFile(filepath.Join(before, "state.db"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := language(before); got != appconfig.LanguageBefore {
		t.Errorf("a data folder from before the languages: %q, want %q", got, appconfig.LanguageBefore)
	}
	if err := appconfig.MarkNew(before); err != nil {
		t.Fatal(err)
	}
	if got := language(before); got != appconfig.LanguageBefore {
		t.Errorf("MarkNew changed a folder in use to %q", got)
	}

	if err := appconfig.Save(before, appconfig.Config{Port: appconfig.DefaultPort, Language: "es"}); err != nil {
		t.Fatal(err)
	}
	if got := language(before); got != "es" {
		t.Errorf("config.json says es: %q", got)
	}

	t.Setenv("WHATSAPP_MCP_LANG", "en")
	if got := language(before); got != "en" {
		t.Errorf("WHATSAPP_MCP_LANG=en: %q", got)
	}
}
