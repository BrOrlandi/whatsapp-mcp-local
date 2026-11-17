package daemon

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/httpserver"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/panel"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/public"
)

const (
	publicURL      = "https://wa.example.com"
	publicPassword = "a throwaway test password"
)

// startServerMode runs a gateway with the app around the panel (so every
// route exists), and with the server mode when withPublic is set.
func startServerMode(t *testing.T, withPublic bool) *Daemon {
	t.Helper()
	base := ""
	if runtime.GOOS != "windows" {
		base = "/tmp" // the store's socket path must stay short
	}
	root, err := os.MkdirTemp(base, "daemon-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	store, data := filepath.Join(root, "s"), filepath.Join(root, "data")
	_ = os.MkdirAll(store, 0o700)
	_ = os.MkdirAll(data, 0o700)
	_ = os.WriteFile(filepath.Join(store, "AUTHED"), nil, 0o600)
	var pub *public.Config
	if withPublic {
		hash, _ := bcrypt.GenerateFromPassword([]byte(publicPassword), public.MinCost)
		if err := os.WriteFile(public.PasswordPath(data), hash, 0o600); err != nil {
			t.Fatal(err)
		}
		vars := map[string]string{public.EnvURL: publicURL, public.EnvListen: "127.0.0.1:" + strconv.Itoa(freePort(t))}
		if pub, err = public.FromEnv(func(k string) string { return vars[k] }, data); err != nil {
			t.Fatal(err)
		}
	}
	var d *Daemon
	d, err = Start(Config{Port: freePort(t), WacliBin: fakeWacli, StoreDir: store, DataDir: data, Public: pub,
		Panel: func(p *panel.Panel) { p.Host = fakeHost{port: func(port int) error { return d.SetPort(port) }} }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Stop)
	waitSync(t, d, "connected")
	return d
}

// concrete turns a route's pattern into a request for it.
func concrete(pattern string) (method, path string) {
	method, path, found := strings.Cut(pattern, " ")
	if !found {
		method, path = "POST", pattern
	}
	path = strings.NewReplacer("{$}", "", "{tool}", "codex", "{jid}", "1@s.whatsapp.net", "{client}", "codex", "{id}", "1").Replace(path)
	if strings.HasSuffix(path, "/media/") {
		path += "x"
	}
	return method, path
}

// fromTunnel is a request as a tunnel on this computer forwards it.
func fromTunnel(method, path string, header map[string]string) *http.Request {
	r := httptest.NewRequest(method, "http://127.0.0.1:47822"+path, strings.NewReader("{}"))
	r.RemoteAddr = "127.0.0.1:50000"
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-For", "198.51.100.7")
	r.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		r.Header.Set(k, v)
	}
	return r
}

// Every route on the public listener, without a credential and with wrong
// ones, is refused or sent to the sign-in page: none answers as the local
// caller would. Only the Open routes, and the sign-in page itself, answer.
func TestThePublicListenerFailsClosed(t *testing.T) {
	d := startServerMode(t, true)
	h := d.PublicHandler()
	if h == nil {
		t.Fatal("no public listener")
	}
	wrong := []map[string]string{
		nil,
		{"Cookie": public.CookieName + "=not-a-session", "Authorization": "Bearer not-a-key"},
		{"Cookie": public.CookieName + "=", "Origin": publicURL, "Sec-Fetch-Site": "same-origin"},
	}
	routes := append(append([]string{}, d.patterns...), public.OwnRoutes...)
	checked := 0
	for _, pattern := range routes {
		access, classified := httpserver.RouteAccess(pattern)
		own := strings.HasSuffix(pattern, " "+public.LoginPath) || strings.HasSuffix(pattern, " "+public.LogoutPath)
		if classified && access == caller.Open || own && strings.HasPrefix(pattern, "GET ") {
			continue
		}
		method, path := concrete(pattern)
		for _, hdr := range wrong {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, fromTunnel(method, path, hdr))
			switch {
			case rec.Code == http.StatusUnauthorized, rec.Code == http.StatusForbidden:
			case rec.Code == http.StatusSeeOther && strings.HasPrefix(rec.Header().Get("Location"), public.LoginPath):
			default:
				t.Errorf("%s %s with %v answered %d (%.120s)", method, path, hdr, rec.Code, rec.Body)
			}
			if len(rec.Result().Cookies()) > 0 && rec.Result().Cookies()[0].Value != "" {
				t.Errorf("%s %s with %v handed out a cookie", method, path, hdr)
			}
		}
		checked++
	}
	if checked < 50 {
		t.Fatalf("only %d routes checked", checked)
	}
}

