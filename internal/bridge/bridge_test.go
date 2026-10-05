package bridge

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// A port changed in the app while Claude Desktop keeps the bridge running:
// the bridge must find the daemon at its new address on its own.
func TestRunFollowsTheDaemonToANewPort(t *testing.T) {
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("X-MCP-Client") != "bridge:claude-ai" {
			t.Errorf("client hint = %q", r.Header.Get("X-MCP-Client"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + idOf(body) + `,"result":{}}`))
	}))
	defer daemon.Close()
	gone := httptest.NewServer(http.NotFoundHandler())
	goneURL := gone.URL + "/mcp"
	gone.Close()

	var asked atomic.Int32
	url := func() string {
		if asked.Add(1) == 1 {
			return goneURL
		}
		return daemon.URL + "/mcp"
	}
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"claude-ai"}}}` + "\n")
	var out bytes.Buffer
	if err := Run(context.Background(), Options{URL: url, NotRunning: "not open"}, in, &out); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != `{"jsonrpc":"2.0","id":1,"result":{}}` {
		t.Fatalf("answer = %s", got)
	}
}

func TestRunSaysTheDaemonIsNotOpen(t *testing.T) {
	gone := httptest.NewServer(http.NotFoundHandler())
	url := gone.URL + "/mcp"
	gone.Close()
	in := strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"tools/list"}` + "\n" + `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")
	var out bytes.Buffer
	if err := Run(context.Background(), Options{URL: func() string { return url }, NotRunning: "WhatsApp MCP is not open"}, in, &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], `"id":7`) || !strings.Contains(lines[0], "WhatsApp MCP is not open") {
		t.Fatalf("a request must get the not-open error, and a notification nothing: %q", out.String())
	}
}

func idOf(body []byte) string {
	s := string(body)
	i := strings.Index(s, `"id":`)
	if i < 0 {
		return "null"
	}
	rest := s[i+5:]
	end := strings.IndexAny(rest, ",}")
	return rest[:end]
}
