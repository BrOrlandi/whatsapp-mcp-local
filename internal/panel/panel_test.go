package panel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/localasr"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/mcp"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/wacli"
)

func TestSameOrigin(t *testing.T) {
	cases := []struct {
		name    string
		method  string
		headers map[string]string
		want    bool
	}{
		{"page fetch from itself", "POST", map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://127.0.0.1:47821"}, true},
		{"form post from itself", "POST", map[string]string{"Origin": "http://127.0.0.1:47821"}, true},
		{"another site", "POST", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}, false},
		{"another localhost app", "POST", map[string]string{"Sec-Fetch-Site": "same-site", "Origin": "http://localhost:3000"}, false},
		{"a post that names no origin", "POST", nil, false},
		{"typed into the address bar", "GET", map[string]string{"Sec-Fetch-Site": "none"}, true},
		{"an api read from another site", "GET", map[string]string{"Sec-Fetch-Site": "cross-site"}, false},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, "http://127.0.0.1:47821/api/pair", nil)
		for k, v := range c.headers {
			r.Header.Set(k, v)
		}
		if got := sameOrigin(r); got != c.want {
			t.Errorf("%s: sameOrigin = %v, want %v", c.name, got, c.want)
		}
	}
}

// The app's window reaches the panel in memory: its webview sends neither
// Sec-Fetch-Site nor, on a fetch, Origin, and nothing else can take that path.
func TestInternalRequestsAreTheAppsOwn(t *testing.T) {
	var got bool
	h := Internal(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = sameOrigin(r) }))
	r := httptest.NewRequest("POST", "wails://localhost/api/pair", nil)
	r.Header.Set("Referer", "wails://localhost/")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if !got {
		t.Fatal("a request from the app's own window must be accepted")
	}
	if sameOrigin(r) {
		t.Fatal("the same request over the network, unmarked, must be refused")
	}
}

