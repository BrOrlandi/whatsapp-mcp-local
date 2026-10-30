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
