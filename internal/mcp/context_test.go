package mcp

import (
	"strings"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/index"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/localasr"
)

// What reaches the engine at each context level: nothing, the names only,
// or the names and the latest messages (1.9.5's prompt).
func TestEnginePromptPerLevel(t *testing.T) {
	c := index.AudioContext{ChatName: "Família", Speaker: "Mãe", People: []string{"Mãe"},
		Before: []index.ContextLine{{Sender: "Mãe", Text: "Vamos almoçar no domingo?"}}}
	if p := enginePrompt(c, localasr.ContextNone); p != "" {
		t.Errorf("none sent %q", p)
	}
	if p := enginePrompt(c, ""); p != "" {
		t.Errorf("an empty level sent %q", p)
	}
	if p := enginePrompt(c, localasr.ContextNames); p != "Família, Mãe." || strings.Contains(p, "almoçar") {
		t.Errorf("names sent %q", p)
	}
	if p := enginePrompt(c, localasr.ContextMessages); p != c.Prompt() || !strings.Contains(p, "almoçar") || !strings.HasPrefix(p, "Família") {
		t.Errorf("messages sent %q", p)
	}
	for _, level := range localasr.ContextLevels {
		if s := contextSent(level); !strings.HasPrefix(s, map[string]string{localasr.ContextNone: "none", localasr.ContextNames: "names only", localasr.ContextMessages: "names and recent messages"}[level]) {
			t.Errorf("contextSent(%q) = %q", level, s)
		}
	}
}
