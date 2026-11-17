package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A value a caller gives never becomes a wacli flag: wacli honours flags
// anywhere on the line, so positional values go after "--", and every
// recipient or number is checked for its shape first.
func TestCallerValuesNeverBecomeWacliFlags(t *testing.T) {
	base := ""
	if runtime.GOOS != "windows" {
		base = "/tmp"
	}
	root, err := os.MkdirTemp(base, "daemon-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	store := filepath.Join(root, "s")
	_ = os.MkdirAll(store, 0o700)
	_ = os.WriteFile(filepath.Join(store, "AUTHED"), nil, 0o600)
	seedStore(t, store)
	port := freePort(t)
	d, err := Start(Config{Port: port, WacliBin: fakeWacli, StoreDir: store, DataDir: filepath.Join(root, "data")})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Stop()
	waitSync(t, d, "connected")
	argsLog := func() string { b, _ := os.ReadFile(filepath.Join(store, "args.log")); return string(b) }

	for _, bad := range []string{"--store=" + filepath.Join(root, "elsewhere"), "--account=x", "-h"} {
		callTool(t, port, "check_numbers", map[string]any{"numbers": []string{"5511900000001", bad}})
		callTool(t, port, "send_text_message", map[string]any{"to": bad, "text": "x"})
		callTool(t, port, "get_profile_picture", map[string]any{"to": bad})
		callTool(t, port, "mark_chat_read", map[string]any{"chat_jid": bad})
	}
	if log := argsLog(); strings.Contains(log, "--store="+filepath.Join(root, "elsewhere")) || strings.Contains(log, "--account=x") || strings.Contains(log, " -h") {
		t.Errorf("a caller's value reached wacli as a flag:\n%s", log)
	}
	// A valid check puts its numbers after "--".
	callTool(t, port, "check_numbers", map[string]any{"numbers": []string{"+55 11 90000-0001", "5511900000002@s.whatsapp.net"}})
	if log := argsLog(); !strings.Contains(log, "contacts check -- +55 11 90000-0001 5511900000002@s.whatsapp.net") {
		t.Errorf("the numbers are not after --:\n%s", log)
	}
	// A search for something that looks like a flag searches for it.
	callTool(t, port, "search_messages", map[string]any{"query": "--store=x"})
	if log := argsLog(); !strings.Contains(log, " -- --store=x") {
		t.Errorf("the query is not after --:\n%s", log)
	}
}
