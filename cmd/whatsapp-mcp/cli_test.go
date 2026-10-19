package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/appconfig"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/daemon"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/platform"
)

// fakeWacli is internal/wacli/testdata/fakewacli, which walks through a
// pairing (a QR code, or a code with --phone) the way wacli does.
var fakeWacli string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakewacli-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fakeWacli = filepath.Join(dir, platform.ExeName("wacli"))
	if out, err := exec.Command("go", "build", "-o", fakeWacli, "../../internal/wacli/testdata/fakewacli").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building the fake wacli: %v\n%s", err, out)
		os.Exit(1)
	}
	// No test touches the login service of the computer it runs on.
	serviceInstalled = func() bool { return false }
	pollEvery = 50 * time.Millisecond
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

// startDaemon runs the daemon over the fake wacli and returns the
// configuration a command run against it would load. files are created in
// the store: AUTHED for a paired one, QR_PAUSE to hold each QR code longer.
func startDaemon(t *testing.T, files map[string]string) config {
	t.Helper()
	base := ""
	if runtime.GOOS != "windows" {
		base = "/tmp" // the store's socket path must stay short
	}
	root, err := os.MkdirTemp(base, "cli-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	cfg := config{Port: freePort(t), PortSource: "default", WacliBin: fakeWacli,
		StoreDir: filepath.Join(root, "s"), DataDir: filepath.Join(root, "data")}
	if err := os.MkdirAll(cfg.StoreDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(cfg.StoreDir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	d, err := daemon.Start(daemon.Config{Port: cfg.Port, RequirePort: true, WacliBin: cfg.WacliBin,
		StoreDir: cfg.StoreDir, DataDir: cfg.DataDir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Stop)
	return cfg
}

// run runs a command the way main does and returns what it printed.
func run(t *testing.T, cmd func(config, []string) error, cfg config, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	stdout = &out
	defer func() { stdout = os.Stdout; jsonOutput = false }()
	err := cmd(cfg, args)
	return out.String(), err
}

// events reads the JSON lines of pair --json.
func events(t *testing.T, out string) []pairEvent {
	t.Helper()
	var evs []pairEvent
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var ev pairEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("not a JSON line: %q", line)
		}
		evs = append(evs, ev)
	}
	return evs
}

// waitReady polls status until WhatsApp is ready, as a script would.
func waitReady(t *testing.T, cfg config) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := run(t, statusCmd, cfg, "--json"); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	out, err := run(t, statusCmd, cfg, "--json")
	t.Fatalf("never ready: %v\n%s", err, out)
}

func TestPairWithAQRCodeFollowsItAsItChanges(t *testing.T) {
	cfg := startDaemon(t, map[string]string{"QR_PAUSE": "600"})
	out, err := run(t, pairCmd, cfg, "--json")
	if err != nil {
		t.Fatalf("pair: %v\n%s", err, out)
	}
	evs := events(t, out)
	var qrs []string
	for _, ev := range evs {
		if ev.Event == "qr" {
			qrs = append(qrs, ev.QR)
			if !strings.HasSuffix(ev.QRPNG, "/api/pair/qr.png") || ev.QRVersion == 0 {
				t.Errorf("a QR event without its image or version: %+v", ev)
			}
		}
	}
	if strings.Join(qrs, " ") != "2@first 2@second" {
		t.Fatalf("QR codes shown: %q, want both, in order\n%s", qrs, out)
	}
	if last := evs[len(evs)-1]; last.Event != "paired" || last.Phone == "" {
		t.Fatalf("the last event is %+v, want paired with the number", last)
	}
	waitReady(t, cfg)

	// Once paired, pair says so and succeeds, so a script can run it again.
	out, err = run(t, pairCmd, cfg, "--json")
	if err != nil || events(t, out)[0].Event != "already_paired" {
		t.Fatalf("pair on a paired computer: %v\n%s", err, out)
	}
}

