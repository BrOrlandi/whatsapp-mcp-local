package mcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/wacli"
)

// A wacli error and wacli's own report name the server's folders: for an
// agent outside, they become short words, and only whole folders do.
func TestServerPathsInWacliTextAreReplaced(t *testing.T) {
	root := t.TempDir()
	s := New(Config{MediaDir: filepath.Join(root, "mcp", "media"), ExportDir: filepath.Join(root, "mcp", "exports"),
		CLI: &wacli.CLI{Bin: filepath.Join(root, "bin", "wacli"), StoreDir: filepath.Join(root, "wacli")}})
	store := filepath.Join(root, "wacli")
	in := toolError("wacli: stat %s: no such file or directory; exec %s failed", filepath.Join(store, "wacli.db"), filepath.Join(root, "bin", "wacli"))
	doctor := textResult(map[string]any{"wacli": map[string]any{"store_dir": store, "lock": filepath.Join(store, "LOCK")},
		"url": "https://wa.example.com/tmp" + root + "x/media"}, false)
	out := s.withoutServerPaths(in)["content"].([]any)[0].(map[string]any)["text"].(string)
	if strings.Contains(out, root) || !strings.Contains(out, "stat wacli-store") || !strings.Contains(out, "exec wacli failed") {
		t.Fatalf("a wacli error for an agent outside: %s", out)
	}
	out = s.withoutServerPaths(doctor)["content"].([]any)[0].(map[string]any)["text"].(string)
	esc, _ := json.Marshal(root)
	if strings.Contains(out, `"`+store) || !strings.Contains(out, `"wacli-store`) || !strings.Contains(out, "https://wa.example.com/tmp"+string(esc[1:len(esc)-1])+"x/media") {
		t.Fatalf("wacli's report for an agent outside: %s", out)
	}
	// Through a call: a local caller keeps the text as it is.
	ctx := caller.With(context.Background(), caller.Caller{Kind: caller.Local})
	if fromRemote(ctx) {
		t.Fatal("the local caller is remote")
	}
}
