package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/panel"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/platform"
)

var fakeWacli string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakewacli-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fakeWacli = filepath.Join(dir, platform.ExeName("wacli"))
	if out, err := exec.Command("go", "build", "-o", fakeWacli, "../wacli/testdata/fakewacli").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building the fake wacli: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// startDaemon runs a gateway over the fake wacli, on a paired store.
func startDaemon(t *testing.T, port int, requirePort bool) (*Daemon, string) {
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
	if err := os.MkdirAll(store, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "AUTHED"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := Start(Config{Port: port, RequirePort: requirePort, WacliBin: fakeWacli, StoreDir: store, DataDir: filepath.Join(root, "data")})
	if err != nil {
		t.Fatal(err)
	}
	return d, store
}

func get(port int, path string) (int, string, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port) + path)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), nil
}

func waitSync(t *testing.T, d *Daemon, want string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if d.Supervisor().Status().State == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("sync never reached %q: %+v", want, d.Supervisor().Status())
}

func TestDaemonServesAndStopsWithoutLeavingProcesses(t *testing.T) {
	port := freePort(t)
	d, store := startDaemon(t, port, true)
	waitSync(t, d, "connected")

	if code, body, err := get(port, "/healthz"); err != nil || code != 200 || strings.TrimSpace(body) != "ok" {
		t.Fatalf("/healthz = %d %q %v", code, body, err)
	}
	code, body, err := get(port, "/health")
	if err != nil {
		t.Fatal(err)
	}
	var health struct {
		Checks []struct{ Name, Status string }
	}
	if err := json.Unmarshal([]byte(body), &health); err != nil || len(health.Checks) == 0 {
		t.Fatalf("/health answered %d %q", code, body)
	}
	if st := d.Status(); st.Tone != "ok" || st.Title != "Conectado" {
		t.Fatalf("status of a connected gateway: %+v", st)
	}

	// The sync process holds the lock; it must be gone once the daemon stops.
	var syncPID int
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && syncPID == 0; {
		syncPID = lockPID(store)
		time.Sleep(20 * time.Millisecond)
	}
	if syncPID == 0 {
		t.Fatal("sync never wrote its pid into the lock")
	}
	d.Stop()
	if platform.Alive(syncPID) {
		t.Fatalf("wacli sync (pid %d) outlived the daemon", syncPID)
	}
	if _, _, err := get(port, "/healthz"); err == nil {
		t.Fatal("the MCP still answers after Stop")
	}
}

func lockPID(store string) int {
	raw, _ := os.ReadFile(filepath.Join(store, "LOCK"))
	v, ok := strings.CutPrefix(strings.TrimSpace(string(raw)), "pid=")
	if !ok {
		return 0
	}
	pid, _ := strconv.Atoi(v)
	return pid
}

