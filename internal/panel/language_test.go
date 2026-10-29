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
		Total       int
		Corrected   int
		Devices     []remoteDevice
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
