package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/remote"
)

// The app on another computer pairs with the daemon and uses its panel, API
// and MCP through the session, as if they were local; the remote banner
// says which server and how its WhatsApp stands.
func TestTheAppUsesTheDaemonRemotely(t *testing.T) {
	d, _ := startDaemon(t, freePort(t), true)
	defer d.Stop()
	proxy := httptest.NewTLSServer(d.Remote().Handler()) // the HTTPS proxy in front
	defer proxy.Close()
	remote.Transport = proxy.Client().Transport
	defer func() { remote.Transport = nil }()

	pair := func(scope string) *remote.Client {
		code, err := d.Remote().NewCode("Mac", scope)
		if err != nil {
			t.Fatal(err)
		}
		dev, key, err := remote.Pair(context.Background(), proxy.URL, code.Code, "Mac do Bruno")
		if err != nil {
			t.Fatal(err)
		}
		c, err := remote.NewClient(dev, key)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		go c.Run(ctx)
		for i := 0; i < 200 && c.Status().State != remote.StateConnected; i++ {
			waitABit()
		}
		return c
	}
	get := func(c *remote.Client, path string, header map[string]string) (int, string) {
		req, _ := http.NewRequest(http.MethodGet, "http://remote"+path, nil)
		for k, v := range header {
			req.Header.Set(k, v)
		}
		resp, err := c.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}

	app := pair(remote.ScopeAdmin)
	code, body := get(app, "/api/status", nil)
	var st struct {
		App      string `json:"app"`
		WhatsApp struct {
			Paired bool `json:"paired"`
		} `json:"whatsapp"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &st) != nil || st.App != "WhatsApp MCP Local" || !st.WhatsApp.Paired {
		t.Fatalf("/api/status over the session: %d %s", code, body)
	}
	code, body = get(app, "/configuracoes", map[string]string{"X-WhatsApp-MCP-Server": "https://whatsapp.home.example"})
	for _, want := range []string{"data-remote-status", "whatsapp.home.example", "MCP Remoto", `href="/__app/remoto"`, `id="remoto"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the settings page over the session lacks %q (status %d)", want, code)
		}
	}
	// Pairing another computer lives in the app's own page; the server's
	// panel, seen through the app, does not offer it.
	for _, unwanted := range []string{"data-remote-code", "data-remote-devices"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("the settings page over the session shows %q", unwanted)
		}
	}
	// A page asked for directly on the loopback port has no remote status card.
	_, local, err := get2(d.MCPURL(), "/configuracoes")
	if err != nil || strings.Contains(local, "data-remote-status") {
		t.Errorf("a local page shows the remote status card: %v", err)
	}

	// An agent elsewhere: its page names the server's address, and a key.
	code, body = get(app, "/conectar/hermes", map[string]string{"X-WhatsApp-MCP-Server": "https://whatsapp.home.example"})
	for _, want := range []string{"https://whatsapp.home.example/mcp", "data-api-key-open"} {
		if code != 200 || !strings.Contains(body, want) {
			t.Errorf("the agent's page over the session lacks %q (status %d)", want, code)
		}
	}

	tools := pair(remote.ScopeTools)
	if code, _ := get(tools, "/api/status", nil); code != http.StatusForbidden {
		t.Errorf("a tools device read the API: %d", code)
	}
	req, _ := http.NewRequest(http.MethodPost, "http://remote/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := tools.RoundTrip(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("a tools device and the MCP: %v %v", resp, err)
	}
	resp.Body.Close()
}

func get2(mcpURL, path string) (int, string, error) {
	base := strings.TrimSuffix(mcpURL, "/mcp")
	resp, err := http.Get(base + path)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), nil
}

func waitABit() { <-time.After(25 * time.Millisecond) }

