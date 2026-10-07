package mcp

import "testing"

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
