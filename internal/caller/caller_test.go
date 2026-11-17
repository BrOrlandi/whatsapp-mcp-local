package caller

import (
	"context"
	"testing"
)

func TestAContextWithoutACallerIsLocal(t *testing.T) {
	if got := From(context.Background()); got.Kind != Local || got.ReadOnly() {
		t.Fatalf("got %+v", got)
	}
	c := Caller{Kind: OAuth, ConnectionID: "c1", Client: "ua:x"}
	if got := From(With(context.Background(), c)); got.Kind != OAuth || got.ConnectionID != "c1" || got.Client != "ua:x" {
		t.Fatalf("got %+v", got)
	}
}

func TestWhatEachCallerReaches(t *testing.T) {
	want := map[Kind]map[Access]bool{
		Local:  {Open: true, PanelAccess: true, MCP: true, Media: true, Health: true},
		Panel:  {Open: true, PanelAccess: true, MCP: false, Media: true, Health: true},
		OAuth:  {Open: true, PanelAccess: false, MCP: true, Media: true, Health: true},
		APIKey: {Open: true, PanelAccess: false, MCP: true, Media: true, Health: true},
		"":     {Open: true, PanelAccess: false, MCP: false, Media: false, Health: false},
	}
	for kind, accesses := range want {
		for a, ok := range accesses {
			if got := (Caller{Kind: kind}).May(a); got != ok {
				t.Errorf("%q on %q: got %v, want %v", kind, a, got, ok)
			}
		}
		if (Caller{Kind: kind}).May("unknown") {
			t.Errorf("%q reaches an unknown access", kind)
		}
	}
}

type onlyTool string

func (o onlyTool) AllowTool(name, _ string) bool { return name == string(o) }

func TestToolsByCategoryAndRules(t *testing.T) {
	local := Caller{Kind: Local}
	for _, cat := range []string{"read", "local", "send", "delete", "groups", "organise", ""} {
		if !local.MayUseTool("x", cat) {
			t.Errorf("local may not use a %q tool", cat)
		}
	}
	ro := Caller{Kind: APIKey, Categories: ReadOnlyCategories()}
	if !ro.ReadOnly() || !ro.MayUseTool("list_chats", "read") {
		t.Error("a read-only caller cannot read")
	}
	for _, cat := range []string{"local", "send", "delete", "groups", "organise"} {
		if ro.MayUseTool("x", cat) {
			t.Errorf("a read-only caller may use a %q tool", cat)
		}
	}
	if (Caller{Categories: []string{"read", "local"}}).ReadOnly() {
		t.Error("read and local records is not read-only")
	}
	ruled := Caller{Kind: OAuth, Rules: onlyTool("list_chats")}
	if !ruled.MayUseTool("list_chats", "read") || ruled.MayUseTool("search_messages", "read") {
		t.Error("the rules are not consulted")
	}
}