// A program on the app's computer reaches the server through the app with
// the same rights it would have on the server: the panel's checks and the
// server's token apply to it. Only what the app's window sends is served as
// the window.
func TestOnlyTheAppsWindowIsTheWindow(t *testing.T) {
	root := t.TempDir()
	store := root + "/s"
	_ = os.MkdirAll(store, 0o700)
	_ = os.WriteFile(store+"/AUTHED", nil, 0o600)
	d, err := Start(Config{Port: freePort(t), RequirePort: true, Token: "server-token-0123456789", WacliBin: fakeWacli,
		StoreDir: store, DataDir: root + "/data"})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Stop()
	proxy := httptest.NewTLSServer(d.Remote().Handler())
	defer proxy.Close()
	remote.Transport = proxy.Client().Transport
	defer func() { remote.Transport = nil }()
	code, _ := d.Remote().NewCode("Mac", remote.ScopeAdmin)
	dev, key, err := remote.Pair(context.Background(), proxy.URL, code.Code, "Mac")
	if err != nil {
		t.Fatal(err)
	}
	c, _ := remote.NewClient(dev, key)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)
	for i := 0; i < 200 && c.Status().State != remote.StateConnected; i++ {
		waitABit()
	}
	post := func(path string, header map[string]string) int {
		req, _ := http.NewRequest(http.MethodPost, "http://remote"+path, strings.NewReader(`{"name":"x"}`))
		req.Header.Set("Content-Type", "application/json")
		for k, v := range header {
			req.Header.Set(k, v)
		}
		resp, err := c.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if got := post("/api/remote/codigo", nil); got != http.StatusForbidden {
		t.Errorf("a program without the token made a pairing code: %d", got)
	}
	if got := post("/api/remote/codigo", map[string]string{"Authorization": "Bearer server-token-0123456789"}); got != 200 {
		t.Errorf("a program with the server's token: %d", got)
	}
	if got := post("/mcp", nil); got != http.StatusUnauthorized {
		t.Errorf("the MCP without the token: %d", got)
	}
	if got := post("/api/remote/codigo", map[string]string{remote.WindowHeader: "1"}); got != 200 {
		t.Errorf("the app's window: %d", got)
	}
	// The app's own status poll (for the tray) asks as its window does, and
	// gets the status even with a token on the server.
	req, _ := http.NewRequest(http.MethodGet, "http://remote/api/status", nil)
	req.Header.Set(remote.WindowHeader, "1")
	resp, err := c.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		WhatsApp struct {
			Paired bool `json:"paired"`
		} `json:"whatsapp"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&st)
	resp.Body.Close()
	if resp.StatusCode != 200 || !st.WhatsApp.Paired {
		t.Errorf("the app's status poll: %d %+v", resp.StatusCode, st)
	}
	// The server's token goes to the app's window only, for the AI tools
	// the app connects on its computer; with it, their MCP calls pass.
	token := func(header map[string]string) (int, string) {
		req, _ := http.NewRequest(http.MethodGet, "http://remote/api/remote/token", nil)
		for k, v := range header {
			req.Header.Set(k, v)
		}
		resp, err := c.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var b struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&b)
		return resp.StatusCode, b.Token
	}
	if code, tok := token(nil); code != http.StatusForbidden || tok != "" {
		t.Errorf("a program got the server's token: %d %q", code, tok)
	}
	if code, tok := token(map[string]string{"Authorization": "Bearer server-token-0123456789"}); code != http.StatusForbidden || tok != "" {
		t.Errorf("only the window gets the token, not a program: %d", code)
	}
	code2, tok := token(map[string]string{remote.WindowHeader: "1"})
	if code2 != 200 || tok != "server-token-0123456789" {
		t.Fatalf("the app's window and the token: %d %q", code2, tok)
	}
	if got := post("/mcp", map[string]string{"Authorization": "Bearer " + tok}); got == http.StatusUnauthorized || got >= 300 {
		t.Errorf("an AI tool with the token the app wrote: %d", got)
	}
}

// download_media's link, asked for through the app in remote mode, opens on
// the app's own computer: it points at the app's port, and the app brings
// the file from the server through the session.
func TestAMediaLinkOpensThroughTheApp(t *testing.T) {
	root, err := os.MkdirTemp(tmpBase(), "daemon-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	store := filepath.Join(root, "s")
	_ = os.MkdirAll(store, 0o700)
	_ = os.WriteFile(filepath.Join(store, "AUTHED"), nil, 0o600)
	seedStore(t, store)
	d, err := Start(Config{Port: freePort(t), WacliBin: fakeWacli, StoreDir: store, DataDir: filepath.Join(root, "data")})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Stop()
	media := filepath.Join(root, "data", "media", testMae)
	_ = os.MkdirAll(media, 0o700)
	_ = os.WriteFile(filepath.Join(media, "D1.txt"), []byte("arroz\nfeijão\n"), 0o600)

	proxy := httptest.NewTLSServer(d.Remote().Handler())
	defer proxy.Close()
	remote.Transport = proxy.Client().Transport
	defer func() { remote.Transport = nil }()
	code, _ := d.Remote().NewCode("Mac", remote.ScopeTools)
	dev, key, err := remote.Pair(context.Background(), proxy.URL, code.Code, "Mac")
	if err != nil {
		t.Fatal(err)
	}
	c, _ := remote.NewClient(dev, key)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)
	for i := 0; i < 200 && c.Status().State != remote.StateConnected; i++ {
		waitABit()
	}

	// The app: its own port on this computer, forwarding through the session
	// and saying where it is, as cmd/app's proxy does.
	app := httptest.NewUnstartedServer(nil)
	appBase := "http://" + app.Listener.Addr().String()
	app.Config.Handler = &httputil.ReverseProxy{Transport: c, Rewrite: func(r *httputil.ProxyRequest) {
		r.Out.URL.Scheme, r.Out.URL.Host = "http", "remote"
		r.Out.Header.Set(remote.LocalBaseHeader, appBase)
	}}
	app.Start()
	defer app.Close()

	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "download_media", "arguments": map[string]any{"message_id": "D1", "link": true}}})
	resp, err := http.Post(appBase+"/mcp", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Result struct{ Content []map[string]any }
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	var res map[string]any
	if len(out.Result.Content) == 0 || json.Unmarshal([]byte(fmt.Sprint(out.Result.Content[0]["text"])), &res) != nil {
		t.Fatalf("download_media: %v", out)
	}
	link := fmt.Sprint(res["url"])
	if !strings.HasPrefix(link, appBase+"/media/") {
		t.Fatalf("the link points at %s, not at the app (%s)", link, appBase)
	}
	got, err := http.Get(link)
	if err != nil {
		t.Fatal(err)
	}
	file, _ := io.ReadAll(got.Body)
	got.Body.Close()
	if got.StatusCode != 200 || string(file) != "arroz\nfeijão\n" {
		t.Fatalf("the link through the app: %d %q", got.StatusCode, file)
	}
}

func TestOnlyALoopbackBaseRebasesLinks(t *testing.T) {
	for base, want := range map[string]bool{"http://127.0.0.1:47821": true, "http://127.0.0.1:1": true,
		"http://evil.example:47821": false, "https://127.0.0.1:47821": false, "http://127.0.0.1:47821/x": false,
		"http://127.0.0.1:0": false, "http://127.0.0.1:070": false, "": false} {
		if localBase(base) != want {
			t.Errorf("localBase(%q) = %v", base, !want)
		}
	}
}

func tmpBase() string {
	if runtime.GOOS != "windows" {
		return "/tmp" // the store's socket path must stay short
	}
	return ""
}
