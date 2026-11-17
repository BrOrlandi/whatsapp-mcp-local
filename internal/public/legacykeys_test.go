package public

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A server that comes from a 2.0.0-beta keeps its keys: the ones in the old
// file answer at /mcp as they are, without the new prefix.
func TestKeysOfTheEarlierVersionStillWork(t *testing.T) {
	s, st, _ := agentServer(t)
	ctx := context.Background()
	now := time.Now()
	s.access.now = func() time.Time { return now }
	const byHand = "wamcp_0123456789abcdef0123456789abcdef0123456789"
	const made = "wamcp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const forgotten = "wamcp_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const fresh = "wamcp_cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	old := now.Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339)
	recent := now.Add(-time.Hour).UTC().Format(time.RFC3339)
	path := filepath.Join(t.TempDir(), LegacyKeysFile)
	file := "# keys of the remote mode\n" +
		hashSecret(byHand) + " hermes\n" +
		hashSecret(made) + " Claude Code # created " + old + " tool claude-code\n" +
		hashSecret(forgotten) + " Esquecida # created " + old + " unused\n" +
		hashSecret(fresh) + " Nova # created " + recent + " unused\n" +
		"not-a-hash someone\n\n"
	if err := os.WriteFile(path, []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	call0 := func(key string) int {
		return serve(s, fromTunnel("POST", "/mcp", map[string]string{"Authorization": "Bearer " + key}, "{}")).Code
	}
	if n, found, err := ImportLegacyKeys(ctx, st, path, now); err != nil || n != 4 || found != 4 {
		t.Fatalf("imported %d of %d keys: %v", n, found, err)
	}
	// A server without such keys does not even look one up.
	if code := call0(byHand); code != 401 {
		t.Fatalf("an earlier key before the server accepts them: %d", code)
	}
	s.AcceptEarlierKeys()
	call := func(key string) *httptest.ResponseRecorder {
		return serve(s, fromTunnel("POST", "/mcp", map[string]string{"Authorization": "Bearer " + key}, "{}"))
	}
	for key, want := range map[string]string{byHand: "apikey/hermes/full", made: "apikey/Claude Code/full", fresh: "apikey/Nova/full"} {
		if rec := call(key); rec.Code != 200 || rec.Body.String() != want {
			t.Errorf("an earlier key: %d %s, want %s", rec.Code, rec.Body, want)
		}
	}
	// One nobody used within its day had expired there, and has here.
	if rec := call(forgotten); rec.Code != 401 {
		t.Errorf("a key that expired unused: %d", rec.Code)
	}
	for _, wrong := range []string{"wamcp_0123456789abcdef0123456789abcdef012345678", "short", refreshPrefix + byHand} {
		if rec := call(wrong); rec.Code != 401 {
			t.Errorf("a wrong key: %d", rec.Code)
		}
	}
	// Only hashes are kept, and the file is left as it was.
	if after, _ := os.ReadFile(path); string(after) != file {
		t.Error("the earlier file was changed")
	}
	// A revoked key does not come back at the next start.
	keys, _ := Keys(ctx, st, now)
	for _, k := range keys {
		if k.Name == "hermes" {
			if err := RevokeKey(ctx, st, k.ID, now); err != nil {
				t.Fatal(err)
			}
		}
	}
	if n, found, err := ImportLegacyKeys(ctx, st, path, now); err != nil || n != 0 || found != 4 {
		t.Fatalf("a second start imported %d of %d keys: %v", n, found, err)
	}
	if rec := call(byHand); rec.Code != 401 {
		t.Errorf("a revoked earlier key: %d", rec.Code)
	}
	// No file, nothing to do.
	if n, found, err := ImportLegacyKeys(ctx, st, filepath.Join(t.TempDir(), "none"), now); err != nil || n != 0 || found != 0 {
		t.Fatalf("no file: %d %d %v", n, found, err)
	}
}
