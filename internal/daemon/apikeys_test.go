package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/panel"
)

const testAPIKey = "wamcp_key_0123456789abcdef0123456789abcdef"

// The MCP by URL answers only over HTTPS and only with an API key, and is
// off until a key exists; removing a key revokes it at once.
func TestTheMCPByURLWithAnAPIKey(t *testing.T) {
	d, _ := startDaemon(t, freePort(t), true)
	defer d.Stop()
	https := httptest.NewTLSServer(d.Remote().Handler())
	defer https.Close()
	call := func(base, key string) int {
		req, _ := http.NewRequest(http.MethodPost, base+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		resp, err := https.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if got := call(https.URL, testAPIKey); got != http.StatusNotFound {
		t.Errorf("with no keys, the MCP by URL must be off: %d", got)
	}
	file := filepath.Join(d.cfg.DataDir, APIKeysFile)
	sum := sha256.Sum256([]byte(testAPIKey))
	if err := os.WriteFile(file, []byte("# keys\n"+hex.EncodeToString(sum[:])+" hermes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := call(https.URL, ""); got != http.StatusUnauthorized {
		t.Errorf("no key: %d", got)
	}
	if got := call(https.URL, "wamcp_key_wrong_0123456789abcdef0123456789"); got != http.StatusUnauthorized {
		t.Errorf("a wrong key: %d", got)
	}
	if got := call(https.URL, testAPIKey); got != 200 {
		t.Errorf("the right key: %d", got)
	}
	// The connection is listed as straight to the server.
	req, _ := http.NewRequest(http.MethodPost, https.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"hermes-agent","version":"1"}}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	if resp, err := https.Client().Do(req); err == nil {
		resp.Body.Close()
	}
	clients, _ := d.primary.st.Clients(context.Background())
	if len(clients) == 0 || clients[0].Name != "hermes-agent" || clients[0].Via != "url" {
		t.Errorf("the API key's client: %+v", clients)
	}
	// Plain HTTP, not through an HTTPS proxy, is refused even with the key.
	plain := httptest.NewServer(d.Remote().Handler())
	defer plain.Close()
	req, _ = http.NewRequest(http.MethodPost, plain.URL+"/mcp", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("plain HTTP: %v %v", resp, err)
	}
	// Revoking: the line goes, the key stops at once.
	_ = os.WriteFile(file, []byte("# keys\n0000000000000000000000000000000000000000000000000000000000000000 other\n"), 0o600)
	if got := call(https.URL, testAPIKey); got != http.StatusUnauthorized {
		t.Errorf("a removed key: %d", got)
	}
}

// A key made by the panel works by URL, is listed by name only, and stops
// when revoked; lines written by hand stay.
func TestAPIKeysMadeAndRevoked(t *testing.T) {
	path := filepath.Join(t.TempDir(), APIKeysFile)
	sum := sha256.Sum256([]byte(testAPIKey))
	hand := hex.EncodeToString(sum[:]) + " by hand"
	if err := os.WriteFile(path, []byte("# mine\n"+hand+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	k := &apiKeys{path: path, now: time.Now}
	if _, _, err := k.Add("  "); err == nil {
		t.Error("a key with no name was made")
	}
	secret, key, err := k.Add("Hermes # Agent\n")
	if err != nil {
		t.Fatal(err)
	}
	if key.Name != "Hermes Agent" || len(key.ID) != 12 || !strings.HasPrefix(secret, "wamcp_") {
		t.Fatalf("made %+v (%d chars)", key, len(secret))
	}
	if ok, on, _ := apiKeyValid(path, secret); !ok || !on {
		t.Fatal("the new key is not valid")
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), secret) {
		t.Fatal("the key itself was written")
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
	list, _ := k.List()
	if len(list) != 2 || list[0].Name != "by hand" || list[1].ID != key.ID || list[1].Created.IsZero() {
		t.Fatalf("list %+v", list)
	}
	if err := k.Revoke("000000000000"); !errors.Is(err, panel.ErrNoAPIKey) {
		t.Errorf("an unknown id: %v", err)
	}
	if err := k.Revoke(key.ID); err != nil {
		t.Fatal(err)
	}
	if ok, _, _ := apiKeyValid(path, secret); ok {
		t.Error("a revoked key still works")
	}
	if ok, _, _ := apiKeyValid(path, testAPIKey); !ok {
		t.Error("the hand-written key went too")
	}
	if raw, _ := os.ReadFile(path); !strings.Contains(string(raw), "# mine") {
		t.Error("the comment went")
	}
}
