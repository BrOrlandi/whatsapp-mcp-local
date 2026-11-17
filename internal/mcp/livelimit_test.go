package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
)

func TestLiveLookupsAreLimitedPerConnection(t *testing.T) {
	l := newLiveLimits()
	now := time.Now()
	l.now = func() time.Time { return now }
	agent := caller.With(context.Background(), caller.Caller{Kind: caller.APIKey, ConnectionID: "k_1"})
	other := caller.With(context.Background(), caller.Caller{Kind: caller.OAuth, ConnectionID: "c_1"})
	if ok, _ := l.allow(agent, liveBurst); !ok {
		t.Fatal("the first burst was refused")
	}
	if ok, wait := l.allow(agent, 1); ok || wait <= 0 {
		t.Fatal("past the burst, a lookup went through")
	}
	if ok, _ := l.allow(other, 5); !ok {
		t.Fatal("another connection was limited by the first")
	}
	now = now.Add(liveEvery)
	if ok, _ := l.allow(agent, 1); !ok {
		t.Fatal("after a minute, one more lookup was refused")
	}
	// Programs on this computer are not limited.
	for i := 0; i < 10; i++ {
		if ok, _ := l.allow(context.Background(), liveBurst); !ok {
			t.Fatal("a local call was limited")
		}
	}
}

// The three tools that ask WhatsApp check the limit before they run.
func TestLiveToolsCheckTheLimitFirst(t *testing.T) {
	s := New(Config{})
	ctx := caller.With(context.Background(), caller.Caller{Kind: caller.APIKey, ConnectionID: "k_2"})
	s.live.allow(ctx, liveBurst)
	for name, args := range map[string]string{
		"check_numbers":       `{"numbers":["5511900000001"]}`,
		"get_profile_picture": `{"to":"5511900000001"}`,
		"get_group":           `{"group_jid":"123-456@g.us","live":true}`,
	} {
		out := string(s.Handle(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+name+`","arguments":`+args+`}}`)))
		if !strings.Contains(out, "too many numbers or groups") {
			t.Errorf("%s: %.200s", name, out)
		}
	}
}
