package public

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/httpserver"
)

const testPassword = "correct horse battery"

func hashOf(t *testing.T, password string, cost int) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		t.Fatal(err)
	}
	return string(h)
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestTheServerModeIsOffWithoutItsVariables(t *testing.T) {
	dir := t.TempDir()
	// A password file alone does not turn it on.
	if err := os.WriteFile(PasswordPath(dir), []byte(hashOf(t, testPassword, MinCost)), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := FromEnv(env(nil), dir)
	if c != nil || err != nil {
		t.Fatalf("got %v, %v", c, err)
	}
}

func TestAHalfMadeServerModeDoesNotStart(t *testing.T) {
	good := hashOf(t, testPassword, MinCost)
	for name, tc := range map[string]struct {
		env  map[string]string
		want string
	}{
		"hash without url":    {map[string]string{EnvPasswordHash: good}, EnvURL},
		"url without hash":    {map[string]string{EnvURL: "https://x.example.com"}, EnvPasswordHash},
		"plain text password": {map[string]string{EnvURL: "https://x.example.com", EnvPasswordHash: testPassword}, "bcrypt"},
		"cheap hash":          {map[string]string{EnvURL: "https://x.example.com", EnvPasswordHash: hashOf(t, testPassword, 4)}, "4"},
		"malformed hash":      {map[string]string{EnvURL: "https://x.example.com", EnvPasswordHash: "$2a$10$short"}, "bcrypt"},
		"url with a path":     {map[string]string{EnvURL: "https://x.example.com/painel", EnvPasswordHash: good}, EnvURL},
		"url not http":        {map[string]string{EnvURL: "ftp://x.example.com", EnvPasswordHash: good}, EnvURL},
		"bad listen":          {map[string]string{EnvURL: "https://x.example.com", EnvPasswordHash: good, EnvListen: "nowhere"}, EnvListen},
		"bad proxy":           {map[string]string{EnvURL: "https://x.example.com", EnvPasswordHash: good, EnvTrustedProxy: "10.0.0.0/33"}, EnvTrustedProxy},
	} {
		c, err := FromEnv(env(tc.env), t.TempDir())
		if err == nil || c != nil {
			t.Errorf("%s: started (%v)", name, c)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %q does not say %q", name, err, tc.want)
		}
	}
}

func TestTheServerModeFromTheVariablesOrThePasswordFile(t *testing.T) {
	dir := t.TempDir()
	c, err := FromEnv(env(map[string]string{EnvURL: "https://WhatsApp.Example.com/", EnvPasswordHash: hashOf(t, testPassword, MinCost)}), dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Origin != "https://whatsapp.example.com" || c.Listen != DefaultListen || !c.HTTPS() {
		t.Fatalf("got %+v", c)
	}
	if err := os.WriteFile(PasswordPath(dir), []byte(hashOf(t, testPassword, MinCost)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err = FromEnv(env(map[string]string{EnvURL: "http://192.0.2.10:8080", EnvListen: "0.0.0.0:47822"}), dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPS() || c.Listen != "0.0.0.0:47822" {
		t.Fatalf("got %+v", c)
	}
}

// testServer is the public listener in front of a stand-in for the daemon's
// routes, which reports the caller each request reached it with.
func testServer(t *testing.T, publicURL, proxies string) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, PasswordFile)
	if err := os.WriteFile(path, []byte(hashOf(t, testPassword, MinCost)), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := FromEnv(env(map[string]string{EnvURL: publicURL, EnvTrustedProxy: proxies}), dir)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	report := func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("caller=" + string(caller.From(r.Context()).Kind)))
	}
	mux.HandleFunc("GET /status", report)
	mux.HandleFunc("GET /api/status", report)
	mux.HandleFunc("GET /favicon.svg", report)
	return New(c, mux, nil), path
}

func request(method, target, peer string, header map[string]string, body string) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.RemoteAddr = peer
	if body != "" {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range header {
		r.Header.Set(k, v)
	}
	return r
}

func serve(s *Server, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, r)
	return rec
}

// signIn posts the password from a peer, as the panel's page does.
func signIn(s *Server, peer, password string, extra map[string]string) *httptest.ResponseRecorder {
	h := map[string]string{"Origin": s.cfg.Origin, "X-Forwarded-Proto": "https"}
	for k, v := range extra {
		h[k] = v
	}
	return serve(s, request("POST", "/entrar", peer, h, url.Values{"password": {password}, "next": {"/status"}}.Encode()))
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName && c.Value != "" {
			return c
		}
	}
	t.Fatalf("no session cookie: %d %s", rec.Code, rec.Body)
	return nil
}

