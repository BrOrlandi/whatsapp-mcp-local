package public

import (
	"context"
	"html"
	"strings"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/account"
)

// Hosts show in their ASCII form: a look-alike written in another alphabet
// shows as the xn-- name it is.
func TestHostsShowInASCII(t *testing.T) {
	for in, want := range map[string]string{
		"münchen.example":  "xn--mnchen-3ya.example",
		"bücher.example":   "xn--bcher-kva.example",
		"аррӏе.example":    "xn--80ak6aa92e.example",
		"Claude.AI":        "claude.ai",
		"bücher.example:8": "xn--bcher-kva.example:8",
		"127.0.0.1":        "127.0.0.1",
	} {
		if got := asciiHost(in); got != want {
			t.Errorf("asciiHost(%q) = %q, want %q", in, got, want)
		}
	}
}

// The consent page shows a registered app's name without the invisible
// characters that reorder or hide text, capped, and the host it goes back
// to in ASCII.
func TestTheConsentPageShowsWhatIsThere(t *testing.T) {
	s, _, _ := agentServer(t)
	name := "Cla\u200bude \u202eelgoog\u202c" + strings.Repeat("x", 100)
	rec := serve(s, fromTunnel("POST", "/oauth/register", map[string]string{"Content-Type": "application/json"},
		`{"client_name":"`+name+`","redirect_uris":["https://аррӏе.example/cb"],"token_endpoint_auth_method":"none"}`))
	if rec.Code != 201 {
		t.Fatalf("registration: %d %s", rec.Code, rec.Body)
	}
	var reg struct {
		ClientID   string `json:"client_id"`
		ClientName string `json:"client_name"`
	}
	jsonBody(t, rec, &reg)
	if strings.ContainsAny(reg.ClientName, "\u200b\u202e\u202c") || len([]rune(reg.ClientName)) > 60 || !strings.HasPrefix(reg.ClientName, "Claude elgoog") {
		t.Fatalf("the name kept: %q", reg.ClientName)
	}
	c := oauthClient{t: t, s: s, id: reg.ClientID, redirect: "https://аррӏе.example/cb"}
	_, challenge := pkce("n")
	page := serve(s, fromTunnel("GET", "/oauth/authorize?"+c.authorizeQuery(challenge, c.redirect).Encode(), nil, "")).Body.String()
	// The form keeps the redirect as sent; what the page shows is ASCII.
	if !strings.Contains(page, "xn--80ak6aa92e.example quer usar") || !strings.Contains(page, "volta para: xn--80ak6aa92e.example") ||
		strings.Contains(page, "аррӏе.example quer") || strings.ContainsAny(page, "\u200b\u202e") {
		t.Fatalf("the consent page:\n%s", page)
	}
}

// The consent page says which number the app will reach, masked: anyone who
// registers an app can open it, so the whole number is never in the page.
func TestTheConsentPageShowsTheNumberMasked(t *testing.T) {
	const phone = "5516999996892"
	s, _, _ := agentServer(t)
	c := registerClient(t, s, "https://claude.example/cb")
	_, challenge := pkce("m")
	open := func() string {
		return html.UnescapeString(serve(s, fromTunnel("GET", "/oauth/authorize?"+c.authorizeQuery(challenge, c.redirect).Encode(), nil, "")).Body.String())
	}
	if page := open(); !strings.Contains(page, "Nenhum número conectado ainda") {
		t.Fatalf("with no number paired, the page does not say so:\n%s", page)
	}
	s.UsePhone(func(context.Context) string { return phone })
	page := open()
	if !strings.Contains(page, "+55 (16) 9••••-6892") {
		t.Fatalf("the masked number is not on the page:\n%s", page)
	}
	for _, whole := range []string{phone, "99999-6892", "999996892", account.FormatPhone(phone)} {
		if strings.Contains(page, whole) {
			t.Errorf("the page has %q", whole)
		}
	}
	if strings.Contains(page, "Nenhum número conectado ainda") {
		t.Error("the page says no number is connected")
	}
}