func TestPairDrawsTheQRCodeAndItsContent(t *testing.T) {
	cfg := startDaemon(t, map[string]string{"QR_PAUSE": "600"})
	png := filepath.Join(t.TempDir(), "qr.png")
	out, err := run(t, pairCmd, cfg, "--qr-file", png)
	if err != nil {
		t.Fatalf("pair: %v\n%s", err, out)
	}
	for _, want := range []string{"2@first", "2@second", "/api/pair/qr.png", "█"} {
		if !strings.Contains(out, want) {
			t.Errorf("the output lacks %q:\n%s", want, out)
		}
	}
	drawn, _ := renderQR("2@second", false, false)
	if !strings.Contains(out, drawn) {
		t.Error("the output does not draw the last QR code")
	}
	if raw, err := os.ReadFile(png); err != nil || !bytes.HasPrefix(raw, []byte("\x89PNG")) {
		t.Errorf("--qr-file did not leave a PNG: %v", err)
	}
	waitReady(t, cfg) // the daemon stops quickly once the pairing has handed over
}

func TestPairWithAPhoneNumberShowsTheCode(t *testing.T) {
	cfg := startDaemon(t, map[string]string{"QR_PAUSE": "300"})
	out, err := run(t, pairCmd, cfg, "--json", "--phone", "+55 11 91234-5678")
	if err != nil {
		t.Fatalf("pair --phone: %v\n%s", err, out)
	}
	evs := events(t, out)
	if evs[0].Event != "code" || evs[0].Code != "ABCD-EFGH" {
		t.Fatalf("first event %+v, want the code", evs[0])
	}
	if evs[len(evs)-1].Event != "paired" {
		t.Fatalf("pairing by code did not end paired:\n%s", out)
	}
	waitReady(t, cfg)
}

func TestPairNoWaitLeavesThePairingToAnotherProgram(t *testing.T) {
	cfg := startDaemon(t, map[string]string{"QR_PAUSE": "3000"})
	out, err := run(t, pairCmd, cfg, "--json", "--no-wait")
	if err != nil {
		t.Fatalf("pair --no-wait: %v\n%s", err, out)
	}
	if evs := events(t, out); len(evs) != 1 || evs[0].Event != "qr" || evs[0].QR != "2@first" {
		t.Fatalf("events %+v, want the first QR code only", evs)
	}
	// The pairing goes on, and the API gives the QR code's text to whoever
	// draws it next.
	st, _, err := newAPI(cfg).status(context.Background())
	if err != nil || st.Pairing.State != "qr" || st.Pairing.QR != "2@first" || st.Next != "scan" {
		t.Fatalf("status after --no-wait: %+v %v", st.Pairing, err)
	}
	var pair struct {
		State string `json:"state"`
		QR    string `json:"qr"`
		QRPNG string `json:"qr_png"`
	}
	if err := newAPI(cfg).call(context.Background(), http.MethodPost, "/api/pair", nil, &pair); err != nil || pair.QR != "2@first" || pair.QRPNG == "" {
		t.Fatalf("POST /api/pair during a pairing: %+v %v", pair, err)
	}
	_ = newAPI(cfg).call(context.Background(), http.MethodPost, "/api/pair/cancel", nil, nil)
}

func TestPairTimeoutCancelsThePairing(t *testing.T) {
	cfg := startDaemon(t, map[string]string{"QR_PAUSE": "10000"})
	out, err := run(t, pairCmd, cfg, "--json", "--timeout", "500ms")
	if exitCode(err) != exitTimeout {
		t.Fatalf("pair --timeout: %v (exit %d), want exit %d\n%s", err, exitCode(err), exitTimeout, out)
	}
	st, _, _ := newAPI(cfg).status(context.Background())
	if st.Pairing.State != "cancelled" || st.Pairing.QR != "" {
		t.Fatalf("the pairing outlived the command: %+v", st.Pairing)
	}
}

func TestStatusExitCodes(t *testing.T) {
	down := config{Port: freePort(t)}
	if _, err := run(t, statusCmd, down, "--json"); exitCode(err) != exitNotRunning {
		t.Errorf("nothing running: exit %d, want %d (%v)", exitCode(err), exitNotRunning, err)
	}
	unpaired := startDaemon(t, nil)
	out, err := run(t, statusCmd, unpaired, "--json")
	if exitCode(err) != exitNotReady {
		t.Errorf("not paired: exit %d, want %d", exitCode(err), exitNotReady)
	}
	var st status
	if json.Unmarshal([]byte(out), &st) != nil || st.Next != "pair" {
		t.Errorf("status --json is not /api/status: %s", out)
	}
	out, _ = run(t, statusCmd, unpaired)
	if !strings.Contains(out, "whatsapp-mcp pair") {
		t.Errorf("status does not say how to connect:\n%s", out)
	}
	paired := startDaemon(t, map[string]string{"AUTHED": ""})
	waitReady(t, paired)
}

