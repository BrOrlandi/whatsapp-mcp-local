package panel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
// the appearance, the language and audio transcription, and none of the app's
// settings.
func TestSettingsOnTheCommandLine(t *testing.T) {
	var b strings.Builder
	if err := parseTemplates().ExecuteTemplate(&b, "configuracoes", struct {
		layout
		Setup       setup
		Port        int
		PortChanged bool
		Local       localasr.Status
		ASR         transcriptionView
		Total       int
		Corrected   int
	}{layout: layout{Title: "Configurações", Active: "configuracoes"}, PortChanged: true}); err != nil {
		t.Fatal(err)
	}
	page := b.String()
	for _, want := range []string{"Aparência", `data-theme-choice`, `id="idioma"`, `id="transcricao"`, "Transcrição de áudio", `value="openai" data-asr-mode`, `data-asr-fields="remote"`, `href="#asr-dialog" data-asr-open`, `id="asr-dialog" role="dialog"`, `class="gear" href="/configuracoes" aria-label="Configurações" title="Configurações" aria-current="page"`} {
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

// The Webhooks card adds and edits through a dialog, on the computer and on
// a server alike: the card keeps only the list and an add button.
func TestWebhooksCardOpensADialog(t *testing.T) {
	for _, l := range []layout{
		{Title: "Configurações", Active: "configuracoes"},
		{Title: "Configurações", Active: "configuracoes", ServerMode: true, Remote: true, Release: &Release{Version: "1.9.0", Container: true}},
	} {
		var b strings.Builder
		if err := parseTemplates().ExecuteTemplate(&b, "configuracoes", struct {
			layout
			Setup       setup
			Port        int
			PortChanged bool
			Local       localasr.Status
			ASR         transcriptionView
			Total       int
			Corrected   int
		}{layout: l}); err != nil {
			t.Fatal(err)
		}
		page := b.String()
		start := strings.Index(page, `id="webhooks"`)
		if start < 0 {
			t.Fatalf("server mode %v: no Webhooks card", l.ServerMode)
		}
		cardHTML := page[start : start+strings.Index(page[start:], "</section>")]
		// A new webhook's box: two numbered steps, the key and then the
		// documentation, opened beside it; "Finalizar", the box's main
		// button, closes it.
		secretHTML := cardHTML[strings.Index(cardHTML, "data-webhook-secret"):]
		order := []string{"Webhook adicionado", `<span class="step__n" aria-hidden="true">1</span>`, "Guarde a chave", "Ela não aparece de novo.",
			"data-webhook-secret-value", "Copiar a chave", `<span class="step__n" aria-hidden="true">2</span>`, "Veja a documentação",
			`href="/webhooks/documentacao" target="_blank"`, "Ver a documentação", `<button class="btn btn--small" type="button" data-webhook-secret-close>Finalizar</button>`}
		pos := 0
		for _, want := range order {
			i := strings.Index(secretHTML[pos:], want)
			if i < 0 {
				t.Errorf("server mode %v: the new webhook's box lacks %q, or has it out of order", l.ServerMode, want)
				continue
			}
			pos += i + len(want)
		}
		for _, want := range []string{`data-webhook-add`, `data-webhook-start`, "Adicionar webhook"} {
			if !strings.Contains(cardHTML, want) {
				t.Errorf("server mode %v: the Webhooks card lacks %q", l.ServerMode, want)
			}
		}
		if strings.Contains(cardHTML, "<form") || strings.Contains(cardHTML, "data-webhook-event") {
			t.Errorf("server mode %v: the Webhooks card still has the form inline", l.ServerMode)
		}
		at := strings.Index(page, `<div class="overlay" id="webhook-dialog" role="dialog" aria-modal="true" aria-labelledby="webhook-dialog-titulo" data-webhook-dialog>`)
		if at < 0 {
			t.Fatalf("server mode %v: no webhook dialog", l.ServerMode)
		}
		dialog := page[at : at+strings.Index(page[at:], "</div></div></div>")]
		for _, want := range []string{`<form class="stack" data-webhook-form>`, `data-webhook-url`, `data-webhook-own`, `data-webhook-error`, `data-webhook-submit`, `class="dialog__close" href="#webhooks"`,
			`role="group" aria-labelledby="webhook-eventos"`, `<span class="field__label" id="webhook-eventos">Eventos</span>`, "Avisar quando"} {
			if !strings.Contains(dialog, want) {
				t.Errorf("server mode %v: the webhook dialog lacks %q", l.ServerMode, want)
			}
		}
		if title, box := strings.Index(dialog, ">Eventos<"), strings.Index(dialog, "data-webhook-event"); title < 0 || box < title {
			t.Errorf("server mode %v: the Eventos title should come before the event choices", l.ServerMode)
		}
		if n := strings.Count(dialog, "data-webhook-event"); n != 4 {
			t.Errorf("server mode %v: the dialog offers %d events, want 4", l.ServerMode, n)
		}
		// Deleting lives in the dialog only, in red, hidden until a webhook
		// is being changed, and asks once more before it happens.
		for _, want := range []string{`<button class="btn btn--danger" type="button" data-webhook-delete hidden>`, "Apagar webhook",
			`data-webhook-delete-confirm hidden`, "Apagar este webhook?", "deixa de receber os avisos", `data-webhook-delete-cancel>Não apagar<`,
			`<button class="btn btn--danger" type="button" data-webhook-delete-yes>`, "Apagar mesmo"} {
			if !strings.Contains(dialog, want) {
				t.Errorf("server mode %v: the webhook dialog lacks %q", l.ServerMode, want)
			}
		}
		if strings.Contains(cardHTML, "data-webhook-delete") {
			t.Errorf("server mode %v: deleting a webhook should be in the dialog, not the card", l.ServerMode)
		}
	}
	// The rows: a switch for on and off, a quiet Testar and Editar; no
	// button to delete or to turn off.
	script, err := os.ReadFile(filepath.Join("assets", "settings.js"))
	if err != nil {
		t.Fatal(err)
	}
	js := string(script)
	for _, want := range []string{`setAttribute("role", "switch")`, `setAttribute("aria-checked"`, `t("Mandar avisos para %s", w.url)`,
		`button(t("Testar"), "btn--quiet`, `button(t("Editar"), "btn--ghost`} {
		if !strings.Contains(js, want) {
			t.Errorf("the webhook rows lack %q", want)
		}
	}
	for _, gone := range []string{`button(t("Apagar"))`, `t("Desligar")`, `t("Religar")`, `"DELETE", "/api/webhooks/" + w.id).then(function () { delete notes[w.id]; return load(); })`} {
		if strings.Contains(js, gone) {
			t.Errorf("the webhook rows still have %q", gone)
		}
	}
	if strings.Count(js, `"DELETE", "/api/webhooks/"`) != 1 || !strings.Contains(js, `delYes.addEventListener("click"`) {
		t.Error("a webhook should be deleted only from the dialog's confirmation")
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

// A failed pairing is worded for the person: wacli is an inner part, and its
// own text, with the command it suggests, never reaches the page.
func TestPairingErrorsNeverShowWacli(t *testing.T) {
	for raw, want := range map[string]string{
		"QR code timed out; run `wacli auth` again to get a new code": "O QR code expirou. Clique em Tentar de novo para gerar outro.",
		"wacli auth ended without pairing; try again":                 "Não foi possível conectar: a conexão terminou sem completar; tente de novo",
		"some new failure of wacli auth":                              "Não foi possível conectar agora. Clique em Tentar de novo.",
		"":                                                            "",
	} {
		got := pairingError(raw)
		if got != want {
			t.Errorf("pairingError(%q) = %q, want %q", raw, got, want)
		}
		if strings.Contains(got, "wacli") {
			t.Errorf("pairingError(%q) names wacli: %q", raw, got)
		}
	}
}

// On a server the page shows its version and its two addresses, and how to
// update; it never offers to install, change the port or erase the data.
func TestSettingsOnAServer(t *testing.T) {
	render := func(rel *Release, state string) string {
		var b strings.Builder
		l := layout{Title: "Configurações", Active: "configuracoes", ServerMode: true, Remote: true, Release: rel,
			PublicURL: "https://wa.example.com", InternalURL: "http://whatsapp-mcp:47822",
			Update: UpdateState{State: state, Latest: "1.9.1", Page: "https://example.com/release"}}
		l.Settings.Version = rel.Version
		if err := parseTemplates().ExecuteTemplate(&b, "configuracoes", struct {
			layout
			Setup       setup
			Port        int
			PortChanged bool
			Local       localasr.Status
			ASR         transcriptionView
			Total       int
			Corrected   int
		}{layout: l}); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	docker := render(&Release{Version: "1.9.0", Container: true}, "available")
	for _, want := range []string{"Versão e atualizações", "1.9.0", "docker compose pull &amp;&amp; docker compose up -d", "WHATSAPP_MCP_VERSION",
		"Endereços do servidor", "https://wa.example.com", "http://whatsapp-mcp:47822", `data-update-banner data-update-server`, `href="/configuracoes#atualizacoes"`} {
		if !strings.Contains(docker, want) {
			t.Errorf("Configurações on a Docker server lacks %q", want)
		}
	}
	for _, unwanted := range []string{"data-update-install", "Instalar e reiniciar", "Porta do MCP", "Ao ligar e ao fechar", "Apagar tudo", "whatsapp-mcp update"} {
		if strings.Contains(docker, unwanted) {
			t.Errorf("Configurações on a Docker server shows %q", unwanted)
		}
	}
	// A new version gets the green box, which holds its name, what changed
	// and the command; the line under the installed version stays out.
	box := updateBox(t, docker)
	if strings.HasPrefix(box, `data-update-found hidden`) {
		t.Error("with a new version out, the green box should show")
	}
	for _, want := range []string{"Uma nova versão foi encontrada", "WhatsApp MCP <span data-update-latest>1.9.1</span>", "Ver o que mudou",
		`href="https://whatsappmcp.com.br/changelog" target="_blank"`, "docker compose pull &amp;&amp; docker compose up -d", "WHATSAPP_MCP_VERSION"} {
		if !strings.Contains(box, want) {
			t.Errorf("the new version's box lacks %q", want)
		}
	}
	if strings.Contains(docker, "Você já tem a versão mais recente.") || !strings.Contains(docker, `data-update-status role="status" hidden`) {
		t.Error("with a new version out, the card should not say it is the latest")
	}
	if i, j := strings.Index(docker, "data-update-check"), strings.Index(docker, "Ver o que mudou"); j < i {
		t.Error("Ver o que mudou should sit in the box, below the button that looks")
	}

	cli := render(&Release{Version: "1.9.0"}, "current")
	if !strings.Contains(cli, "whatsapp-mcp update") || strings.Contains(cli, "docker compose") {
		t.Error("a server without Docker should name its own update command, and only that")
	}
	if !strings.HasPrefix(updateBox(t, cli), `data-update-found hidden`) {
		t.Error("the steps to update should stay hidden until a new version is out")
	}
	if !strings.Contains(cli, `<span data-update-status-text>Você já tem a versão mais recente.</span>`) || !strings.Contains(cli, "update-status--ok") {
		t.Error("with nothing new, the card should say it has the latest version")
	}
}

// updateBox returns the version card's green box, from its attribute to the
// end of the card.
func updateBox(t *testing.T, page string) string {
	t.Helper()
	i := strings.Index(page, "data-update-found")
	if i < 0 {
		t.Fatal("the version card has no box for a new version")
	}
	return page[i : i+strings.Index(page[i:], "</section>")]
}

// On the computer the card has the same parts: the box holds the version's
// name, what changed and the button that installs it.
func TestUpdateCardOnTheComputer(t *testing.T) {
	render := func(state string) string {
		var b strings.Builder
		l := layout{Title: "Configurações", Active: "configuracoes", App: true,
			Update: UpdateState{State: state, Current: "1.8.0", Latest: "1.9.1", Page: "https://example.com/release"}}
		l.Settings.Version = "1.8.0"
		if err := parseTemplates().ExecuteTemplate(&b, "configuracoes", struct {
			layout
			Setup       setup
			Port        int
			PortChanged bool
			Local       localasr.Status
			ASR         transcriptionView
			Total       int
			Corrected   int
		}{layout: l}); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	box := updateBox(t, render("available"))
	if strings.HasPrefix(box, `data-update-found hidden`) {
		t.Error("with a new version out, the green box should show")
	}
	for _, want := range []string{"Uma nova versão foi encontrada", "1.9.1", "Ver o que mudou", "Instalar e reiniciar"} {
		if !strings.Contains(box, want) {
			t.Errorf("the new version's box lacks %q", want)
		}
	}
	if strings.Contains(box, "data-update-how") {
		t.Error("the computer's box should not show a server's command")
	}
	current := render("current")
	if !strings.HasPrefix(updateBox(t, current), `data-update-found hidden`) || !strings.Contains(current, "Você já tem a versão mais recente.") {
		t.Error("with nothing new, the card should say so and hide the box")
	}
}

// The logs card opens a dialog whose form downloads the file without the
// script; with a support address it shows the two steps and the address,
// and without one only the download.
func TestLogsDialog(t *testing.T) {
	render := func(email string) string {
		var b strings.Builder
		if err := parseTemplates().ExecuteTemplate(&b, "configuracoes", struct {
			layout
			Setup       setup
			Port        int
			PortChanged bool
			Local       localasr.Status
			ASR         transcriptionView
			Total       int
			Corrected   int
		}{layout: layout{Title: "Configurações", Active: "configuracoes", SupportEmail: email}}); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	both := []string{
		`href="#logs-dialog" data-logs-open>Baixar os logs</a>`,
		`id="logs-dialog"`,
		`<form class="stack" method="get" action="/diagnostico/logs.txt" data-logs-form>`,
		`name="horas" value="24">`,
		`name="horas" value="72" checked>`,
		`name="horas" value="168">`,
		`type="submit" data-logs-submit>Baixar</button>`,
	}

	page := render("suporte@example.com")
	for _, want := range append(both,
		`<p class="step__title">Mande para o suporte</p>`,
		`href="mailto:suporte@example.com?subject=WhatsApp%20MCP%3a%20logs"`,
		`>suporte@example.com</a>`,
	) {
		if !strings.Contains(page, want) {
			t.Errorf("with an address, the logs dialog lacks %q", want)
		}
	}

	page = render("")
	for _, want := range both {
		if !strings.Contains(page, want) {
			t.Errorf("without an address, the logs dialog lacks %q", want)
		}
	}
	for _, unwanted := range []string{"mailto:", "Mande para o suporte"} {
		if strings.Contains(page, unwanted) {
			t.Errorf("without an address, the logs dialog has %q", unwanted)
		}
	}
	// The webhook key box uses the same steps, so look right after the form.
	if strings.Contains(page, `data-logs-form>`+"\n"+`<ol class="secret__steps">`) {
		t.Error("without an address, the logs dialog still numbers its steps")
	}
}

// The version in the footer and Ajuda open what changed in each version, on
// the site (one address for every version), never GitHub's releases.
func TestChangelogLinks(t *testing.T) {
	for _, server := range []bool{false, true} {
		var b strings.Builder
		if err := parseTemplates().ExecuteTemplate(&b, "ajuda", ajudaPage{layout: layout{Title: "Ajuda", Active: "ajuda", App: !server, ServerMode: server},
			ASR: ajudaASROf(localasr.Settings{Mode: localasr.ModeLocal}, localasr.Status{Supported: true, Ready: true}, server)}); err != nil {
			t.Fatal(err)
		}
		page := b.String()
		if !strings.Contains(page, `<a class="colophon__version" href="https://whatsappmcp.com.br/changelog" rel="noopener noreferrer" target="_blank"`) {
			t.Errorf("server=%v: the footer's version should link to the changelog", server)
		}
		if !strings.Contains(page, `href="https://whatsappmcp.com.br/changelog" target="_blank" rel="noopener noreferrer" data-changelog>Ver o que mudou em cada versão</a>`) {
			t.Errorf("server=%v: Ajuda should link to every version's changes", server)
		}
		if !strings.Contains(page, `<a class="btn btn--quiet btn--small" href="/configuracoes#asr-dialog">Trocar onde transcrever</a>`) {
			t.Errorf("server=%v: Ajuda's voice notes section should open where to transcribe", server)
		}
		for _, gone := range []string{"Outros lugares para transcrever", "Contexto da conversa"} {
			if strings.Contains(page, gone) {
				t.Errorf("server=%v: Ajuda should not explain %q, the settings do", server, gone)
			}
		}
		if strings.Contains(page, "github.com/BrOrlandi/whatsapp-mcp-local/releases") {
			t.Errorf("server=%v: the page should link to the site, not to GitHub's releases", server)
		}
	}
}

// Ajuda says only where voice notes are transcribed now, in one sentence,
// with the one button that changes it.
func TestAjudaSaysWhereVoiceNotesGo(t *testing.T) {
	local := localasr.Settings{Mode: localasr.ModeLocal}
	for _, c := range []struct {
		name   string
		s      localasr.Settings
		st     localasr.Status
		server bool
		line   string
		href   string
	}{
		{"here", local, localasr.Status{Supported: true, Ready: true}, false, "Os áudios são transcritos neste computador, sem sair dele.", "/configuracoes#asr-dialog"},
		{"server", local, localasr.Status{Supported: true, Ready: true}, true, "Os áudios são transcritos neste servidor, sem sair dele.", "/configuracoes#asr-dialog"},
		{"not installed", local, localasr.Status{Supported: true}, false, "A transcrição ainda não está instalada.", "/configuracoes#transcricao"},
		{"windows", local, localasr.Status{Supported: true, Unavailable: "windows-runtime"}, false, "A transcrição neste computador ainda não funciona no Windows. Escolha um servidor compatível com a OpenAI ou a OpenAI, ou peça à sua ferramenta de IA que transcreva os áudios do jeito dela.", "/configuracoes#asr-dialog"},
		{"remote", localasr.Settings{Mode: localasr.ModeRemote, URL: "https://transcricao.exemplo.com/v1"}, localasr.Status{}, false, "Os áudios vão para o servidor de transcrição em transcricao.exemplo.com.", "/configuracoes#asr-dialog"},
		{"openai", localasr.Settings{Mode: localasr.ModeOpenAI}, localasr.Status{}, false, "Os áudios vão para a OpenAI.", "/configuracoes#asr-dialog"},
		{"from env", localasr.Settings{Mode: localasr.ModeOpenAI, FromEnv: true}, localasr.Status{}, true, "Os áudios vão para a OpenAI.", "/configuracoes#transcricao"},
	} {
		v := ajudaASROf(c.s, c.st, c.server)
		if v.Line != c.line || v.Href != c.href || v.Action == "" {
			t.Errorf("%s: %+v, want %q at %s", c.name, v, c.line, c.href)
		}
	}
}