// signedIn signs in to the public listener as the panel's page does.
func signedIn(t *testing.T, h http.Handler) string {
	t.Helper()
	body := url.Values{"password": {publicPassword}}.Encode()
	r := httptest.NewRequest("POST", "http://127.0.0.1:47822"+public.LoginPath, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:50000"
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("Origin", publicURL)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	for _, c := range rec.Result().Cookies() {
		if c.Name == public.CookieName && c.Value != "" {
			return public.CookieName + "=" + c.Value
		}
	}
	t.Fatalf("could not sign in: %d %s", rec.Code, rec.Body)
	return ""
}

func TestASignedInPanelOnThePublicListener(t *testing.T) {
	d := startServerMode(t, true)
	h := d.PublicHandler()
	cookie := signedIn(t, h)
	page := func(method, path string, hdr map[string]string) *httptest.ResponseRecorder {
		all := map[string]string{"Cookie": cookie, "Origin": publicURL, "Sec-Fetch-Site": "same-origin"}
		for k, v := range hdr {
			all[k] = v
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, fromTunnel(method, path, all))
		return rec
	}
	rec := page("GET", "/conectar", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `action="/sair"`) || !strings.Contains(rec.Body.String(), "de fora do servidor") {
		t.Fatalf("the connect page: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if rec := page("GET", "/status", nil); rec.Code != 200 {
		t.Fatalf("/status: %d", rec.Code)
	}
	if rec := page("GET", "/api/status", map[string]string{"Origin": ""}); rec.Code != 200 {
		t.Fatalf("/api/status: %d", rec.Code)
	}
	// What only makes sense on the server itself is refused, in one place.
	for _, pattern := range d.patterns {
		if a, _ := httpserver.RouteAccess(pattern); a != caller.LocalOnly {
			continue
		}
		method, path := concrete(pattern)
		if rec := page(method, path, nil); rec.Code != http.StatusForbidden {
			t.Errorf("%s answered %d to a panel session", pattern, rec.Code)
		}
	}
	if rec := page("POST", "/api/clients/codex", nil); !strings.Contains(rec.Body.String(), "próprio servidor") {
		t.Errorf("the refusal does not say why: %s", rec.Body)
	}
	// The same-origin checks hold against the public address.
	for name, hdr := range map[string]map[string]string{
		"another site":          {"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"},
		"the loopback":          {"Origin": "http://127.0.0.1:47822"},
		"no origin (a program)": {"Origin": "", "Sec-Fetch-Site": ""},
	} {
		if rec := page("POST", "/api/idioma", hdr); rec.Code != http.StatusForbidden {
			t.Errorf("a change from %s answered %d", name, rec.Code)
		}
	}
	// From the panel's own page it passes both checks: the empty body is
	// then refused as a request, not as a caller.
	if rec := page("POST", "/api/idioma", nil); rec.Code == http.StatusForbidden || rec.Code == http.StatusUnauthorized {
		t.Errorf("a change from the panel's own page answered %d: %s", rec.Code, rec.Body)
	}
	if rec := page("POST", "/mcp", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("/mcp answered %d", rec.Code)
	}
}

var unstable = regexp.MustCompile(`127\.0\.0\.1:\d+|\d{4}-\d\d-\d\dT[0-9:.]+[-+Z0-9:]*|"(ms|uptime[a-z_]*|started_at|since|at|last[a-z_]*)":\s*("[^"]*"|[0-9.]+)|daemon-\d+`)

// localAnswers is what the local listener answers to a fixed set of
// requests, with what changes from run to run taken out.
func localAnswers(t *testing.T, d *Daemon, extra map[string]string) []string {
	t.Helper()
	h := d.HTTPHandler()
	var out []string
	for _, c := range []struct {
		method, path, host, body string
		hdr                      map[string]string
	}{
		{"POST", "/mcp", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil},
		{"POST", "/mcp", "", `{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"clientInfo":{"name":"x"}}}`, nil},
		{"GET", "/healthz", "", "", nil},
		{"GET", "/api/status", "", "", nil},
		{"GET", "/api/webhooks", "", "", nil},
		{"GET", "/funcoes", "", "", nil},
		{"GET", "/", "", "", nil},
		{"GET", "/api/pair/qr.png", "", "", nil},
		{"POST", "/api/clients/nada", "", `{}`, nil},
		{"POST", "/api/configuracoes", "", `{}`, map[string]string{"Origin": "https://evil.example"}},
		{"POST", "/whatsapp/nome", "", "", map[string]string{"Origin": "https://evil.example"}},
		{"GET", "/api/status", "wa.example.com", "", nil},
		{"GET", public.LoginPath, "", "", nil},
		{"POST", public.LogoutPath, "", "", nil},
	} {
		host := c.host
		if host == "" {
			host = "127.0.0.1:47821"
		}
		r := httptest.NewRequest(c.method, "http://"+host+c.path, strings.NewReader(c.body))
		r.RemoteAddr = "127.0.0.1:50000"
		if c.body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		for k, v := range extra {
			r.Header.Set(k, v)
		}
		for k, v := range c.hdr {
			r.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		out = append(out, c.method+" "+c.path+" "+c.host+" -> "+rec.Result().Status+" "+rec.Header().Get("Location")+"\n"+
			unstable.ReplaceAllString(rec.Body.String(), "X"))
	}
	return out
}

// The local listener answers the same with the server mode on or off, and
// never takes the panel's session or a forwarded header for who is calling.
// Without the server mode's variables, nothing else listens.
func TestTheLocalListenerIsTheSameInServerMode(t *testing.T) {
	off := startServerMode(t, false)
	if off.PublicHandler() != nil || off.PublicAddr() != "" {
		t.Fatal("a public listener without the server mode")
	}
	if c, err := public.FromEnv(func(string) string { return "" }, t.TempDir()); c != nil || err != nil {
		t.Fatalf("server mode with no variables: %v %v", c, err)
	}
	want := localAnswers(t, off, nil)

	on := startServerMode(t, true)
	if on.PublicAddr() == "" {
		t.Fatal("no public listener in server mode")
	}
	cookie := signedIn(t, on.PublicHandler())
	for name, extra := range map[string]map[string]string{
		"plain": nil,
		"with the panel's session and forwarded headers": {"Cookie": cookie, "X-Forwarded-For": "198.51.100.7",
			"X-Forwarded-Proto": "https", "X-Forwarded-Host": "wa.example.com"},
	} {
		got := localAnswers(t, on, extra)
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: the local listener changed:\n--- off\n%.400s\n--- on\n%.400s", name, want[i], got[i])
			}
		}
	}
}
