package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
)

// A caller that may use no tool gets "tools": [], never null.
func TestNoToolsIsAnEmptyListNotNull(t *testing.T) {
	s := New(Config{})
	ctx := caller.With(context.Background(), caller.Caller{Kind: caller.OAuth, Categories: []string{}})
	out := string(s.Handle(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)))
	if !strings.Contains(out, `"tools":[]`) {
		t.Fatalf("got %s", out)
	}
}
