package mcp

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Every tool listed has a handler and a category, and every handler is listed.
func TestToolsAreComplete(t *testing.T) {
	handlers := (&Server{}).handlers()
	listed := map[string]bool{}
	for _, d := range toolDefinitions() {
		def := d.(map[string]any)
		name := def["name"].(string)
		if listed[name] {
			t.Errorf("%s is listed twice", name)
		}
		listed[name] = true
		if ToolCategory(name) == "" {
			t.Errorf("%s has no category", name)
		}
		if _, ok := def["annotations"].(map[string]any); !ok {
			t.Errorf("%s has no annotations", name)
		}
		if _, ok := handlers[name]; !ok {
			t.Errorf("%s is listed but has no handler", name)
		}
	}
	for name := range handlers {
		if !listed[name] {
			t.Errorf("%s has a handler but is not listed", name)
		}
	}
	for name := range toolCategories {
		if !listed[name] {
			t.Errorf("%s has a category but is not listed", name)
		}
	}
}

// An assistant learns at initialize that webhooks exist, and where.
func TestInstructionsMentionWebhooks(t *testing.T) {
	s := &Server{baseURL: "http://127.0.0.1:47821", clients: map[string]string{}, touched: map[string]time.Time{}}
	out := string(s.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)))
	for _, want := range []string{"Configurações › Webhooks", "http://127.0.0.1:47821/webhooks/documentacao"} {
		if !strings.Contains(out, want) {
			t.Errorf("initialize lacks %q", want)
		}
	}
}
