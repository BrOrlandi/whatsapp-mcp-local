package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/panel"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/public"
)

const localToken = "a-local-token-of-some-length"

// startWithToken runs a gateway with a local token, in server mode, so the
// route that makes API keys exists too.
func startWithToken(t *testing.T, token string) *Daemon {
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
	hash, _ := bcrypt.GenerateFromPassword([]byte(publicPassword), public.MinCost)
	if err := os.WriteFile(public.PasswordPath(data), hash, 0o600); err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{public.EnvURL: publicURL, public.EnvListen: "127.0.0.1:" + strconv.Itoa(freePort(t))}
	pub, err := public.FromEnv(func(k string) string { return vars[k] }, data)
	if err != nil {
		t.Fatal(err)
	}
	var d *Daemon
	d, err = Start(Config{Port: freePort(t), Token: token, WacliBin: fakeWacli, StoreDir: store, DataDir: data, Public: pub,
		Panel: func(p *panel.Panel) { p.Host = fakeHost{port: func(port int) error { return d.SetPort(port) }} }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Stop)
	waitSync(t, d, "connected")
	return d
}

// local is a request on the local listener, as a program on this computer
// sends it.
func local(d *Daemon, method, path, body string, header map[string]string) *httptest.ResponseRecorder {
	host := "127.0.0.1:" + strconv.Itoa(d.Port())
	r := httptest.NewRequest(method, "http://"+host+path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:50001"
	for k, v := range header {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	d.HTTPHandler().ServeHTTP(rec, r)
	return rec
}

func countKeys(t *testing.T, d *Daemon) int {
	t.Helper()
	keys, err := d.Primary().st.APIKeys(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return len(keys)
}

// With a local token, a program that sends the panel's own Origin is still a
// program: it cannot add a webhook or make an API key without the token. A
// browser types the token once and acts with its session; programs send it
// as a Bearer, as before.
func TestTheLocalTokenCannotBeSkippedWithAnOrigin(t *testing.T) {
	d := startWithToken(t, localToken)
	own := "http://127.0.0.1:" + strconv.Itoa(d.Port())
	forged := map[string]string{"Origin": own, "Sec-Fetch-Site": "same-origin", "Content-Type": "application/json"}
	hook := `{"url":"http://127.0.0.1:9/x","events":["message"]}`

	if rec := local(d, "POST", "/api/webhooks", hook, forged); rec.Code != http.StatusForbidden {
		t.Fatalf("a webhook with a forged Origin and no token: %d %s", rec.Code, rec.Body)
	}
	form := map[string]string{"Origin": own, "Sec-Fetch-Site": "same-origin", "Content-Type": "application/x-www-form-urlencoded"}
	rec := local(d, "POST", "/conexoes/chaves", url.Values{"name": {"x"}}.Encode(), form)
	if rec.Code == http.StatusOK || countKeys(t, d) != 0 {
		t.Fatalf("an API key with a forged Origin and no token: %d, %d keys", rec.Code, countKeys(t, d))
	}
	if rec := local(d, "POST", "/whatsapp/nome", url.Values{"name": {"x"}}.Encode(), form); rec.Code != http.StatusSeeOther ||
		!strings.HasPrefix(rec.Header().Get("Location"), panel.TokenPath) {
		t.Fatalf("a form without the token goes to the token page: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	// A wrong token is refused too, as a Bearer or on the page.
	if rec := local(d, "POST", "/api/webhooks", hook, map[string]string{"Origin": own, "Content-Type": "application/json", "Authorization": "Bearer wrong"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a wrong Bearer with the Origin: %d", rec.Code)
	}
	if rec := local(d, "POST", panel.TokenPath, url.Values{"token": {"wrong"}}.Encode(), form); rec.Code != http.StatusUnauthorized || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("a wrong token typed: %d %v", rec.Code, rec.Result().Cookies())
	}

	// A program with the token, as the command line sends it.
	if rec := local(d, "POST", "/api/webhooks", hook, map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + localToken}); rec.Code != http.StatusOK {
		t.Fatalf("a program with the token: %d %s", rec.Code, rec.Body)
	}

	// A browser: the panel asks for the token, then acts with its session.
	if rec := local(d, "GET", "/configuracoes", "", map[string]string{"Sec-Fetch-Site": "none"}); rec.Code != http.StatusSeeOther ||
		!strings.HasPrefix(rec.Header().Get("Location"), panel.TokenPath+"?next=%2Fconfiguracoes") {
		t.Fatalf("a page without the token's session: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := local(d, "GET", panel.TokenPath, "", nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `name="token"`) {
		t.Fatalf("the token page: %d", rec.Code)
	}
	rec = local(d, "POST", panel.TokenPath, url.Values{"token": {localToken}, "next": {"/configuracoes"}}.Encode(), form)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/configuracoes" {
		t.Fatalf("the right token typed: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	var session string
	for _, c := range rec.Result().Cookies() {
		if c.Value != "" {
			session = c.Name + "=" + c.Value
			if strings.Contains(c.Value, localToken) || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
				t.Fatalf("the session cookie: %+v", c)
			}
		}
	}
	if session == "" {
		t.Fatal("no session cookie")
	}
	withSession := map[string]string{"Origin": own, "Sec-Fetch-Site": "same-origin", "Content-Type": "application/json", "Cookie": session}
	if rec := local(d, "POST", "/api/webhooks", hook, withSession); rec.Code != http.StatusOK {
		t.Fatalf("the panel with the token's session: %d %s", rec.Code, rec.Body)
	}
	if rec := local(d, "GET", "/configuracoes", "", map[string]string{"Cookie": session}); rec.Code != http.StatusOK {
		t.Fatalf("a page with the token's session: %d", rec.Code)
	}
	// A session the panel did not sign is worth nothing.
	if rec := local(d, "POST", "/api/webhooks", hook, map[string]string{"Origin": own, "Content-Type": "application/json", "Cookie": "whatsapp_mcp_local_" + strconv.Itoa(d.Port()) + "=1.2.3"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a made-up session: %d", rec.Code)
	}

	// Reads stay open to programs, as docs/api.md says.
	if rec := local(d, "GET", "/api/status", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("a read without the token: %d", rec.Code)
	}
	// The app's own window never asks.
	r := httptest.NewRequest("POST", "/api/webhooks", strings.NewReader(hook))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	d.AppHandler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("the app's window with a token set: %d %s", w.Code, w.Body)
	}
}

// Without a token nothing changes: the panel's page acts as before.
func TestWithoutATokenThePanelActsAsBefore(t *testing.T) {
	d := startWithToken(t, "")
	own := "http://127.0.0.1:" + strconv.Itoa(d.Port())
	if rec := local(d, "POST", "/api/webhooks", `{"url":"http://127.0.0.1:9/x","events":["message"]}`,
		map[string]string{"Origin": own, "Sec-Fetch-Site": "same-origin", "Content-Type": "application/json"}); rec.Code != http.StatusOK {
		t.Fatalf("the panel without a token: %d %s", rec.Code, rec.Body)
	}
	if rec := local(d, "GET", "/configuracoes", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("a page without a token: %d", rec.Code)
	}
	if rec := local(d, "GET", panel.TokenPath, "", nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("the token page without a token: %d", rec.Code)
	}
}

// A public address that is http:// starts, as before, but the log says
// plainly what that exposes, and so does the panel seen from outside.
func TestAnHTTPPublicAddressIsWarnedAbout(t *testing.T) {
	base := ""
	if runtime.GOOS != "windows" {
		base = "/tmp"
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
	hash, _ := bcrypt.GenerateFromPassword([]byte(publicPassword), public.MinCost)
	_ = os.WriteFile(public.PasswordPath(data), hash, 0o600)
	vars := map[string]string{public.EnvURL: "http://192.0.2.10:47822", public.EnvListen: "127.0.0.1:" + strconv.Itoa(freePort(t))}
	pub, err := public.FromEnv(func(k string) string { return vars[k] }, data)
	if err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	d, err := Start(Config{Port: freePort(t), WacliBin: fakeWacli, StoreDir: store, DataDir: data, Public: pub,
		Logger: slog.New(slog.NewTextHandler(&log, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Stop)
	if !strings.Contains(log.String(), "level=WARN") || !strings.Contains(log.String(), "the public address is http://") {
		t.Fatalf("no warning in the log:\n%s", log.String())
	}
	// The panel's addresses card, seen from outside, says so too.
	body := url.Values{"password": {publicPassword}}.Encode()
	r := httptest.NewRequest("POST", "http://192.0.2.10:47822"+public.LoginPath, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:50000"
	r.Header.Set("Origin", "http://192.0.2.10:47822")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	d.PublicHandler().ServeHTTP(rec, r)
	var cookie string
	for _, c := range rec.Result().Cookies() {
		if c.Name == public.CookieName {
			cookie = c.Name + "=" + c.Value
		}
	}
	if cookie == "" {
		t.Fatalf("signing in: %d", rec.Code)
	}
	r = httptest.NewRequest("GET", "http://192.0.2.10:47822/configuracoes", nil)
	r.RemoteAddr = "127.0.0.1:50000"
	r.Header.Set("Cookie", cookie)
	rec = httptest.NewRecorder()
	d.PublicHandler().ServeHTTP(rec, r)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Este endereço não é protegido.") {
		t.Fatalf("the panel does not warn: %d", rec.Code)
	}
}

// Every route that acts, with a local token set, refuses a program that only
// sends the panel's own Origin: none answers as if the token had been given.
func TestEveryActionAsksForTheLocalToken(t *testing.T) {
	d := startWithToken(t, localToken)
	own := "http://127.0.0.1:" + strconv.Itoa(d.Port())
	checked := 0
	for _, pattern := range d.patterns {
		method, path := concrete(pattern)
		if method == "GET" || method == "HEAD" || pattern == "POST "+panel.TokenPath || path == "/healthz" {
			continue
		}
		for _, ctype := range []string{"application/json", "application/x-www-form-urlencoded"} {
			rec := local(d, method, path, "{}", map[string]string{"Origin": own, "Sec-Fetch-Site": "same-origin", "Content-Type": ctype})
			switch {
			case rec.Code == http.StatusUnauthorized, rec.Code == http.StatusForbidden, rec.Code == http.StatusNotFound, rec.Code == http.StatusMethodNotAllowed:
			case rec.Code == http.StatusSeeOther && strings.HasPrefix(rec.Header().Get("Location"), panel.TokenPath):
			default:
				t.Errorf("%s %s (%s) without the token answered %d (%.120s)", method, path, ctype, rec.Code, rec.Body)
			}
		}
		checked++
	}
	if checked < 30 {
		t.Fatalf("only %d routes checked", checked)
	}
}

// A command that holds the token opens the browser signed in, with a code
// that works once; a program without the token gets no code. Typing wrong
// tokens soon has to wait.
func TestTheBrowserLandsSignedInAndGuessesWait(t *testing.T) {
	d := startWithToken(t, localToken)
	own := "http://127.0.0.1:" + strconv.Itoa(d.Port())
	if rec := local(d, "POST", "/api/token/codigo", "{}", map[string]string{"Origin": own, "Sec-Fetch-Site": "same-origin", "Content-Type": "application/json"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a code without the token: %d", rec.Code)
	}
	rec := local(d, "POST", "/api/token/codigo", "{}", map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + localToken})
	var out struct {
		URL string `json:"url"`
	}
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &out) != nil || !strings.HasPrefix(out.URL, panel.TokenPath+"?codigo=") {
		t.Fatalf("a code with the token: %d %s", rec.Code, rec.Body)
	}
	rec = local(d, "GET", out.URL+"&next=%2Fstatus", "", map[string]string{"Sec-Fetch-Site": "none"})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/status" || len(rec.Result().Cookies()) != 1 {
		t.Fatalf("opening with the code: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	c := rec.Result().Cookies()[0]
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
		t.Fatalf("the cookie: %+v", c)
	}
	if rec := local(d, "GET", "/status", "", map[string]string{"Cookie": c.Name + "=" + c.Value}); rec.Code != 200 {
		t.Fatalf("the panel with that session: %d", rec.Code)
	}
	if rec := local(d, "GET", out.URL, "", nil); rec.Code != 200 || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("the same code again: %d %v", rec.Code, rec.Result().Cookies())
	}
	// The page says where the token is.
	if rec := local(d, "GET", panel.TokenPath, "", nil); !strings.Contains(rec.Body.String(), "token show") {
		t.Fatalf("the token page does not say where the token is:\n%s", rec.Body)
	}

	form := map[string]string{"Origin": own, "Content-Type": "application/x-www-form-urlencoded"}
	codes := []int{}
	for i := 0; i < 5; i++ {
		codes = append(codes, local(d, "POST", panel.TokenPath, url.Values{"token": {"wrong"}}.Encode(), form).Code)
	}
	if codes[0] != http.StatusUnauthorized || codes[4] != http.StatusTooManyRequests {
		t.Fatalf("wrong tokens answered %v", codes)
	}
	// The right token is never held back by the wrong ones, and clears them.
	rec = local(d, "POST", panel.TokenPath, url.Values{"token": {localToken}}.Encode(), form)
	if rec.Code != http.StatusSeeOther || len(rec.Result().Cookies()) != 1 {
		t.Fatalf("the right token while wrong ones wait: %d", rec.Code)
	}
	if rec := local(d, "POST", panel.TokenPath, url.Values{"token": {"wrong"}}.Encode(), form); rec.Code != http.StatusUnauthorized {
		t.Fatalf("a wrong token after the right one: %d", rec.Code)
	}
	// Each listener has its own cookie: browsers keep cookies by host, not
	// port, and the app and the command line may both run on 127.0.0.1.
	if want := "whatsapp_mcp_local_" + strconv.Itoa(d.Port()); c.Name != want {
		t.Fatalf("the cookie is named %q, want %q", c.Name, want)
	}
	other := "whatsapp_mcp_local_1=" + c.Value
	if rec := local(d, "POST", "/api/webhooks", `{"url":"http://127.0.0.1:9/x","events":["message"]}`,
		map[string]string{"Origin": own, "Content-Type": "application/json", "Cookie": other}); rec.Code != http.StatusForbidden {
		t.Fatalf("a session under another port's name: %d", rec.Code)
	}
}

// Everything a tool reaches with Authorization: Bearer answers as before the
// local sign-in existed: the MCP, health, the API's reads and actions.
func TestBearerClientsAreUnchanged(t *testing.T) {
	d := startWithToken(t, localToken)
	bearer := map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + localToken}
	list := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	if rec := local(d, "POST", "/mcp", list, bearer); rec.Code != 200 || !strings.Contains(rec.Body.String(), "send_text_message") {
		t.Fatalf("/mcp with the token: %d", rec.Code)
	}
	for _, h := range []map[string]string{{"Content-Type": "application/json"}, {"Content-Type": "application/json", "Authorization": "Bearer wrong"}} {
		if rec := local(d, "POST", "/mcp", list, h); rec.Code != http.StatusUnauthorized {
			t.Fatalf("/mcp with %v: %d", h, rec.Code)
		}
		if rec := local(d, "GET", "/health", "", h); rec.Code != http.StatusUnauthorized {
			t.Fatalf("/health with %v: %d", h, rec.Code)
		}
	}
	if rec := local(d, "GET", "/health", "", bearer); rec.Code != 200 && rec.Code != 503 {
		t.Fatalf("/health with the token: %d", rec.Code)
	}
	for _, path := range []string{"/api/status", "/api/state", "/api/pulse", "/api/webhooks", "/diagnostico/logs.txt"} {
		if rec := local(d, "GET", path, "", bearer); rec.Code != 200 {
			t.Errorf("GET %s with the token: %d", path, rec.Code)
		}
	}
	for _, path := range []string{"/api/status", "/api/pulse"} {
		if rec := local(d, "GET", path, "", nil); rec.Code != 200 {
			t.Errorf("GET %s without the token (reads are open): %d", path, rec.Code)
		}
	}
	if rec := local(d, "POST", "/api/media/retention", `{"days": 30}`, bearer); rec.Code != 200 {
		t.Errorf("an action with the token: %d %s", rec.Code, rec.Body)
	}
	if rec := local(d, "POST", "/api/media/retention", `{"days": 30}`, map[string]string{"Content-Type": "application/json"}); rec.Code != http.StatusForbidden {
		t.Errorf("an action without the token: %d", rec.Code)
	}
}
