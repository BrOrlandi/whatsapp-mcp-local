package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
)

func listedTools(t *testing.T, s *Server, ctx context.Context) []string {
	t.Helper()
	var resp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(s.Handle(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)), &resp); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range resp.Result.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// The local caller, with or without a mark in the context, sees every tool,
// in the same order and with the same definitions as before.
func TestTheLocalCallerListsEveryTool(t *testing.T) {
	s := New(Config{})
	plain := s.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	marked := s.Handle(caller.With(context.Background(), caller.Caller{Kind: caller.Local}), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	all, _ := json.Marshal(map[string]any{"id": 1, "jsonrpc": "2.0", "result": map[string]any{"tools": toolDefinitions()}})
	if string(plain) != string(all) || string(marked) != string(all) {
		t.Fatal("the local caller's tool list differs from every tool")
	}
}

func TestAReadOnlyCallerOnlyReads(t *testing.T) {
	s := New(Config{})
	ctx := caller.With(context.Background(), caller.Caller{Kind: caller.APIKey, Categories: caller.ReadOnlyCategories()})
	names := listedTools(t, s, ctx)
	if len(names) == 0 {
		t.Fatal("a read-only caller sees no tools")
	}
	for _, name := range names {
		if ToolCategory(name) != CategoryRead {
			t.Errorf("a read-only caller sees %s (%s)", name, ToolCategory(name))
		}
	}
	reads := 0
	for _, c := range toolCategories {
		if c == CategoryRead {
			reads++
		}
	}
	if len(names) != reads {
		t.Errorf("a read-only caller sees %d tools, want every read tool (%d)", len(names), reads)
	}
	for _, name := range []string{"send_text_message", "delete_message", "leave_group", "purge_media", "organise_chat"} {
		out := string(s.Handle(ctx, []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"`+name+`","arguments":{}}}`)))
		if !strings.Contains(out, "not allowed for this connection") || !strings.Contains(out, `"isError":true`) {
			t.Errorf("%s was not refused: %s", name, out)
		}
	}
	// An unknown tool is still answered as unknown.
	if out := string(s.Handle(ctx, []byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"nope","arguments":{}}}`))); !strings.Contains(out, "unknown tool") {
		t.Errorf("an unknown tool: %s", out)
	}
}

// A restricted list leaves the next caller's list whole.
func TestARestrictedListLeavesTheFullOne(t *testing.T) {
	s := New(Config{})
	ro := caller.With(context.Background(), caller.Caller{Kind: caller.APIKey, Categories: caller.ReadOnlyCategories()})
	_ = listedTools(t, s, ro)
	if got, want := len(listedTools(t, s, context.Background())), len(toolDefinitions()); got != want {
		t.Fatalf("the local list has %d tools after a read-only one, want %d", got, want)
	}
}

type noTools struct{}

func (noTools) AllowTool(string, string) bool { return false }

// A caller whose rules allow nothing sees no tool and calls none.
func TestRulesThatAllowNothing(t *testing.T) {
	s := New(Config{})
	ctx := caller.With(context.Background(), caller.Caller{Kind: caller.OAuth, Rules: noTools{}})
	if names := listedTools(t, s, ctx); len(names) != 0 {
		t.Fatalf("sees %v", names)
	}
	for name := range s.handlers() {
		out := string(s.Handle(ctx, []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"`+name+`","arguments":{}}}`)))
		if !strings.Contains(out, "not allowed for this connection") {
			t.Errorf("%s was not refused: %.200s", name, out)
		}
	}
}
