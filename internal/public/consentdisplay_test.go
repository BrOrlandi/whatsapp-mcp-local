package public

import (
	"strings"
	"testing"
)

// Hosts show in their ASCII form: a look-alike written in another alphabet
// shows as the xn-- name it is.
func TestHostsShowInASCII(t *testing.T) {
	for in, want := range map[string]string{
		"münchen.example":  "xn--mnchen-3ya.example",
		"bücher.example":   "xn--bcher-kva.example",
		"аррӏе.com":        "xn--80ak6aa92e.com",
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
		`{"client_name":"`+name+`","redirect_uris":["https://аррӏе.com/cb"],"token_endpoint_auth_method":"none"}`))
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
	c := oauthClient{t: t, s: s, id: reg.ClientID, redirect: "https://аррӏе.com/cb"}
	_, challenge := pkce("n")
	page := serve(s, fromTunnel("GET", "/oauth/authorize?"+c.authorizeQuery(challenge, c.redirect).Encode(), nil, "")).Body.String()
	// The form keeps the redirect as sent; what the page shows is ASCII.
	if !strings.Contains(page, "xn--80ak6aa92e.com quer usar") || !strings.Contains(page, "volta para: xn--80ak6aa92e.com") ||
		strings.Contains(page, "аррӏе.com quer") || strings.ContainsAny(page, "\u200b\u202e") {
		t.Fatalf("the consent page:\n%s", page)
	}
}
