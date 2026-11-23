package panel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/appconfig"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/localasr"
)

// On the command line there is no app to keep the language: the panel keeps
// it in config.json, next to the port, and shows it at once.
func TestLanguageOnTheCommandLine(t *testing.T) {
	defer i18n.Set(i18n.Source)
	dir := t.TempDir()
	if err := appconfig.Save(dir, appconfig.Config{Port: 47999}); err != nil {
		t.Fatal(err)
	}
	p := &Panel{DataDir: dir}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/idioma", p.api(p.apiLanguage))
	post := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://127.0.0.1:47821/api/idioma", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		r.Header.Set("Origin", "http://127.0.0.1:47821")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}

	if w := post(`{"language":"es"}`); w.Code != http.StatusOK {
		t.Fatalf("choosing Spanish: %d %s", w.Code, w.Body)
	}
	if i18n.Lang() != "es" {
		t.Errorf("the panel shows %q, want es", i18n.Lang())
	}
	c, err := appconfig.Load(dir)
	if err != nil || c.Language != "es" || c.Port != 47999 {
		t.Errorf("config.json = %+v, %v; want the language kept and the port untouched", c, err)
	}
	if got := p.chosenLanguage(); got != "es" {
		t.Errorf("chosenLanguage = %q, want es", got)
	}

	if w := post(`{"language":"fr"}`); w.Code != http.StatusConflict {
		t.Errorf("an unknown language: %d, want 409", w.Code)
	}
	if w := post(`{}`); w.Code != http.StatusConflict {
		t.Errorf("no language: %d, want 409", w.Code)
	}
	if c, _ := appconfig.Load(dir); c.Language != "es" {
		t.Errorf("a refused request changed the language to %q", c.Language)
	}

	if w := post(`{"language":""}`); w.Code != http.StatusOK {
		t.Fatalf("following the system: %d %s", w.Code, w.Body)
	}
	if c, _ := appconfig.Load(dir); c.Language != "" {
		t.Errorf("following the system kept %q", c.Language)
	}
}

// "O do servidor" goes back to the language the command line started with:
// WHATSAPP_MCP_LANG, which a server in Docker without LANG relies on, not the
// system's.
func TestNoChoiceFollowsTheStartLanguage(t *testing.T) {
	defer i18n.Set(i18n.Source)
	for _, v := range []string{"LC_ALL", "LC_MESSAGES"} {
		t.Setenv(v, "")
	}
	t.Setenv("LANG", "en_US.UTF-8")
	p := &Panel{DataDir: t.TempDir()}

	t.Setenv("WHATSAPP_MCP_LANG", "pt-BR")
	i18n.Set("es")
	if err := p.setLanguage(""); err != nil {
		t.Fatal(err)
	}
	if i18n.Lang() != "pt-BR" {
		t.Errorf("with WHATSAPP_MCP_LANG=pt-BR, no choice shows %q", i18n.Lang())
	}

	t.Setenv("WHATSAPP_MCP_LANG", "")
	if err := p.setLanguage(""); err != nil {
		t.Fatal(err)
	}
	if i18n.Lang() != "en" {
		t.Errorf("without WHATSAPP_MCP_LANG, no choice shows %q, want the system's (en)", i18n.Lang())
	}
}

// The first screen offers every language, each named in itself and, on hover,
// in the panel's language, with the one shown pressed; Configurações offers
// them on the command line too.
func TestLanguagePickers(t *testing.T) {
	defer i18n.Set(i18n.Source)
	i18n.Set("en")
	var b strings.Builder
	if err := parseTemplates().ExecuteTemplate(&b, "instalacao", struct {
		layout
		snapshot
		Step         int
		Steps        []wizardStep
		Setup        setup
		Connections  []Connection
		LiveCount    int
		Verification string
		Tools        []toolChoice
	}{Step: 1, Steps: steps(1)}); err != nil {
		t.Fatal(err)
	}
	page := b.String()
	for _, want := range []string{
		`<html lang="en"`,
		`lang="pt-BR" data-language-pick="pt-BR" aria-pressed="false"`, `aria-hidden="true" lang="en">Brazilian Portuguese</span>`,
		`lang="es" data-language-pick="es" aria-pressed="false"`, `aria-hidden="true" lang="en">Spanish</span>`,
		`lang="en" data-language-pick="en" aria-pressed="true"`, `aria-hidden="true" lang="en">English</span>`,
		`<span>Português</span>`, `<span>Español</span>`, `<span>English</span>`,
		`<svg class="flag"`,
		"Connect your WhatsApp",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the first screen lacks %q", want)
		}
	}

	b.Reset()
	if err := parseTemplates().ExecuteTemplate(&b, "configuracoes", struct {
		layout
		Setup       setup
		Port        int
		PortChanged bool
		Local       localasr.Status
		ASR         transcriptionView
		Total       int
		Corrected   int
	}{layout: layout{Title: "Configurações", Active: "configuracoes", Language: "es"}}); err != nil {
		t.Fatal(err)
	}
	page = b.String()
	for _, want := range []string{`id="idioma"`, `value="es" data-language checked`, `value="" data-language>`, `aria-hidden="true" lang="en">Spanish</span>`} {
		if !strings.Contains(page, want) {
			t.Errorf("Configurações on the command line lacks %q", want)
		}
	}
}