// The app's webview does not follow redirects from the in-memory handler: a
// redirect becomes a page that navigates.
func TestInternalRedirectsNavigate(t *testing.T) {
	h := Internal(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/instalacao?erro=a&b", http.StatusSeeOther)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	body := rec.Body.String()
	if rec.Code != 200 || rec.Header().Get("Location") != "" || !strings.Contains(body, `url=/instalacao?erro=a&amp;b`) || strings.Contains(body, "See Other") {
		t.Fatalf("redirect answered %d %q: %s", rec.Code, rec.Header().Get("Location"), body)
	}
}

func TestFormatPhone(t *testing.T) {
	for in, want := range map[string]string{
		"5511987654321": "+55 (11) 98765-4321",
		"551132654321":  "+55 (11) 3265-4321",
		"4915112345678": "+4915112345678",
		"":              "",
	} {
		if got := formatPhone(in); got != want {
			t.Errorf("formatPhone(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTemplatesParse(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("templates do not parse: %v", r)
		}
	}()
	parseTemplates()
}

func TestParseMCPGet(t *testing.T) {
	out := "whatsapp:\n  Scope: User config (available in all your projects)\n  Status: ✔ Connected\n  Type: http\n  URL: https://example.test/mcp\n  Headers:\n    Authorization: Bearer x\n"
	if got := parseMCPGet(out); got != "https://example.test/mcp" {
		t.Fatalf("parseMCPGet = %q", got)
	}
	if got := parseMCPGet("whatsapp:\n  Type: stdio\n  Command: /bin/x\n"); got != "/bin/x" {
		t.Fatalf("parseMCPGet stdio = %q", got)
	}
}

// A connection on its way up is no alert; one that keeps failing still is.
func TestToneLeavesOutSettling(t *testing.T) {
	warnSync := []mcp.Check{{Name: "sync", Status: "warn"}, {Name: "receiving", Status: "ok"}}
	for _, c := range []struct {
		name   string
		checks []mcp.Check
		sync   wacli.Status
		want   string
	}{
		{"sending not ready yet", warnSync, wacli.Status{State: "connected"}, "ok"},
		{"starting", warnSync, wacli.Status{State: "starting"}, "ok"},
		{"reconnecting", warnSync, wacli.Status{State: "reconnecting"}, "warn"},
		{"starting for too long", []mcp.Check{{Name: "sync", Status: "fail"}}, wacli.Status{State: "starting"}, "fail"},
		{"another warning", []mcp.Check{{Name: "sync", Status: "warn"}, {Name: "coverage", Status: "warn"}}, wacli.Status{State: "connected"}, "warn"},
		{"all ready", []mcp.Check{{Name: "sync", Status: "ok"}}, wacli.Status{State: "connected", Delegate: true}, "ok"},
	} {
		if got := tone(c.checks, c.sync); got != c.want {
			t.Errorf("%s: tone = %q, want %q", c.name, got, c.want)
		}
	}
}

// Right after the app opens, WhatsApp is connected and receiving while
// sending gets ready: Status says so, with no alert and no warning box.
func TestStatusWhileSendingGetsReady(t *testing.T) {
	s := snapshot{Sync: wacli.Status{State: "connected"}, Health: mcp.Health{Status: "warn", Checks: []mcp.Check{
		{Name: "sync", Status: "warn"}, {Name: "receiving", Status: "ok"},
	}}}
	s.SyncTone, s.SyncLabel = syncLabel(s.Sync)
	s.readChecks()
	var b strings.Builder
	l := layout{Title: "Status", Active: "status", HealthTone: s.Tone, LiveKey: syncKey(s.Sync), LiveBusy: settling(s.Sync)}
	if err := parseTemplates().ExecuteTemplate(&b, "status", struct {
		layout
		snapshot
	}{l, s}); err != nil {
		t.Fatal(err)
	}
	page := b.String()
	for _, want := range []string{"Nenhum problema detectado", "O envio de mensagens fica pronto em instantes", "pill--busy", `data-live="connected//false"`, "data-live-busy"} {
		if !strings.Contains(page, want) {
			t.Errorf("Status lacks %q", want)
		}
	}
	for _, unwanted := range []string{`class="nav__alert`, `class="problems__warn"`} {
		if strings.Contains(page, unwanted) {
			t.Errorf("Status shows %q", unwanted)
		}
	}
}

// On the command line there is no app around the panel: Configurações keeps
// the appearance and audio transcription, and none of the app's settings.
func TestSettingsOnTheCommandLine(t *testing.T) {
	var b strings.Builder
	if err := parseTemplates().ExecuteTemplate(&b, "configuracoes", struct {
		layout
		Setup       setup
		Port        int
		PortChanged bool
		Local       localasr.Status
		Total       int
		Corrected   int
	}{layout: layout{Title: "Configurações", Active: "configuracoes"}, PortChanged: true}); err != nil {
		t.Fatal(err)
	}
	page := b.String()
	for _, want := range []string{"Aparência", `data-theme-choice`, `id="transcricao"`, "Transcrição de áudio", `class="gear" href="/configuracoes" aria-label="Configurações" title="Configurações" aria-current="page"`} {
		if !strings.Contains(page, want) {
			t.Errorf("Configurações lacks %q", want)
		}
	}
	for _, unwanted := range []string{"Porta do MCP", "Ao ligar e ao fechar", "Versão e atualizações", "Apagar tudo", "Avise as suas ferramentas"} {
		if strings.Contains(page, unwanted) {
			t.Errorf("Configurações on the command line shows %q", unwanted)
		}
	}
}

// Each tool in the connect flow shows where it stands, from Suas conexões.
func TestToolState(t *testing.T) {
	conns := []Connection{
		{Key: "claude-code", Configured: true, Live: true},
		{Key: "codex", Configured: true},
		{Key: "client:windsurf", Live: true},
	}
	want := map[string]string{"claude-desktop": "", "claude-code": "live", "codex": "configured", "cursor": "", "outra": "live"}
	for _, c := range toolChoices(conns) {
		if c.State != want[c.Key] {
			t.Errorf("%s is %q, want %q", c.Key, c.State, want[c.Key])
		}
	}
}

func TestParseCodexGet(t *testing.T) {
	for out, want := range map[string]string{
		`{"name":"whatsapp","transport":{"type":"streamable_http","url":"http://127.0.0.1:47821/mcp"}}`: "http://127.0.0.1:47821/mcp",
		`{"name":"whatsapp","transport":{"type":"stdio","command":"npx","args":["-y","whatsapp-mcp"]}}`: "npx -y whatsapp-mcp",
		`not json`: "",
	} {
		if got := parseCodexGet([]byte(out)); got != want {
			t.Errorf("parseCodexGet(%s) = %q, want %q", out, got, want)
		}
	}
}

// The first page shows its examples for the first three days of use, counted
// from the first AI tool that connected.
func TestNewcomer(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := &Panel{State: st}
	ctx := context.Background()
	now := time.Now()
	if p.newcomer(ctx, now) {
		t.Error("a newcomer before any tool connected")
	}
	if err := st.SeeClient(ctx, "claude-ai", "1", now.Add(-50*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !p.newcomer(ctx, now) {
		t.Error("not a newcomer two days in")
	}
	if p.newcomer(ctx, now.Add(30*time.Hour)) {
		t.Error("still a newcomer after three days")
	}
}

// A tool is matched by how its name starts: Cursor adds the editor's.
func TestConnectionsByPrefix(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	_ = st.SeeClient(ctx, "cursor-vscode", "1.0", time.Now())
	_ = st.SeeClient(ctx, "codex-mcp-client", "0.160.0", time.Now())
	_ = st.SeeClient(ctx, "windsurf", "2", time.Now())
	p := &Panel{State: st}
	keys := map[string]bool{}
	for _, c := range p.connections(ctx, setup{}) {
		keys[c.Key] = c.Live
	}
	for _, k := range []string{"cursor", "codex", "client:windsurf"} {
		if !keys[k] {
			t.Errorf("%s is not a live connection: %v", k, keys)
		}
	}
}

// Another tool's connection takes the name the person gives it, and an empty
// name brings back the tool's own. The known tools show their logo instead.
func TestRenameConnection(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	_ = st.SeeClient(ctx, "windsurf", "2", time.Now())
	_ = st.SeeClient(ctx, "claude-code", "2.1", time.Now())
	p := &Panel{State: st}
	rename := func(client, name string) {
		t.Helper()
		r := httptest.NewRequest("POST", "/conexoes/renomear", strings.NewReader("client="+client+"&name="+name))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if _, err := p.renameConnection(r); err != nil {
			t.Fatal(err)
		}
	}
	tools := func() map[string]Connection {
		out := map[string]Connection{}
		for _, c := range p.connections(ctx, setup{}) {
			out[c.Key] = c
		}
		return out
	}
	rename("client:windsurf", "Meu+editor")
	got := tools()
	if got["client:windsurf"].Tool != "Meu editor" || got["client:windsurf"].Mark != "" {
		t.Errorf("renamed: %+v", got["client:windsurf"])
	}
	if got["claude-code"].Mark != "claude-code" {
		t.Errorf("Claude Code has no logo: %+v", got["claude-code"])
	}
	rename("client:windsurf", "")
	if tool := tools()["client:windsurf"].Tool; tool != "Windsurf" {
		t.Errorf("the name did not come back: %q", tool)
	}
}
