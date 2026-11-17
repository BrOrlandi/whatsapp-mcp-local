package daemon

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/httpserver"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/panel"
)

// startWithHost runs a gateway with the app around the panel, so the app's
// own routes are registered too.
func startWithHost(t *testing.T) *Daemon {
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
	store := filepath.Join(root, "s")
	_ = os.MkdirAll(store, 0o700)
	_ = os.WriteFile(filepath.Join(store, "AUTHED"), nil, 0o600)
	var d *Daemon
	d, err = Start(Config{Port: freePort(t), WacliBin: fakeWacli, StoreDir: store, DataDir: filepath.Join(root, "data"),
		Panel: func(p *panel.Panel) { p.Host = fakeHost{port: func(port int) error { return d.SetPort(port) }} }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Stop)
	return d
}

// Every route the server registers says which callers reach it, so a new
// route cannot be born open on a listener that faces the network.
func TestEveryRouteIsClassified(t *testing.T) {
	d := startWithHost(t)
	if len(d.patterns) == 0 {
		t.Fatal("no routes recorded")
	}
	for _, p := range d.patterns {
		if _, ok := httpserver.RouteAccess(p); !ok {
			t.Errorf("the route %q is not classified in internal/httpserver/routes.go", p)
		}
	}
	// And the table names no route that does not exist.
	for _, p := range httpserver.ClassifiedPatterns() {
		if !slices.Contains(d.patterns, p) {
			t.Errorf("routes.go classifies %q, which is not registered", p)
		}
	}
}

// A caller that does not reach a route is refused by it; the local caller
// reaches every one. The listener that faces the network does not exist yet,
// so the other callers are set on the request here.
func TestEachRouteChecksItsCaller(t *testing.T) {
	d := startWithHost(t)
	waitSync(t, d, "connected")
	refusedBy := map[caller.Access]caller.Kind{caller.PanelAccess: caller.OAuth, caller.MCP: caller.Panel,
		caller.Media: "", caller.Health: ""}
	for _, p := range d.patterns {
		access, _ := httpserver.RouteAccess(p)
		kind, checked := refusedBy[access]
		if !checked {
			continue
		}
		method, path, found := strings.Cut(p, " ")
		if !found {
			method, path = "POST", p
		}
		path = strings.NewReplacer("{$}", "", "{tool}", "codex", "{jid}", "1@s.whatsapp.net", "{client}", "codex", "{id}", "1").Replace(path)
		if strings.HasSuffix(path, "/media/") {
			path += "x"
		}
		r := httptest.NewRequest(method, "http://127.0.0.1"+path, nil)
		// The page's own headers pass the same-origin check, so a 403 can
		// only be the caller's.
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		r.Header.Set("Origin", "http://127.0.0.1")
		r = r.WithContext(caller.With(r.Context(), caller.Caller{Kind: kind}))
		rec := httptest.NewRecorder()
		d.routes.ServeHTTP(rec, r)
		if rec.Code != 403 {
			t.Errorf("%s answered %d to a %q caller", p, rec.Code, kind)
		}
	}
}
