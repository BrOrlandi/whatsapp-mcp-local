package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/account"
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

// Marking unread is a change of display, so it is not filed with the sends.
func TestMarkChatUnreadIsOrganise(t *testing.T) {
	if got := ToolCategory("mark_chat_unread"); got != CategoryOrganise {
		t.Errorf("mark_chat_unread is %q, want %q", got, CategoryOrganise)
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

// The number's label becomes the connection's title; the instructions name
// the number only when the app has more than one.
func TestInitializeNamesTheNumber(t *testing.T) {
	count := 1
	s := &Server{baseURL: "http://127.0.0.1:47821", clients: map[string]string{}, touched: map[string]time.Time{},
		account: func(context.Context) account.Info {
			return account.Info{Label: "Trabalho", Phone: "+55 (11) 98765-4321"}
		},
		accounts: func() int { return count }}
	initialize := func() string {
		return string(s.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)))
	}
	one := initialize()
	if !strings.Contains(one, `"title":"WhatsApp Trabalho"`) {
		t.Errorf("no title from the label: %s", one)
	}
	if strings.Contains(one, "This connection is the WhatsApp number") {
		t.Errorf("one number, yet the instructions name it: %s", one)
	}
	count = 2
	if two := initialize(); !strings.Contains(two, `This connection is the WhatsApp number \"Trabalho\" (+55 (11) 98765-4321)`) {
		t.Errorf("two numbers, and the instructions do not say which: %s", two)
	}
}

func TestInitializeWithoutAnAccount(t *testing.T) {
	s := &Server{baseURL: "http://127.0.0.1:47821", clients: map[string]string{}, touched: map[string]time.Time{}}
	if out := string(s.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))); !strings.Contains(out, `"title":"WhatsApp"`) {
		t.Errorf("title without a label: %s", out)
	}
}