func TestSetPortMovesTheMCPWithoutDroppingIt(t *testing.T) {
	first := freePort(t)
	d, _ := startDaemon(t, first, true)
	defer d.Stop()
	waitSync(t, d, "connected")

	// A taken port is refused, and the current one keeps answering.
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	busy := taken.Addr().(*net.TCPAddr).Port
	var inUse ErrPortInUse
	if err := d.SetPort(busy); !errors.As(err, &inUse) || inUse.Port != busy {
		t.Fatalf("a taken port must be refused, got %v", err)
	}
	if code, _, err := get(first, "/healthz"); err != nil || code != 200 {
		t.Fatalf("the current port stopped answering after a refused change: %d %v", code, err)
	}
	if err := d.SetPort(80); err == nil {
		t.Fatal("ports below 1024 must be refused")
	}

	second := freePort(t)
	if err := d.SetPort(second); err != nil {
		t.Fatal(err)
	}
	if code, _, err := get(second, "/healthz"); err != nil || code != 200 {
		t.Fatalf("the new port does not answer: %d %v", code, err)
	}
	if d.MCPURL() != "http://127.0.0.1:"+strconv.Itoa(second)+"/mcp" {
		t.Fatalf("MCPURL = %s", d.MCPURL())
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, _, err := get(first, "/healthz"); err != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("the old port kept answering after the move")
}

func TestATakenPortKeepsWhatsAppRunningAndSaysWho(t *testing.T) {
	port := freePort(t)
	other, _ := startDaemon(t, port, true)
	defer other.Stop()

	if _, err := Start(Config{Port: port, RequirePort: true, WacliBin: fakeWacli, StoreDir: t.TempDir(), DataDir: t.TempDir()}); err == nil {
		t.Fatal("the command line must refuse a taken port")
	}

	d, _ := startDaemon(t, port, false)
	defer d.Stop()
	p := d.PortProblem()
	if p == nil || p.Port != port || !p.OtherGateway || p.Suggest == 0 || p.Suggest == port {
		t.Fatalf("port problem = %+v", p)
	}
	waitSync(t, d, "connected")
	if st := d.Status(); !st.MCPDown || st.Tone != "warn" {
		t.Fatalf("status with the MCP down: %+v", st)
	}
	if err := d.SetPort(p.Suggest); err != nil {
		t.Fatal(err)
	}
	if d.PortProblem() != nil {
		t.Fatal("moving to a free port must clear the problem")
	}
	if code, _, err := get(p.Suggest, "/healthz"); err != nil || code != 200 {
		t.Fatalf("the suggested port does not answer: %d %v", code, err)
	}
}

func TestWatchFollowsSync(t *testing.T) {
	d, _ := startDaemon(t, freePort(t), true)
	defer d.Stop()
	updates, stop := d.Watch()
	defer stop()
	deadline := time.After(15 * time.Second)
	for {
		select {
		case st := <-updates:
			if st.Sync == "connected" {
				return
			}
		case <-deadline:
			t.Fatal("the watcher never saw sync connect")
		}
	}
}

type fakeHost struct{ port func(int) error }

func (fakeHost) Settings() panel.HostSettings {
	return panel.HostSettings{Autostart: true, CloseToTray: true, CanHide: true, DataDir: "/data", Version: "1.0.0"}
}
func (fakeHost) SetAutostart(bool) error   { return nil }
func (fakeHost) SetCloseToTray(bool) error { return nil }
func (h fakeHost) SetPort(p int) error     { return h.port(p) }
func (fakeHost) OpenURL(string) error      { return nil }
func (fakeHost) OpenDataFolder() error     { return nil }
func (fakeHost) Update() panel.UpdateState {
	return panel.UpdateState{State: "available", Current: "1.0.0", Latest: "1.1.0"}
}
func (fakeHost) CheckUpdate()           {}
func (fakeHost) InstallUpdate() error   { return nil }
func (fakeHost) Quit()                  {}
func (fakeHost) ShowWindow(string)      {}
func (fakeHost) EraseEverything() error { return nil }

// Every page renders inside the app, where the window loads them in memory.
func TestEveryPageRendersInTheApp(t *testing.T) {
	var d *Daemon
	base := ""
	if runtime.GOOS != "windows" {
		base = "/tmp"
	}
	root, err := os.MkdirTemp(base, "daemon-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	store := filepath.Join(root, "s")
	_ = os.MkdirAll(store, 0o700)
	_ = os.WriteFile(filepath.Join(store, "AUTHED"), nil, 0o600)
	d, err = Start(Config{Port: freePort(t), WacliBin: fakeWacli, StoreDir: store, DataDir: filepath.Join(root, "data"),
		Panel: func(p *panel.Panel) { p.Host = fakeHost{port: func(port int) error { return d.SetPort(port) }} }})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Stop()
	waitSync(t, d, "connected")
	h := d.AppHandler()
	for _, path := range []string{"/", "/instalacao", "/whatsapp", "/status", "/funcoes", "/receitas", "/configuracoes", "/configuracoes?porta=1", "/estado", "/transcricao", "/documentacao",
		"/conectar", "/conectar/claude-desktop", "/conectar/claude-code", "/conectar/codex", "/conectar/cursor", "/conectar/codex?conectado=1", "/conectar/outra", "/conectar/nada", "/ajuda"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "wails://localhost"+path, nil))
		body := rec.Body.String()
		if rec.Code != 200 || strings.Contains(body, "can't evaluate") {
			t.Errorf("%s answered %d: %.300s", path, rec.Code, body)
		}
		if strings.Contains(body, "<html") && !strings.Contains(body, "data-app") {
			t.Errorf("%s is not marked as the app's", path)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "wails://localhost/configuracoes", nil))
	if !strings.Contains(rec.Body.String(), "Configurações") || !strings.Contains(rec.Body.String(), "1.1.0") {
		t.Error("the settings page does not show the update")
	}
}
