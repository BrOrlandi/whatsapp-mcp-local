package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/httpserver"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/public"
)

// outsideRequest is a request from a tunnel on this computer, with a header
// set (and others removed when given empty).
func outsideRequest(method, path, body string, header map[string]string) *http.Request {
	r := httptest.NewRequest(method, "http://127.0.0.1:47822"+path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:50000"
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-For", "198.51.100.7")
	r.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		if v == "" {
			r.Header.Del(k)
		} else {
			r.Header.Set(k, v)
		}
	}
	return r
}

func serveAgent(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// accessToken walks the OAuth flow through the public listener, as an MCP
// client does, and returns an access token.
func accessToken(t *testing.T, h http.Handler, access string) string {
	t.Helper()
	const redirect = "https://claude.ai/api/mcp/auth_callback"
	rec := serveAgent(h, outsideRequest("POST", public.RegisterPath, `{"client_name":"Test client","redirect_uris":["`+redirect+`"]}`, nil))
	var reg struct {
		ClientID string `json:"client_id"`
	}
	if json.Unmarshal(rec.Body.Bytes(), &reg) != nil || reg.ClientID == "" {
		t.Fatalf("registration: %d %s", rec.Code, rec.Body)
	}
	verifier := strings.Repeat("v", 64)
	sum := sha256.Sum256([]byte(verifier))
	form := url.Values{"response_type": {"code"}, "client_id": {reg.ClientID}, "redirect_uri": {redirect}, "state": {"s"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"},
		"resource": {publicURL + "/mcp"}, "password": {publicPassword}, "access": {access}, "decision": {"allow"}}
	rec = serveAgent(h, outsideRequest("POST", public.AuthorizePath, form.Encode(),
		map[string]string{"Origin": publicURL, "Content-Type": "application/x-www-form-urlencoded"}))
	loc, _ := url.Parse(rec.Header().Get("Location"))
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatalf("consent: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	ex := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {reg.ClientID},
		"redirect_uri": {redirect}, "code_verifier": {verifier}, "resource": {publicURL + "/mcp"}}
	rec = serveAgent(h, outsideRequest("POST", public.TokenPath, ex.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded"}))
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if json.Unmarshal(rec.Body.Bytes(), &tok) != nil || tok.AccessToken == "" {
		t.Fatalf("token: %d %s", rec.Code, rec.Body)
	}
	return tok.AccessToken
}

// An agent's credential, a valid API key or a valid OAuth access token,
// reaches the MCP, its media links and the health report, and nothing of
// the panel. The Open routes and the public listener's own routes (sign-in,
// OAuth) are the way in for everyone. The panel's session never reaches the
// MCP.
func TestAgentCredentialsReachOnlyTheMCP(t *testing.T) {
	d := startServerMode(t, true)
	h := d.PublicHandler()
	key, _, err := public.CreateKey(context.Background(), d.primary.st, "Test key", false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	credentials := map[string]string{"api key": key, "access token": accessToken(t, h, "full")}
	reach := []string{"/mcp", "/media/", "/health"}
	own := append(append([]string{}, public.OwnRoutes...), public.OAuthRoutes...)
	routes := append(append([]string{}, d.patterns...), own...)
	checked := 0
	for name, credential := range credentials {
		for _, pattern := range routes {
			access, _ := httpserver.RouteAccess(pattern)
			if access == caller.Open || slices.Contains(own, pattern) {
				continue
			}
			method, path := concrete(pattern)
			rec := serveAgent(h, outsideRequest(method, path, `{"jsonrpc":"2.0","id":1,"method":"ping"}`,
				map[string]string{"Authorization": "Bearer " + credential, "Origin": publicURL, "Sec-Fetch-Site": "same-origin"}))
			refused := rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden
			if slices.Contains(reach, pattern) {
				if refused {
					t.Errorf("%s: %s %s refused (%d %.100s)", name, method, path, rec.Code, rec.Body)
				}
			} else if rec.Code != http.StatusForbidden {
				t.Errorf("%s: %s %s answered %d, want 403 (%.100s)", name, method, path, rec.Code, rec.Body)
			}
			checked++
		}
	}
	if checked < 100 {
		t.Fatalf("only %d checks", checked)
	}

	// The panel's session, valid, is not an agent's credential.
	cookie := signedIn(t, h)
	rec := serveAgent(h, outsideRequest("POST", "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		map[string]string{"Cookie": cookie, "Origin": publicURL, "Sec-Fetch-Site": "same-origin"}))
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Header().Get("WWW-Authenticate"), "resource_metadata=") {
		t.Fatalf("/mcp with the panel's session: %d", rec.Code)
	}
	// Nor is it in the Authorization header.
	value := strings.TrimPrefix(cookie, public.CookieName+"=")
	if rec := serveAgent(h, outsideRequest("POST", "/mcp", "{}", map[string]string{"Authorization": "Bearer " + value})); rec.Code != http.StatusUnauthorized {
		t.Fatalf("/mcp with the session as a Bearer: %d", rec.Code)
	}
}

// The exception, stated: a download_media link works on the public address
// with no credential, by its own secret (192 random bits, ten minutes).
// Without a live link, /media/ is refused like the rest.
func TestAMediaLinkIsItsOwnCredential(t *testing.T) {
	d := startServerMode(t, true)
	h := d.PublicHandler()
	file := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(file, []byte("not really a photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	token := d.primary.server.Links().Add(file, "image/jpeg", 10*time.Minute)
	if len(token) < 48 {
		t.Fatalf("the link's secret is %d hex digits", len(token))
	}
	if rec := serveAgent(h, outsideRequest("GET", "/media/"+token, "", nil)); rec.Code != 200 || rec.Body.String() != "not really a photo" {
		t.Fatalf("a live link with no credential: %d %s", rec.Code, rec.Body)
	}
	for name, path := range map[string]string{"a made-up link": "/media/" + strings.Repeat("0", 48), "no link": "/media/"} {
		if rec := serveAgent(h, outsideRequest("GET", path, "", nil)); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	if rec := serveAgent(h, outsideRequest("GET", "/media/"+token, "", map[string]string{"Authorization": "Bearer wamcp_key_wrong"})); rec.Code != http.StatusUnauthorized {
		t.Errorf("a live link with a wrong key: %d", rec.Code)
	}
}

func toolsCall(t *testing.T, h http.Handler, credential, body string) map[string]any {
	t.Helper()
	rec := serveAgent(h, outsideRequest("POST", "/mcp", body, map[string]string{"Authorization": "Bearer " + credential}))
	var out map[string]any
	if json.Unmarshal(rec.Body.Bytes(), &out) != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	return out
}

// A read-only connection lists only the read tools and is refused a write.
func TestAReadOnlyAgent(t *testing.T) {
	d := startServerMode(t, true)
	h := d.PublicHandler()
	ro, _, _ := public.CreateKey(context.Background(), d.primary.st, "Reader", true, time.Now())
	full, _, _ := public.CreateKey(context.Background(), d.primary.st, "Writer", false, time.Now())
	count := func(credential string) int {
		out := toolsCall(t, h, credential, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
		return len(out["result"].(map[string]any)["tools"].([]any))
	}
	readOnly, all := count(ro), count(full)
	if readOnly == 0 || readOnly >= all {
		t.Fatalf("read-only sees %d tools, full %d", readOnly, all)
	}
	token := accessToken(t, h, "read")
	if n := count(token); n != readOnly {
		t.Fatalf("a read-only login sees %d tools, want %d", n, readOnly)
	}
	for _, credential := range []string{ro, token} {
		out := toolsCall(t, h, credential, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"send_text_message","arguments":{"to":"1@s.whatsapp.net","message":"x"}}}`)
		b, _ := json.Marshal(out)
		if !strings.Contains(string(b), "not allowed for this connection") {
			t.Errorf("a read-only send: %s", b)
		}
	}
}