func TestLogout(t *testing.T) {
	cfg := startDaemon(t, map[string]string{"AUTHED": ""})
	waitReady(t, cfg)
	// Not on a terminal, it asks for --yes rather than for an answer.
	if _, err := run(t, logoutCmd, cfg); exitCode(err) != exitUsage {
		t.Fatalf("logout without --yes: exit %d (%v)", exitCode(err), err)
	}
	out, err := run(t, logoutCmd, cfg, "--yes", "--json")
	if err != nil || !strings.Contains(out, `"paired":false`) {
		t.Fatalf("logout --yes: %v\n%s", err, out)
	}
	st, _, _ := newAPI(cfg).status(context.Background())
	if st.WhatsApp.Paired || st.Next != "pair" {
		t.Fatalf("still paired after logout: %+v next=%s", st.WhatsApp, st.Next)
	}
}

func TestWebhooksFromTheCommandLine(t *testing.T) {
	cfg := startDaemon(t, map[string]string{"AUTHED": ""})
	out, err := run(t, webhooksCmd, cfg, "add", "http://127.0.0.1:9/hook", "--events", "message,reaction", "--include-own", "--json")
	if err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}
	var created struct {
		Webhook webhookInfo `json:"webhook"`
		Secret  string      `json:"secret"`
	}
	if json.Unmarshal([]byte(out), &created) != nil || created.Secret == "" || created.Webhook.ID == "" ||
		strings.Join(created.Webhook.Events, ",") != "message,reaction" || !created.Webhook.IncludeOwn {
		t.Fatalf("add did not create what was asked: %s", out)
	}
	id := created.Webhook.ID
	if out, err := run(t, webhooksCmd, cfg, "disable", id, "--json"); err != nil || !strings.Contains(out, `"enabled":false`) {
		t.Fatalf("disable: %v\n%s", err, out)
	}
	if out, _ := run(t, webhooksCmd, cfg); !strings.Contains(out, id) || !strings.Contains(out, "http://127.0.0.1:9/hook") {
		t.Fatalf("list does not show it:\n%s", out)
	}
	if _, err := run(t, webhooksCmd, cfg, "test", id); exitCode(err) != exitFailed {
		t.Fatalf("a test to an address nobody answers: exit %d (%v)", exitCode(err), err)
	}
	if _, err := run(t, webhooksCmd, cfg, "remove", id); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, webhooksCmd, cfg, "remove", id); exitCode(err) != exitConflict {
		t.Fatalf("removing it twice: exit %d (%v)", exitCode(err), err)
	}
	if _, err := run(t, webhooksCmd, cfg, "add", "not a url"); exitCode(err) != exitConflict {
		t.Fatalf("a bad url: exit %d (%v)", exitCode(err), err)
	}
}

// The port lives in config.json, where every command finds it, unless
// WHATSAPP_MCP_PORT fixes it.
func TestPort(t *testing.T) {
	t.Setenv("WHATSAPP_MCP_PORT", "")
	data := t.TempDir()
	t.Setenv("WHATSAPP_MCP_DATA", data)
	cfg, err := load()
	if err != nil || cfg.Port != appconfig.DefaultPort || cfg.PortSource != "default" {
		t.Fatalf("a new data folder: %+v %v", cfg, err)
	}
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	taken := fmt.Sprint(busy.Addr().(*net.TCPAddr).Port)
	if _, err := run(t, portCmd, cfg, taken); exitCode(err) != exitConflict {
		t.Fatalf("a taken port: exit %d (%v)", exitCode(err), err)
	}
	if _, err := run(t, portCmd, cfg, "80"); exitCode(err) != exitUsage {
		t.Fatalf("a privileged port: exit %d (%v)", exitCode(err), err)
	}
	free := freePort(t)
	if out, err := run(t, portCmd, cfg, fmt.Sprint(free), "--json"); err != nil || !strings.Contains(out, fmt.Sprintf(`"port":%d`, free)) {
		t.Fatalf("port %d: %v\n%s", free, err, out)
	}
	if cfg, _ := load(); cfg.Port != free || cfg.PortSource != "config" {
		t.Fatalf("after port %d, load gives %d from %s", free, cfg.Port, cfg.PortSource)
	}
	t.Setenv("WHATSAPP_MCP_PORT", "48000")
	cfg, _ = load()
	if cfg.Port != 48000 {
		t.Fatalf("WHATSAPP_MCP_PORT does not win: %d", cfg.Port)
	}
	if _, err := run(t, portCmd, cfg, fmt.Sprint(freePort(t))); exitCode(err) != exitConflict {
		t.Fatalf("changing a port the environment fixes: exit %d (%v)", exitCode(err), err)
	}
}

