package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	// Plain HTTP, not through an HTTPS proxy, is refused even with the key.
	plain := httptest.NewServer(d.Remote().Handler())
	defer plain.Close()
	req, _ := http.NewRequest(http.MethodPost, plain.URL+"/mcp", strings.NewReader(`{}`))
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
