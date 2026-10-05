package legacy

import (
	"os"
	"path/filepath"
	"testing"
)

const plist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.brorlandi.whatsapp-mcp-v2</string>
	<key>ProgramArguments</key>
	<array>
		<string>/Users/x/.local/bin/whatsapp-mcp-v2</string>
		<string>serve</string>
	</array>
	<key>EnvironmentVariables</key>
	<dict>
		<key>WACLI_STORE_DIR</key>
		<string>/Users/x/.wacli</string>
		<key>WHATSAPP_MCP_PORT</key>
		<string>47821</string>
	</dict>
	<key>RunAtLoad</key>
	<true/>
	<key>ThrottleInterval</key>
	<integer>10</integer>
</dict>
</plist>`

func TestParsePlist(t *testing.T) {
	args, env := parsePlist([]byte(plist))
	if len(args) != 2 || args[0] != "/Users/x/.local/bin/whatsapp-mcp-v2" || args[1] != "serve" {
		t.Fatalf("args = %q", args)
	}
	if env["WACLI_STORE_DIR"] != "/Users/x/.wacli" || env["WHATSAPP_MCP_PORT"] != "47821" || len(env) != 2 {
		t.Fatalf("env = %v", env)
	}
}

func TestMoveTakesTheStoreAndTheData(t *testing.T) {
	old := t.TempDir()
	in := &Install{StoreDir: filepath.Join(old, "wacli"), DataDir: filepath.Join(old, "data")}
	for path, body := range map[string]string{
		"wacli/session.db": "session", "wacli/wacli.db": "messages",
		"data/state.db": "state", "data/models/model.bin": "model", "data/media/a.ogg": "audio", "data/other": "stays",
	} {
		p := filepath.Join(old, path)
		_ = os.MkdirAll(filepath.Dir(p), 0o700)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app := t.TempDir()
	store, data := filepath.Join(app, "wacli"), app
	if err := os.MkdirAll(store, 0o700); err != nil { // an empty store the app made is fine
		t.Fatal(err)
	}
	if err := Move(t.Context(), in, store, data); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"wacli/session.db": "session", "state.db": "state", "models/model.bin": "model", "media/a.ogg": "audio"} {
		if got, err := os.ReadFile(filepath.Join(app, path)); err != nil || string(got) != want {
			t.Errorf("%s = %q, %v", path, got, err)
		}
	}
	if _, err := os.Stat(in.StoreDir); !os.IsNotExist(err) {
		t.Error("the old store must be moved, not copied")
	}
	if _, err := os.Stat(filepath.Join(in.DataDir, "other")); err != nil {
		t.Error("what the app does not use stays where it was")
	}
}

func TestMoveRefusesToOverwriteAPairedStore(t *testing.T) {
	old := t.TempDir()
	in := &Install{StoreDir: filepath.Join(old, "wacli"), DataDir: filepath.Join(old, "data")}
	_ = os.MkdirAll(in.StoreDir, 0o700)
	_ = os.WriteFile(filepath.Join(in.StoreDir, "session.db"), []byte("old"), 0o600)
	app := t.TempDir()
	store := filepath.Join(app, "wacli")
	_ = os.MkdirAll(store, 0o700)
	_ = os.WriteFile(filepath.Join(store, "session.db"), []byte("new"), 0o600)
	if err := Move(t.Context(), in, store, app); err == nil {
		t.Fatal("moving over a store that already has a session must be refused")
	}
	if got, _ := os.ReadFile(filepath.Join(in.StoreDir, "session.db")); string(got) != "old" {
		t.Fatal("a refused move must leave the old store in place")
	}
}
