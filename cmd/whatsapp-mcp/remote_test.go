package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPIKeyCommands(t *testing.T) {
	cfg := config{DataDir: t.TempDir()}
	out, err := run(t, apikeyCmd, cfg, "create", "--name", "Claude Code", "--read-only")
	if err != nil {
		t.Fatal(err)
	}
	key := strings.SplitN(out, "\n", 2)[0]
	if !strings.HasPrefix(key, "wamcp_key_") || len(key) < 50 {
		t.Fatalf("the key line: %q", key)
	}
	out, err = run(t, apikeyCmd, cfg, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, key) {
		t.Fatal("list shows the key")
	}
	var list struct {
		Keys []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			ReadOnly bool   `json:"read_only"`
		} `json:"keys"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil || len(list.Keys) != 1 || list.Keys[0].Name != "Claude Code" || !list.Keys[0].ReadOnly {
		t.Fatalf("list: %v %s", err, out)
	}
	if _, err := run(t, apikeyCmd, cfg, "revoke", list.Keys[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, apikeyCmd, cfg, "revoke", list.Keys[0].ID); exitCode(err) != exitUsage {
		t.Fatalf("revoking twice: %v", err)
	}
	if _, err := run(t, apikeyCmd, cfg, "create"); exitCode(err) != exitUsage {
		t.Fatalf("a key with no name: %v", err)
	}
	out, _ = run(t, apikeyCmd, cfg, "list", "--json")
	if !strings.Contains(out, `"keys":[]`) {
		t.Fatalf("after revoking: %s", out)
	}
}

func TestBridgeURL(t *testing.T) {
	cfg := config{Port: 47821, Token: "local-token"}
	// Without --url, the local daemon, as always.
	opts, err := bridgeOptions(cfg, nil)
	if err != nil || opts.URL() != "http://127.0.0.1:47821/mcp" || opts.Token != "local-token" {
		t.Fatalf("local: %v %+v", err, opts)
	}
	t.Setenv(EnvAPIKey, "wamcp_key_from_env")
	opts, err = bridgeOptions(cfg, []string{"--url", "https://wa.example.com"})
	if err != nil || opts.URL() != "https://wa.example.com/mcp" || opts.Token != "wamcp_key_from_env" {
		t.Fatalf("--url: %v %+v", err, opts)
	}
	file := filepath.Join(t.TempDir(), "key")
	_ = os.WriteFile(file, []byte("wamcp_key_from_file\n"), 0o600)
	opts, err = bridgeOptions(cfg, []string{"--url", "https://wa.example.com/mcp", "--key-file", file})
	if err != nil || opts.Token != "wamcp_key_from_file" {
		t.Fatalf("--key-file: %v %+v", err, opts)
	}
	for _, args := range [][]string{
		{"--url", "http://wa.example.com/mcp"},                      // the key would travel in clear
		{"--url", "wa.example.com"},                                 // no scheme
		{"--url", "https://wa.example.com", "--key", "wamcp_key_x"}, // never an argument
		{"--key-file", file},                                        // a key file without --url
	} {
		if _, err := bridgeOptions(cfg, args); err == nil {
			t.Errorf("%q was accepted", args)
		}
	}
	t.Setenv(EnvAPIKey, "")
	if _, err := bridgeOptions(cfg, []string{"--url", "https://wa.example.com"}); err == nil {
		t.Error("--url with no key was accepted")
	}
	// http on loopback is fine, for a server on the same computer.
	t.Setenv(EnvAPIKey, "wamcp_key_x")
	if _, err := bridgeOptions(cfg, []string{"--url", "http://127.0.0.1:47822/mcp"}); err != nil {
		t.Errorf("loopback http: %v", err)
	}
}