func TestSigningIn(t *testing.T) {
	s, _ := testServer(t, "https://wa.example.com", "")
	const peer = "127.0.0.1:40000"
	https := map[string]string{"X-Forwarded-Proto": "https"}

	// Nothing but the Open routes without a session.
	if rec := serve(s, request("GET", "/status", peer, https, "")); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/entrar?next=%2Fstatus" {
		t.Fatalf("a page without a session: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := serve(s, request("GET", "/api/status", peer, https, "")); rec.Code != http.StatusUnauthorized {
		t.Fatalf("the API without a session: %d", rec.Code)
	}
	if rec := serve(s, request("GET", "/favicon.svg", peer, https, "")); rec.Code != 200 || rec.Body.String() != "caller=anonymous" {
		t.Fatalf("an icon: %d %s", rec.Code, rec.Body)
	}
	if rec := serve(s, request("GET", "/entrar", peer, https, "")); rec.Code != 200 || !strings.Contains(rec.Body.String(), `type="password"`) {
		t.Fatalf("the sign-in page: %d", rec.Code)
	}

	// A sign-in from another site, or with no origin, is refused.
	for _, origin := range []string{"", "https://evil.example", "http://127.0.0.1:47822"} {
		rec := serve(s, request("POST", "/entrar", peer, map[string]string{"Origin": origin, "X-Forwarded-Proto": "https"}, "password="+url.QueryEscape(testPassword)))
		if rec.Code != http.StatusForbidden || len(rec.Result().Cookies()) != 0 {
			t.Errorf("a sign-in from %q: %d", origin, rec.Code)
		}
	}

	if rec := signIn(s, peer, "wrong password!", nil); rec.Code != http.StatusUnauthorized || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("a wrong password: %d", rec.Code)
	}
	rec := signIn(s, peer, testPassword, nil)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/status" {
		t.Fatalf("the right password: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	c := sessionCookie(t, rec)
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
		t.Fatalf("the cookie: %+v", c)
	}
	withSession := map[string]string{"X-Forwarded-Proto": "https", "Cookie": CookieName + "=" + c.Value}
	if rec := serve(s, request("GET", "/status", peer, withSession, "")); rec.Code != 200 || rec.Body.String() != "caller=panel" {
		t.Fatalf("a page with the session: %d %s", rec.Code, rec.Body)
	}
	// The MCP is not on the public address yet, session or not.
	if rec := serve(s, request("POST", "/mcp", peer, withSession, "")); rec.Code != http.StatusUnauthorized {
		t.Fatalf("/mcp: %d", rec.Code)
	}

	// Signing out ends the session on the server, not only in the browser.
	out := map[string]string{"X-Forwarded-Proto": "https", "Cookie": CookieName + "=" + c.Value, "Origin": s.cfg.Origin}
	if rec := serve(s, request("POST", "/sair", peer, out, "")); rec.Code != http.StatusSeeOther {
		t.Fatalf("signing out: %d", rec.Code)
	}
	if rec := serve(s, request("GET", "/status", peer, withSession, "")); rec.Code != http.StatusSeeOther {
		t.Fatalf("the session after signing out: %d", rec.Code)
	}
}

func TestTheCookieIsNotSecureOnAnHTTPAddress(t *testing.T) {
	s, _ := testServer(t, "http://192.0.2.10:47822", "")
	rec := signIn(s, "192.0.2.50:1000", testPassword, nil)
	if c := sessionCookie(t, rec); c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("the cookie: %+v", c)
	}
}

func TestANewPasswordEndsEverySession(t *testing.T) {
	s, path := testServer(t, "https://wa.example.com", "")
	const peer = "127.0.0.1:40000"
	c := sessionCookie(t, signIn(s, peer, testPassword, nil))
	h := map[string]string{"X-Forwarded-Proto": "https", "Cookie": CookieName + "=" + c.Value}
	if rec := serve(s, request("GET", "/status", peer, h, "")); rec.Code != 200 {
		t.Fatalf("before: %d", rec.Code)
	}
	// What `password set` does; the modification time moves forward even on
	// a coarse clock.
	if err := os.WriteFile(path, []byte(hashOf(t, "another password here", MinCost)), 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(path, later, later)
	if rec := serve(s, request("GET", "/status", peer, h, "")); rec.Code != http.StatusSeeOther {
		t.Fatalf("after a new password the session still works: %d", rec.Code)
	}
	if rec := signIn(s, peer, testPassword, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("the old password still works: %d", rec.Code)
	}
	sessionCookie(t, signIn(s, peer, "another password here", nil))
}

// Wrong passwords slow their own address down, never another one.
func TestTheThrottleIsPerAddress(t *testing.T) {
	s, _ := testServer(t, "https://wa.example.com", "")
	now := time.Now()
	s.throttle.now = func() time.Time { return now }
	a := map[string]string{"X-Forwarded-For": "198.51.100.7"}
	for i := 0; i < 8; i++ {
		signIn(s, "127.0.0.1:40000", "wrong password!", a)
	}
	if rec := signIn(s, "127.0.0.1:40000", testPassword, a); rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("address A was not slowed down: %d", rec.Code)
	}
	b := map[string]string{"X-Forwarded-For": "203.0.113.9"}
	sessionCookie(t, signIn(s, "127.0.0.1:40000", testPassword, b))
	// After the wait, A signs in.
	now = now.Add(maxDelay + time.Second)
	sessionCookie(t, signIn(s, "127.0.0.1:40000", testPassword, a))
}