func TestToken(t *testing.T) {
	t.Setenv("WHATSAPP_MCP_TOKEN", "")
	data := t.TempDir()
	t.Setenv("WHATSAPP_MCP_DATA", data)
	cfg, _ := load()
	out, err := run(t, tokenCmd, cfg, "new", "--json")
	var got struct {
		Token  string `json:"token"`
		Source string `json:"source"`
	}
	if err != nil || json.Unmarshal([]byte(out), &got) != nil || len(got.Token) < 32 || got.Source != "file" {
		t.Fatalf("token new: %v\n%s", err, out)
	}
	if fi, err := os.Stat(filepath.Join(data, "token")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("the token file must be readable only by its owner: %v %v", fi.Mode(), err)
	}
	if cfg, _ := load(); cfg.Token != got.Token || cfg.TokenSource != "file" {
		t.Fatalf("load does not read the token file: %+v", cfg)
	}
	if _, err := run(t, tokenCmd, cfg, "set", "short"); exitCode(err) != exitUsage {
		t.Fatalf("a short token: exit %d (%v)", exitCode(err), err)
	}
	if _, err := run(t, tokenCmd, cfg, "clear"); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := load(); cfg.Token != "" {
		t.Fatal("token clear left the token")
	}
	t.Setenv("WHATSAPP_MCP_TOKEN", "from-the-environment-0123456789")
	cfg, _ = load()
	if cfg.TokenSource != "env" {
		t.Fatalf("WHATSAPP_MCP_TOKEN does not win: %+v", cfg)
	}
	if _, err := run(t, tokenCmd, cfg, "new"); exitCode(err) != exitConflict {
		t.Fatalf("changing a token the environment fixes: exit %d (%v)", exitCode(err), err)
	}
}

// With a token, the commands present it: the API's actions refuse a
// program without it.
func TestCommandsUseTheToken(t *testing.T) {
	root, _ := os.MkdirTemp("/tmp", "cli-")
	t.Cleanup(func() { os.RemoveAll(root) })
	cfg := config{Port: freePort(t), WacliBin: fakeWacli, StoreDir: filepath.Join(root, "s"),
		DataDir: filepath.Join(root, "data"), Token: "a-token-of-sixteen-or-more", TokenSource: "file"}
	_ = os.MkdirAll(cfg.StoreDir, 0o700)
	d, err := daemon.Start(daemon.Config{Port: cfg.Port, RequirePort: true, Token: cfg.Token,
		WacliBin: cfg.WacliBin, StoreDir: cfg.StoreDir, DataDir: cfg.DataDir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Stop)
	if out, err := run(t, webhooksCmd, cfg, "add", "http://127.0.0.1:9/x", "--json"); err != nil {
		t.Fatalf("with the token: %v\n%s", err, out)
	}
	cfg.Token = ""
	if _, err := run(t, webhooksCmd, cfg, "add", "http://127.0.0.1:9/x"); err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("without the token: %v", err)
	}
}

// Flags may come after the positional arguments.
func TestFlagsAnywhere(t *testing.T) {
	f := newFlags("x")
	events := f.String("events", "", "")
	rest, err := f.parse([]string{"add", "http://a", "--events", "message", "--json"})
	if err != nil || strings.Join(rest, " ") != "add http://a" || *events != "message" || !jsonOutput {
		t.Fatalf("rest %q events %q json %v err %v", rest, *events, jsonOutput, err)
	}
	jsonOutput = false
	if _, err := newFlags("x").parse([]string{"--nope"}); exitCode(err) != exitUsage {
		t.Fatalf("an unknown flag: exit %d", exitCode(err))
	}
}
