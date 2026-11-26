package panel

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
)

// renderTool is a tool's connect page, as connectTool fills it.
func renderTool(t *testing.T, g guide, set setup, l layout) string {
	t.Helper()
	var b strings.Builder
	if err := parseTemplates().ExecuteTemplate(&b, "conectarferramenta", struct {
		layout
		Tool         guide
		Setup        setup
		Live         bool
		Done         bool
		InSetup      bool
		Known        string
		Since        int64
		Verification string
	}{l, g, set, false, false, false, "", 1, verificationPrompt}); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// Seen from outside the server, the tool is on the person's computer: every
// step uses the public address and a key from Conexões (the login, for
// Claude), never the server's loopback, its local token, its files or a
// button that would run a program on the server.
func TestConnectStepsSeenFromOutsideTheServer(t *testing.T) {
	// The container's own home and Codex folder, which must not show.
	t.Setenv("HOME", "/data")
	t.Setenv("CODEX_HOME", "/data/.codex")
	p := &Panel{PublicURL: "https://wa.example.com", Token: "local-token-secret",
		Endpoint: func() string { return "http://127.0.0.1:47821/mcp" }}
	ctx := caller.With(context.Background(), caller.Caller{Kind: caller.Panel, Origin: "https://wa.example.com"})
	set := p.setup(ctx)
	if !set.Remote || set.Endpoint != "https://wa.example.com/mcp" {
		t.Fatalf("setup seen from outside: remote %v, endpoint %q", set.Remote, set.Endpoint)
	}
	if set.Desktop.Found || set.Code.Found || set.Codex.Found || set.Cursor.Found {
		t.Error("seen from outside, the server's own programs and files must not be looked at")
	}
	l := layout{Active: "conectar", Remote: true, ServerMode: true, PublicMCP: "https://wa.example.com/mcp"}
	want := map[string][]string{
		"claude-desktop": {"Adicionar conector personalizado", "<code>https://wa.example.com/mcp</code>", "senha do painel", `data-wait-tool="claude-ai"`},
		"claude-code": {"claude mcp add --scope user --transport http whatsapp https://wa.example.com/mcp --header &#39;Authorization: Bearer SUA_CHAVE&#39;",
			`href="/conexoes#chave-dialog"`},
		"codex": {"~/.codex/config.toml", "url = &#34;https://wa.example.com/mcp&#34;",
			"http_headers = { &#34;Authorization&#34; = &#34;Bearer SUA_CHAVE&#34; }", `href="/conexoes#chave-dialog"`},
		"cursor": {"~/.cursor/mcp.json", "&#34;url&#34;: &#34;https://wa.example.com/mcp&#34;", "&#34;Authorization&#34;: &#34;Bearer SUA_CHAVE&#34;",
			`href="/conexoes#chave-dialog"`},
		"outra": {"URL: https://wa.example.com/mcp", "Authorization: Bearer SUA_CHAVE", `href="/conexoes#chave-dialog"`},
	}
	for _, g := range guides {
		page := renderTool(t, g, set, l)
		for _, w := range want[g.Key] {
			if !strings.Contains(page, w) {
				t.Errorf("%s seen from outside lacks %q", g.Key, w)
			}
		}
		for _, unwanted := range []string{"127.0.0.1", "/data", "local-token-secret", "data-add-client", "claude_desktop_config.json",
			"bearer_token_env_var", "não foi encontrado", "data-wait-help"} {
			if strings.Contains(page, unwanted) {
				t.Errorf("%s seen from outside shows %q", g.Key, unwanted)
			}
		}
	}
}

// On this computer (and on the server's own loopback) the steps stay as they
// were: this computer's address, its files and the one-click buttons.
func TestConnectStepsOnThisComputer(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CODEX_HOME", "")
	t.Setenv("PATH", "")
	for _, public := range []string{"", "https://wa.example.com"} {
		p := &Panel{PublicURL: public, Endpoint: func() string { return "http://127.0.0.1:47821/mcp" }}
		set := p.setup(context.Background())
		if set.Remote || set.Endpoint != "http://127.0.0.1:47821/mcp" || set.CodexPath != filepath.Join(home, ".codex", "config.toml") ||
			set.CursorPath != filepath.Join(home, ".cursor", "mcp.json") || set.Key != "" {
			t.Fatalf("local setup with public address %q changed: %+v", public, set)
		}
		if !strings.Contains(set.CodexTOML, `url = "http://127.0.0.1:47821/mcp"`) || !strings.Contains(set.Command, "http://127.0.0.1:47821/mcp") {
			t.Errorf("local snippets changed: %q / %q", set.CodexTOML, set.Command)
		}
		for _, g := range guides {
			page := renderTool(t, g, set, layout{Active: "conectar"})
			if strings.Contains(page, "chave-dialog") || strings.Contains(page, "SUA_CHAVE") || strings.Contains(page, "conector personalizado") {
				t.Errorf("%s on this computer shows the server's steps", g.Key)
			}
			if g.Key != "outra" && !strings.Contains(page, `data-add-client="`+g.Key+`"`) {
				t.Errorf("%s on this computer lost its button", g.Key)
			}
		}
	}
}
