package public

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
)

// While an address waits for its wrong keys, the same wrong key sent again
// is refused without reading state.db, with when to try again; a valid key
// from there is still looked up and goes through.
func TestAStaleKeyIsNotLookedUpAgainWhileItsAddressWaits(t *testing.T) {
	s, st, _ := agentServer(t)
	var lookups atomic.Int32
	old := authenticate
	authenticate = func(a *access, ctx context.Context, bearer string) (caller.Caller, bool) {
		lookups.Add(1)
		return old(a, ctx, bearer)
	}
	t.Cleanup(func() { authenticate = old })
	stale := map[string]string{"Authorization": "Bearer " + KeyPrefix + strings.Repeat("a", 43), "X-Forwarded-For": "203.0.113.9"}
	for i := 0; i < 20; i++ {
		rec := serve(s, fromTunnel("POST", "/mcp", stale, "{}"))
		if rec.Code != 401 {
			t.Fatalf("a wrong key: %d", rec.Code)
		}
		if i >= freeTries && rec.Header().Get("Retry-After") == "" {
			t.Fatalf("try %d from a waiting address has no Retry-After", i+1)
		}
	}
	if n := lookups.Load(); n > freeTries {
		t.Fatalf("the same wrong key was looked up %d times", n)
	}
	key, _, _ := CreateKey(context.Background(), st, "x", false, time.Now())
	ok := map[string]string{"Authorization": "Bearer " + key, "X-Forwarded-For": "203.0.113.9"}
	if rec := serve(s, fromTunnel("POST", "/mcp", ok, "{}")); rec.Code != 200 {
		t.Fatalf("a valid key from the waiting address: %d", rec.Code)
	}
	// An address that does not wait looks a key up again: a refusal is
	// not remembered for it.
	before := lookups.Load()
	other := map[string]string{"Authorization": stale["Authorization"], "X-Forwarded-For": "203.0.113.10"}
	serve(s, fromTunnel("POST", "/mcp", other, "{}"))
	if lookups.Load() != before+1 {
		t.Fatal("an address that does not wait was not looked up")
	}
}
