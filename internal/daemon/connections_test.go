package daemon

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The panel in server mode, seen from outside: the Conexões page makes a
// key, shows it once, lists it and cuts it off; the pages give the public
// address, never this computer's.
func TestTheConnectionsPage(t *testing.T) {
	d := startServerMode(t, true)
	h := d.PublicHandler()
	cookie := signedIn(t, h)
	page := func(method, path, body string) *http.Response {
		hdr := map[string]string{"Cookie": cookie, "Origin": publicURL, "Sec-Fetch-Site": "same-origin"}
		if body != "" {
			hdr["Content-Type"] = "application/x-www-form-urlencoded"
		}
		return serveAgent(h, outsideRequest(method, path, body, hdr)).Result()
	}
	read := func(r *http.Response) string {
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				return b.String()
			}
		}
	}
	r := page("GET", "/conexoes", "")
	if body := read(r); r.StatusCode != 200 || !strings.Contains(body, publicURL+"/mcp") {
		t.Fatalf("the page: %d", r.StatusCode)
	}
	r = page("POST", "/conexoes/chaves", url.Values{"name": {"Notebook"}, "read_only": {"1"}}.Encode())
	body := read(r)
	key := regexp.MustCompile(`wamcp_key_[A-Za-z0-9_-]{43}`).FindString(body)
	if r.StatusCode != 200 || key == "" || r.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("making a key: %d", r.StatusCode)
	}
	// It works, and the list shows it but never again the key.
	if rec := serveAgent(h, outsideRequest("POST", "/mcp", `{"jsonrpc":"2.0","id":1,"method":"ping"}`, map[string]string{"Authorization": "Bearer " + key})); rec.Code != 200 {
		t.Fatalf("the new key: %d", rec.Code)
	}
	list := read(page("GET", "/conexoes", ""))
	if !strings.Contains(list, "Notebook") || strings.Contains(list, key) {
		t.Fatal("the list does not show the key's name, or shows the key")
	}
	id := regexp.MustCompile(`name="id" value="(k_[0-9a-f]+)"`).FindStringSubmatch(list)
	if id == nil {
		t.Fatal("no key to revoke in the list")
	}
	if r := page("POST", "/conexoes/revogar", url.Values{"kind": {"key"}, "id": {id[1]}}.Encode()); r.StatusCode != http.StatusSeeOther {
		t.Fatalf("revoking: %d", r.StatusCode)
	}
	if rec := serveAgent(h, outsideRequest("POST", "/mcp", "{}", map[string]string{"Authorization": "Bearer " + key})); rec.Code != 401 {
		t.Fatalf("a revoked key: %d", rec.Code)
	}
	// A login shows up there too, and is cut off the same way.
	token := accessToken(t, h, "full")
	list = read(page("GET", "/conexoes", ""))
	login := regexp.MustCompile(`name="kind" value="login"><input type="hidden" name="id" value="(c_[0-9a-f]+)"`).FindStringSubmatch(list)
	if login == nil || !strings.Contains(list, "Test client") {
		t.Fatal("the login is not listed")
	}
	page("POST", "/conexoes/revogar", url.Values{"kind": {"login"}, "id": {login[1]}}.Encode())
	if rec := serveAgent(h, outsideRequest("POST", "/mcp", "{}", map[string]string{"Authorization": "Bearer " + token})); rec.Code != 401 {
		t.Fatalf("a revoked login: %d", rec.Code)
	}

	// Seen from outside, the pages give the public address and the two
	// ways in, and never the MCP's address on this computer (the webhook
	// examples' receivers, on another port, are the reader's own).
	local := "127.0.0.1:" + strconv.Itoa(d.Port())
	for _, path := range []string{"/", "/status", "/conectar", "/configuracoes", "/webhooks/documentacao", "/api/status"} {
		r := page("GET", path, "")
		body := read(r)
		if r.StatusCode == http.StatusSeeOther {
			continue // the first run sends / to the setup
		}
		if strings.Contains(body, local) {
			t.Errorf("%s shows the MCP's local address", path)
		}
	}
	if body := read(page("GET", "/conectar", "")); !strings.Contains(body, publicURL+"/mcp") || !strings.Contains(body, "Authorization: Bearer") {
		t.Error("the connect page does not give the public address and the two ways in")
	}
}

// Without the server mode, the page does not exist.
func TestNoConnectionsPageWithoutServerMode(t *testing.T) {
	d := startServerMode(t, false)
	rec := serveAgent(d.HTTPHandler(), outsideRequest("GET", "/conexoes", "", map[string]string{"X-Forwarded-For": "", "X-Forwarded-Proto": ""}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("/conexoes: %d", rec.Code)
	}
}
