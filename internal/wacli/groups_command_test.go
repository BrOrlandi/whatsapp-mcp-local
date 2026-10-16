package wacli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A wacli without a groups subcommand answers with the groups help instead.
func TestHasGroupsCommandReadsTheUsageLine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script stand-in")
	}
	bin := filepath.Join(t.TempDir(), "wacli")
	script := "#!/bin/sh\nif [ \"$2\" = photo ]; then printf 'Usage:\\n  wacli groups photo [command]\\n'; else printf 'Usage:\\n  wacli groups [command]\\n'; fi\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &CLI{Bin: bin, StoreDir: t.TempDir()}
	if !c.HasGroupsCommand(context.Background(), "photo") {
		t.Error("photo should be found")
	}
	if c.HasGroupsCommand(context.Background(), "admin-add") {
		t.Error("admin-add should be missing")
	}
}

func TestHasSubcommandChecksTheParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script stand-in")
	}
	bin := filepath.Join(t.TempDir(), "wacli")
	script := "#!/bin/sh\nif [ \"$1 $2\" = 'contacts save' ]; then printf 'Usage:\\n  wacli contacts save --jid PHONE_OR_JID --name NAME [flags]\\n'; else printf 'Usage:\\n  wacli %s [command]\\n' \"$1\"; fi\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &CLI{Bin: bin, StoreDir: t.TempDir()}
	if !c.HasSubcommand(context.Background(), "contacts", "save") {
		t.Error("contacts save should be found")
	}
	if c.HasSubcommand(context.Background(), "groups", "save") {
		t.Error("groups save should be missing")
	}
}
