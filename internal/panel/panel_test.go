package panel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/mcp"
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
// sending gets ready: Estado says so, with no alert and no warning box.
func TestEstadoWhileSendingGetsReady(t *testing.T) {
	s := snapshot{Sync: wacli.Status{State: "connected"}, Health: mcp.Health{Status: "warn", Checks: []mcp.Check{
		{Name: "sync", Status: "warn"}, {Name: "receiving", Status: "ok"},
	}}}
	s.SyncTone, s.SyncLabel = syncLabel(s.Sync)
	s.readChecks()
	var b strings.Builder
	l := layout{Title: "Estado", Active: "estado", HealthTone: s.Tone, LiveKey: syncKey(s.Sync), LiveBusy: settling(s.Sync)}
	if err := parseTemplates().ExecuteTemplate(&b, "estado", struct {
		layout
		snapshot
	}{l, s}); err != nil {
		t.Fatal(err)
	}
	page := b.String()
	for _, want := range []string{"Nenhum problema detectado", "O envio de mensagens fica pronto em instantes", "pill--busy", `data-live="connected//false"`, "data-live-busy"} {
		if !strings.Contains(page, want) {
			t.Errorf("Estado lacks %q", want)
		}
	}
	for _, unwanted := range []string{`class="nav__alert`, `class="problems__warn"`} {
		if strings.Contains(page, unwanted) {
			t.Errorf("Estado shows %q", unwanted)
		}
	}
}
