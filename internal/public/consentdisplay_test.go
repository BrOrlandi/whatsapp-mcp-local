package public

import (
	"context"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	signedIn := map[string]string{"Cookie": CookieName + "=" + sessionCookie(t, signIn(s, "127.0.0.1:40000", testPassword, nil)).Value}
	page := serve(s, fromTunnel("GET", "/oauth/authorize?"+c.authorizeQuery(challenge, c.redirect).Encode(), signedIn, "")).Body.String()
	// The form keeps the redirect as sent; what the page shows is ASCII.
	if !strings.Contains(page, "xn--80ak6aa92e.example quer usar") || !strings.Contains(page, "volta para: xn--80ak6aa92e.example") ||
		strings.Contains(page, "аррӏе.example quer") || strings.ContainsAny(page, "\u200b\u202e") {
		t.Fatalf("the consent page:\n%s", page)
	}
}

// The consent page's second step says which number the app will reach,
// masked: the whole number is never in the page.
func TestTheConsentPageShowsTheNumberMasked(t *testing.T) {
	const phone = "5516999996892"
	s, _, _ := agentServer(t)
	c := registerClient(t, s, "https://claude.example/cb")
	_, challenge := pkce("m")
	signedIn := map[string]string{"Cookie": CookieName + "=" + sessionCookie(t, signIn(s, "127.0.0.1:40000", testPassword, nil)).Value}
	open := func() string {
		return html.UnescapeString(serve(s, fromTunnel("GET", "/oauth/authorize?"+c.authorizeQuery(challenge, c.redirect).Encode(), signedIn, "")).Body.String())
	}
	if page := open(); !strings.Contains(page, "Nenhum número conectado ainda") {
		t.Fatalf("with no number paired, the page does not say so:\n%s", page)
	}
	// Signed out, not even that: the first step says nothing about the account.
	if page := serve(s, fromTunnel("GET", "/oauth/authorize?"+c.authorizeQuery(challenge, c.redirect).Encode(), nil, "")).Body.String(); strings.Contains(html.UnescapeString(page), "Nenhum número") {
		t.Fatalf("the first step says whether a number is connected:\n%s", page)
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

// The consent is in two steps. Someone not signed in to the panel sees only
// who is asking and the password: not the number, masked or whole, not
// whether one is connected, not the access choices. The right password
// signs the browser in and brings it back to the same request, now showing
// the number, and approving there needs no second password.
func TestTheConsentIsInTwoSteps(t *testing.T) {
	const phone = "5516999996892"
	s, _, _ := agentServer(t)
	s.UsePhone(func(context.Context) string { return phone })
	c := registerClient(t, s, "https://claude.ai/api/mcp/auth_callback")
	verifier, challenge := pkce("2")
	q := c.authorizeQuery(challenge, c.redirect)
	get := func(header map[string]string) *httptest.ResponseRecorder {
		return serve(s, fromTunnel("GET", "/oauth/authorize?"+q.Encode(), header, ""))
	}
	post := func(header map[string]string, extra url.Values) *httptest.ResponseRecorder {
		form := url.Values{}
		for k, v := range q {
			form[k] = v
		}
		for k, v := range extra {
			form[k] = v
		}
		h := map[string]string{"Origin": origin}
		for k, v := range header {
			h[k] = v
		}
		return serve(s, fromTunnel("POST", "/oauth/authorize", h, form.Encode()))
	}
	noAccount := func(what, page string) {
		t.Helper()
		page = html.UnescapeString(page)
		for _, leak := range []string{"6892", phone, "Número do WhatsApp", "Nenhum número conectado", `name="access"`, "volta para:"} {
			if strings.Contains(page, leak) {
				t.Errorf("%s has %q:\n%s", what, leak, page)
			}
		}
	}

	// Step 1: who asks, and the password.
	rec := get(nil)
	page := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(page, "Passo 1 de 2") || !strings.Contains(page, "claude.ai quer usar") ||
		!strings.Contains(page, `type="password"`) || !strings.Contains(page, `name="step" value="password"`) {
		t.Fatalf("the first step: %d\n%s", rec.Code, page)
	}
	noAccount("the first step", page)

	// Password and permission in one go, without a session: no code, and
	// the first step again.
	rec = post(nil, url.Values{"password": {testPassword}, "access": {"full"}, "decision": {"allow"}})
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("Location") != "" || !strings.Contains(rec.Body.String(), "Passo 1 de 2") {
		t.Fatalf("an approval without a session: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	noAccount("an approval without a session", rec.Body.String())

	// A wrong password stays at the first step, with the error, signed out.
	rec = post(nil, url.Values{"step": {"password"}, "password": {"wrong password!"}})
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "Senha errada.") || !strings.Contains(rec.Body.String(), "Passo 1 de 2") {
		t.Fatalf("a wrong password: %d\n%s", rec.Code, rec.Body)
	}
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == CookieName && ck.Value != "" {
			t.Fatal("a wrong password made a session")
		}
	}
	noAccount("the first step after a wrong password", rec.Body.String())

	// The right password: a panel session, as the sign-in page makes, and
	// back to this same request on this server.
	rec = post(nil, url.Values{"step": {"password"}, "password": {testPassword}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("the right password: %d\n%s", rec.Code, rec.Body)
	}
	ck := sessionCookie(t, rec)
	if !ck.HttpOnly || !ck.Secure || ck.SameSite != http.SameSiteLaxMode || ck.Path != "/" {
		t.Errorf("the session cookie: %+v", ck)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil || loc.Host != "" || loc.Scheme != "" || loc.Path != "/oauth/authorize" {
		t.Fatalf("the first step sends to %q", rec.Header().Get("Location"))
	}
	for k := range q {
		if loc.Query().Get(k) != q.Get(k) {
			t.Errorf("the request lost %s: %q", k, loc.Query().Get(k))
		}
	}
	if loc.Query().Has("password") || strings.Contains(loc.String(), testPassword) {
		t.Fatal("the password is in the address")
	}
	session := map[string]string{"Cookie": CookieName + "=" + ck.Value}

	// Step 2: the number, masked, and the choices; no second password.
	page = html.UnescapeString(get(session).Body.String())
	if !strings.Contains(page, "Passo 2 de 2") || !strings.Contains(page, "+55 (16) 9••••-6892") || !strings.Contains(page, `name="access"`) ||
		strings.Contains(page, `type="password"`) || strings.Contains(page, phone) {
		t.Fatalf("the second step:\n%s", page)
	}

	// Another signed-in browser does not ride on that password: the first
	// approval of this client still asks for it there.
	other := map[string]string{"Cookie": CookieName + "=" + sessionCookie(t, signIn(s, "127.0.0.1:40000", testPassword, nil)).Value}
	if page := get(other).Body.String(); strings.Contains(page, "Passo") || !strings.Contains(page, `type="password"`) {
		t.Fatalf("another session's page:\n%s", page)
	}
	if rec := post(other, url.Values{"access": {"full"}, "decision": {"allow"}}); rec.Header().Get("Location") != "" {
		t.Fatalf("another session approved without the password: %q", rec.Header().Get("Location"))
	}

	// Approving issues the code as before.
	rec = post(session, url.Values{"access": {"read"}, "decision": {"allow"}})
	back, _ := url.Parse(rec.Header().Get("Location"))
	if rec.Code != http.StatusSeeOther || back == nil || !strings.HasPrefix(rec.Header().Get("Location"), c.redirect+"?") ||
		back.Query().Get("code") == "" || back.Query().Get("state") != "st4te" || back.Query().Get("iss") != origin {
		t.Fatalf("the approval: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if r, out := c.token(url.Values{"grant_type": {"authorization_code"}, "code": {back.Query().Get("code")}, "client_id": {c.id},
		"redirect_uri": {c.redirect}, "code_verifier": {verifier}, "resource": {origin + "/mcp"}}); r.Code != 200 || out["access_token"] == nil {
		t.Fatalf("the exchange: %d %v", r.Code, out)
	}
	// The first step's password was spent with the code.
	if s.passwordPassed(request("GET", "/", "127.0.0.1:1", session, ""), readAuthRequest(q)) {
		t.Error("the first step's password still stands after the approval")
	}
}

// A browser already signed in goes straight to the permission.
func TestSignedInGoesStraightToThePermission(t *testing.T) {
	s, _, _ := agentServer(t)
	s.UsePhone(func(context.Context) string { return "5516999996892" })
	c := registerClient(t, s, "https://claude.ai/api/mcp/auth_callback")
	_, challenge := pkce("3")
	q := c.authorizeQuery(challenge, c.redirect)
	session := map[string]string{"Cookie": CookieName + "=" + sessionCookie(t, signIn(s, "127.0.0.1:40000", testPassword, nil)).Value}
	page := html.UnescapeString(serve(s, fromTunnel("GET", "/oauth/authorize?"+q.Encode(), session, "")).Body.String())
	if strings.Contains(page, `name="step"`) || !strings.Contains(page, "+55 (16) 9••••-6892") || !strings.Contains(page, `name="access"`) {
		t.Fatalf("a signed-in browser's page:\n%s", page)
	}
}
