package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/remote"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/transfer"
)

// transferDaemon runs a gateway that can restart, on a store paired or not,
// and the app's session with it.
func transferDaemon(t *testing.T, paired bool) (*Daemon, string, *remote.Client, chan struct{}) {
	t.Helper()
	base := ""
	if runtime.GOOS != "windows" {
		base = "/tmp"
	}
	root, err := os.MkdirTemp(base, "daemon-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	store := filepath.Join(root, "s")
	if err := os.MkdirAll(store, 0o700); err != nil {
		t.Fatal(err)
	}
	if paired {
		for _, name := range []string{"AUTHED", "session.db"} {
			if err := os.WriteFile(filepath.Join(store, name), []byte(name), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	restarted := make(chan struct{}, 1)
	d, err := Start(Config{Port: freePort(t), RequirePort: true, WacliBin: fakeWacli, StoreDir: store, DataDir: filepath.Join(root, "data"),
		Restart: func() { restarted <- struct{}{} }})
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewTLSServer(d.Remote().Handler())
	t.Cleanup(proxy.Close)
	remote.Transport = proxy.Client().Transport
	t.Cleanup(func() { remote.Transport = nil })
	code, err := d.Remote().NewCode("Mac", remote.ScopeAdmin)
	if err != nil {
		t.Fatal(err)
	}
	dev, key, err := remote.Pair(context.Background(), proxy.URL, code.Code, "Mac")
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
	return d, store, c, restarted
}

func call(t *testing.T, c *remote.Client, method, path string, body io.Reader, window bool) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, "http://remote"+path, body)
	if window {
		req.Header.Set(remote.WindowHeader, "1")
	}
	resp, err := c.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestOnlyTheAppsWindowReachesTransfers(t *testing.T) {
	d, _, c, _ := transferDaemon(t, true)
	defer d.Stop()
	resp := call(t, c, http.MethodGet, "/api/transferencia", nil, false)
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Error("a program on the app's computer reached the transfers")
	}
	if code, _, _ := get(d.Port(), "/api/transferencia"); code == http.StatusOK {
		t.Error("the loopback port serves the transfers")
	}
	resp = call(t, c, http.MethodGet, "/api/transferencia", nil, true)
	defer resp.Body.Close()
	var info TransferInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil || !info.Available || !info.Paired {
		t.Errorf("info = %+v, %v", info, err)
	}
}

func TestTheNumberGoesToTheApp(t *testing.T) {
	d, store, c, restarted := transferDaemon(t, true)
	resp := call(t, c, http.MethodPost, "/api/transferencia/enviar", nil, true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enviar: %s", resp.Status)
	}
	if d.Supervisor().Status().State != "paused" {
		t.Errorf("sync is %q while the data goes out", d.Supervisor().Status().State)
	}
	local := t.TempDir()
	staging, _ := transfer.NewStaging(filepath.Join(local, "wacli"), filepath.Join(local, "data"))
	if err := transfer.Unpack(resp.Body, staging, nil); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	resp = call(t, c, http.MethodPost, "/api/transferencia/concluir", bytes.NewReader([]byte("{}")), true)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("concluir: %s", resp.Status)
	}
	<-restarted
	d.Stop()
	if _, err := os.Stat(filepath.Join(store, "session.db")); err == nil {
		t.Error("the server kept the session in place")
	}
	matches, _ := filepath.Glob(store + ".levado-*")
	if len(matches) != 1 {
		t.Errorf("the session was not set aside: %v", matches)
	}
}

func TestTheNumberComesFromTheApp(t *testing.T) {
	d, store, c, restarted := transferDaemon(t, false)
	resp := call(t, c, http.MethodGet, "/api/transferencia", nil, true)
	var info TransferInfo
	_ = json.NewDecoder(resp.Body).Decode(&info)
	resp.Body.Close()
	if info.Paired {
		t.Fatal("the server says it is paired")
	}

	from := t.TempDir()
	for _, name := range []string{"AUTHED", "session.db"} {
		_ = os.WriteFile(filepath.Join(from, name), []byte("from the app"), 0o600)
	}
	var buf bytes.Buffer
	if err := transfer.Pack(context.Background(), &buf, from, t.TempDir(), nil); err != nil {
		t.Fatal(err)
	}
	resp = call(t, c, http.MethodPost, "/api/transferencia/receber", &buf, true)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("receber: %s", resp.Status)
	}
	<-restarted
	d.Stop()
	got, err := os.ReadFile(filepath.Join(store, "session.db"))
	if err != nil || string(got) != "from the app" {
		t.Errorf("session.db = %q, %v", got, err)
	}
}

func TestAPairedServerRefusesTheAppsNumber(t *testing.T) {
	d, _, c, _ := transferDaemon(t, true)
	defer d.Stop()
	from := t.TempDir()
	_ = os.WriteFile(filepath.Join(from, "session.db"), []byte("x"), 0o600)
	var buf bytes.Buffer
	_ = transfer.Pack(context.Background(), &buf, from, t.TempDir(), nil)
	resp := call(t, c, http.MethodPost, "/api/transferencia/receber", &buf, true)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("receber on a paired server: %s", resp.Status)
	}
}