// Sentences built from counts and lists read right in every language: one
// takes the singular, a list ends with the language's "and", and a date in
// English names its month.
func TestCountsListsAndDates(t *testing.T) {
	defer i18n.Set(i18n.Source)
	tools := []Connection{{Tool: "Claude Code", Live: true}, {Tool: "Codex"}, {Tool: "Cursor", Live: true}, {Tool: "VS Code", Live: true}}
	at := time.Date(2026, 5, 10, 15, 4, 0, 0, time.Local)
	for lang, want := range map[string][]string{
		i18n.Source: {"1 mensagem nova", "1.234 mensagens novas", "1 conversa", "<strong>Claude Code</strong>, <strong>Cursor</strong> e <strong>VS Code</strong>", "10/05/2026 15:04"},
		"es":        {"1 mensaje nuevo", "1.234 mensajes nuevos", "1 conversación", "<strong>Claude Code</strong>, <strong>Cursor</strong> y <strong>VS Code</strong>", "10/05/2026 15:04"},
		"en":        {"1 new message", "1,234 new messages", "1 chat", "<strong>Claude Code</strong>, <strong>Cursor</strong> and <strong>VS Code</strong>", "May 10, 2026, 3:04 PM"},
	} {
		i18n.Set(lang)
		got := []string{newMessages(1), newMessages(1234), chatCount(1), string(liveTools(tools)), momentAny(at)}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: got %q, want %q", lang, got[i], want[i])
			}
		}
	}
	if got := string(liveTools(tools[:1])); got != "<strong>Claude Code</strong>" {
		t.Errorf("one tool: %q", got)
	}
}

// A language change does not blink: the page's scripts come back as "not
// modified", Aparência is shown by a mark theme.js sets before the first
// paint rather than unhidden later, the cross-fade waits on reduced motion,
// and a browser without scripts still changes the language with a plain form.
func TestLanguageChangeKeepsThePageStill(t *testing.T) {
	defer i18n.Set(i18n.Source)

	mux := http.NewServeMux()
	registerAssets(mux)
	get := func(etag string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://127.0.0.1:47821/assets/app.js", nil)
		if etag != "" {
			r.Header.Set("If-None-Match", etag)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	first := get("")
	tag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || tag == "" || first.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("app.js: %d, ETag %q, Cache-Control %q; want 200, a tag and no-cache", first.Code, tag, first.Header().Get("Cache-Control"))
	}
	if again := get(tag); again.Code != http.StatusNotModified {
		t.Errorf("app.js asked again with its tag: %d, want 304", again.Code)
	}

	if !strings.Contains(stylesheet, "@media (prefers-reduced-motion:no-preference){@view-transition{navigation:auto}") {
		t.Error("the stylesheet lacks the cross-fade, or does not keep it from reduced motion")
	}
	if !strings.Contains(stylesheet, "[data-themes] [data-theme-switch]{display:block}") {
		t.Error("the stylesheet does not show Aparência by theme.js's mark")
	}
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
	}{layout: layout{Title: "Configurações", Active: "configuracoes"}}); err != nil {
		t.Fatal(err)
	}
	page := b.String()
	if !strings.Contains(page, `id="aparencia" data-theme-switch>`) {
		t.Error("Aparência is still hidden in the markup, to be shown late")
	}
	if !strings.Contains(page, `<form class="stack" method="post" action="/configuracoes/idioma">`) || !strings.Contains(page, `<noscript>`) {
		t.Error("the Idioma card has no form for a browser without scripts")
	}

	dir := t.TempDir()
	if err := appconfig.Save(dir, appconfig.Config{Port: 47999}); err != nil {
		t.Fatal(err)
	}
	p := &Panel{DataDir: dir}
	forms := http.NewServeMux()
	forms.HandleFunc("POST /configuracoes/idioma", p.form(p.languageForm))
	post := func(value string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://127.0.0.1:47821/configuracoes/idioma", strings.NewReader("idioma="+value))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		r.Header.Set("Origin", "http://127.0.0.1:47821")
		w := httptest.NewRecorder()
		forms.ServeHTTP(w, r)
		return w
	}
	if w := post("en"); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/configuracoes#idioma" {
		t.Errorf("the form: %d to %q, want 303 to /configuracoes#idioma", w.Code, w.Header().Get("Location"))
	}
	if i18n.Lang() != "en" || p.chosenLanguage() != "en" {
		t.Errorf("after the form the panel shows %q and keeps %q, want en", i18n.Lang(), p.chosenLanguage())
	}
	if w := post("fr"); !strings.HasPrefix(w.Header().Get("Location"), "/configuracoes?erro=") || p.chosenLanguage() != "en" {
		t.Errorf("an unknown language: to %q, keeping %q; want an error and en kept", w.Header().Get("Location"), p.chosenLanguage())
	}
}
