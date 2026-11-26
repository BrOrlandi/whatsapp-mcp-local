package mcp

import (
	"context"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

// A tool signed in with OAuth is listed by the name approved on the consent
// page, not by the one its MCP library gives at initialize ("mcp"); any
// other caller keeps the name it gives.
func TestAnOAuthToolGoesByItsApprovedName(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := New(Config{State: st})
	init := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"mcp","version":"1.2.0"}}}`)
	oauth := caller.With(context.Background(), caller.Caller{Kind: caller.OAuth, ConnectionID: "c_1", ConnectionName: "Orca", Client: "oauth:c_1"})
	s.Handle(oauth, init)
	key := caller.With(context.Background(), caller.Caller{Kind: caller.APIKey, ConnectionID: "k_1", ConnectionName: "minha chave", Client: "apikey:k_1"})
	s.Handle(key, []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"n8n","version":"1"}}}`))

	clients, err := st.Clients(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range clients {
		got[c.Name] = c.Version
	}
	if len(got) != 2 || got["Orca"] != "1.2.0" || got["n8n"] != "1" {
		t.Fatalf("clients recorded: %v", got)
	}
}