// When the proxy does not say whom it forwards, every visitor shares its
// address, and the wait stays a few seconds.
func TestTheSharedAddressWaitsOnlySeconds(t *testing.T) {
	s, _ := testServer(t, "https://wa.example.com", "")
	now := time.Now()
	s.throttle.now = func() time.Time { return now }
	for i := 0; i < 50; i++ {
		signIn(s, "127.0.0.1:40000", "wrong password!", nil)
	}
	if w := s.throttle.wait("127.0.0.1"); w <= 0 || w > maxSharedWait {
		t.Fatalf("the shared address waits %v", w)
	}
	now = now.Add(maxSharedWait)
	sessionCookie(t, signIn(s, "127.0.0.1:40000", testPassword, nil))
}

// The three kinds of peer: loopback (the tunnel next door), believed; any
// other peer without a trusted proxy configured, not believed; and a
// configured range, believed while everyone else is refused.
func TestWhoseForwardedHeadersAreBelieved(t *testing.T) {
	headers := map[string]string{"X-Forwarded-For": "198.51.100.7", "X-Forwarded-Proto": "https"}

	s, _ := testServer(t, "https://wa.example.com", "")
	if pr := s.cfg.proxies.read(request("GET", "/", "127.0.0.1:5000", headers, "")); !pr.proxy || pr.client != "198.51.100.7" || !pr.https || pr.shared {
		t.Errorf("loopback: %+v", pr)
	}
	if pr := s.cfg.proxies.read(request("GET", "/", "[::1]:5000", headers, "")); !pr.proxy || pr.client != "198.51.100.7" {
		t.Errorf("loopback v6: %+v", pr)
	}
	pr := s.cfg.proxies.read(request("GET", "/", "192.0.2.80:5000", headers, ""))
	if pr.proxy || pr.client != "192.0.2.80" || pr.https {
		t.Errorf("a peer on the network: %+v", pr)
	}
	// Under https it is refused: nobody types the password over plain http.
	if rec := serve(s, request("GET", "/entrar", "192.0.2.80:5000", headers, "")); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "did not arrive over HTTPS") {
		t.Errorf("a peer on the network under https: %d %s", rec.Code, rec.Body)
	}
	// Over its own TLS, it is fine.
	r := request("GET", "/entrar", "192.0.2.80:5000", nil, "")
	r.TLS = &tls.ConnectionState{}
	if rec := serve(s, r); rec.Code != 200 {
		t.Errorf("a peer over TLS: %d", rec.Code)
	}

	s, _ = testServer(t, "https://wa.example.com", "172.18.0.0/16, 10.1.2.3")
	if pr := s.cfg.proxies.read(request("GET", "/", "172.18.0.5:5000", headers, "")); !pr.proxy || pr.client != "198.51.100.7" || !pr.https {
		t.Errorf("the configured range: %+v", pr)
	}
	if pr := s.cfg.proxies.read(request("GET", "/", "10.1.2.3:5000", headers, "")); !pr.proxy {
		t.Errorf("the configured address: %+v", pr)
	}
	for _, peer := range []string{"127.0.0.1:5000", "172.19.0.5:5000"} {
		if rec := serve(s, request("GET", "/entrar", peer, headers, "")); rec.Code != http.StatusForbidden {
			t.Errorf("%s is not the proxy, and answered %d", peer, rec.Code)
		}
	}
	// A client cannot pass for another by writing X-Forwarded-For itself:
	// the proxy appends the address it saw, which is the one believed.
	spoofed := map[string]string{"X-Forwarded-For": "203.0.113.1, 198.51.100.7"}
	if pr := s.cfg.proxies.read(request("GET", "/", "172.18.0.5:5000", spoofed, "")); pr.client != "198.51.100.7" {
		t.Errorf("a spoofed hop was believed: %+v", pr)
	}
	// Nothing forwarded: the proxy's own address, shared.
	if pr := s.cfg.proxies.read(request("GET", "/", "172.18.0.5:5000", nil, "")); pr.client != "172.18.0.5" || !pr.shared {
		t.Errorf("no X-Forwarded-For: %+v", pr)
	}
}

func TestSafeReturn(t *testing.T) {
	for in, want := range map[string]string{"/status": "/status", "/conectar/codex?x=1": "/conectar/codex?x=1", "": "/",
		"//evil.example": "/", "https://evil.example": "/", "/\\evil.example": "/", "status": "/"} {
		if got := safeReturn(in); got != want {
			t.Errorf("safeReturn(%q) = %q, want %q", in, got, want)
		}
	}
}

// Every route the public listener adds is classified as the way in, and
// the daemon's own routes go through the table.
func TestOwnRoutesAreNotInTheDaemonsTable(t *testing.T) {
	for _, p := range OwnRoutes {
		if _, ok := httpserver.RouteAccess(p); ok {
			t.Errorf("%s is the public listener's own route, not the daemon's", p)
		}
	}
}
