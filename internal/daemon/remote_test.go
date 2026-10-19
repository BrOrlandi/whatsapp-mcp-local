package daemon

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
	for _, want := range []string{"data-remote-banner", "whatsapp.home.example", "data-remote-link", "data-remote-whatsapp", `id="remoto"`, "Mac do Bruno"} {
		if !strings.Contains(body, want) {
			t.Errorf("the settings page over the session lacks %q (status %d)", want, code)
		}
	}
	// A page asked for directly on the loopback port has no banner.
	_, local, err := get2(d.MCPURL(), "/configuracoes")
	if err != nil || strings.Contains(local, "data-remote-banner") {
		t.Errorf("a local page shows the remote banner: %v", err)
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
