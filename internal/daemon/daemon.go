// Package daemon assembles the gateway: wacli's supervisor, the index, the
// state, transcription, the MCP server, the control panel and the HTTP
// listener. The desktop app and the command line both run it, the app inside
// its own process and the command line as `whatsapp-mcp serve`.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/remote"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/httpserver"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/localasr"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/mcp"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/panel"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/wacli"
)

type Config struct {
	Port int
	// RequirePort makes Start fail when the port is taken, as the command
	// line does: a second daemon must not fight the first one for the
	// WhatsApp session. The app keeps WhatsApp running and reports the port.
	RequirePort bool
	Token       string
	WacliBin    string
	StoreDir    string
	DataDir     string
	LogPath     string
	Version     string
	Logger      *slog.Logger
	// Desktop is what Claude Desktop runs to reach this daemon.
	Desktop panel.DesktopCommand
	// Panel adjusts the control panel before it is served, for the app to
	// plug in what only it can do.
	Panel func(*panel.Panel)
	// RemoteListen, when set (host:port), serves the app's remote access
	// there (internal/remote): two routes, for an HTTPS proxy on a private
	// network to reach. Everything else stays on the loopback port.
	RemoteListen string
}

// Daemon is a running gateway.
type Daemon struct {
	cfg    Config
	logger *slog.Logger
	cancel context.CancelFunc

	// primary is the main number, the one the panel and the tray show.
	// accounts holds it first, then any further numbers.
	primary *Account
	panel   *panel.Panel
	routes  *http.ServeMux

	watchDone chan struct{}

	mu        sync.Mutex
	accounts  []*Account
	port      int
	srv       *http.Server
	listenErr *PortProblem

	remote    *remote.Server
	remoteSrv *http.Server

	watch watchers
}

// ErrWacliMissing means the wacli program could not be found or run.
var ErrWacliMissing = errors.New("wacli not found")

// PortProblem says why the MCP is not answering: its port is taken.
type PortProblem = panel.PortProblem

// Start brings the gateway up. With RequirePort it fails when the port is
// taken; otherwise WhatsApp starts anyway and PortProblem says why the MCP
// is off.
func Start(cfg Config) (*Daemon, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Version != "" {
		mcp.Version = cfg.Version
	}
	if _, err := os.Stat(cfg.WacliBin); err != nil {
		if _, lookErr := exec.LookPath(cfg.WacliBin); lookErr != nil {
			return nil, fmt.Errorf("%w (%s)", ErrWacliMissing, cfg.WacliBin)
		}
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("cannot create %s: %w", cfg.DataDir, err)
	}

	// Claim the port before starting sync, so the command line fails here
	// rather than after it has taken the store.
	ln, err := listen(cfg.Port)
	var problem *PortProblem
	if err != nil {
		problem = describePortProblem(cfg.Port)
		if cfg.RequirePort {
			return nil, fmt.Errorf("cannot listen on %s: %w (is another WhatsApp MCP already running?)", addr(cfg.Port), err)
		}
		logger.Warn("the MCP port is taken; WhatsApp runs, the MCP waits for another port", "port", cfg.Port, "error", err)
	}

	d := &Daemon{cfg: cfg, logger: logger, port: cfg.Port, listenErr: problem, watchDone: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel

	// Transcription models and programs are shared by every number.
	asr := localasr.New(cfg.DataDir)
	primary, err := d.startAccount(ctx, AccountSpec{StoreDir: cfg.StoreDir, DataDir: cfg.DataDir}, asr)
	if err != nil {
		cancel()
		closeQuietly(ln)
		return nil, err
	}
	d.primary, d.accounts = primary, []*Account{primary}

	d.panel = &panel.Panel{Server: primary.server, Supervisor: primary.sup, Index: primary.idx, State: primary.st, Token: cfg.Token,
		Endpoint: d.MCPURL, Desktop: cfg.Desktop, LogPath: cfg.LogPath, MCPProblem: d.PortProblem, DataDir: cfg.DataDir}
	if cfg.Panel != nil {
		cfg.Panel(d.panel)
	}
	register := func(mux *http.ServeMux) {
		d.panel.Register(mux)
		d.panel.RegisterWebhooks(mux, primary.hooks, primary.webhooksOn)
	}
	d.routes = httpserver.Routes(primary.server, httpserver.Options{Token: cfg.Token, Logger: logger, Register: register})

	if ln != nil {
		d.serve(ln)
		logger.Info("serving MCP", "url", d.MCPURL(), "wacli", cfg.WacliBin, "store", cfg.StoreDir, "token", cfg.Token != "")
	}
	go d.watchLoop(ctx)
	if err := d.startRemote(cfg, primary); err != nil {
		d.Stop()
		return nil, err
	}
	return d, nil
}

// startRemote prepares remote access: the server's key and the paired
// devices, always (so a code can be made before the listener is up), and the
// listener when RemoteListen is set.
func (d *Daemon) startRemote(cfg Config, primary *Account) error {
	srv, err := remote.NewServer(cfg.DataDir, primary.st, d.logger)
	if err != nil {
		return err
	}
	// A device's requests reach the daemon as the app's own window reaches
	// it: the device proved itself in the handshake. The loopback guard is
	// for the network, which these requests did not come through.
	local := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Host = addr(d.Port())
			if cfg.Token != "" {
				r.Header.Set("Authorization", "Bearer "+cfg.Token)
			}
			next.ServeHTTP(w, r)
		})
	}
	srv.Admin = local(panel.Internal(d.routes))
	srv.Tools = local(d.routes)
	d.remote = srv
	d.panel.Remote = srv
	if cfg.RemoteListen == "" {
		return nil
	}
	ln, err := net.Listen("tcp", cfg.RemoteListen)
	if err != nil {
		return fmt.Errorf("cannot listen for remote access on %s: %w", cfg.RemoteListen, err)
	}
	hs := httpserver.NewServer(srv.Handler())
	d.mu.Lock()
	d.remoteSrv = hs
	d.mu.Unlock()
	go func() {
		if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			d.logger.Error("the remote listener stopped", "error", err)
		}
	}()
	d.logger.Info("serving remote access", "listen", cfg.RemoteListen, "server_key", remote.Fingerprint(srv.PublicKey()))
	return nil
}

// Remote is the daemon's remote access.
func (d *Daemon) Remote() *remote.Server { return d.remote }

func addr(port int) string    { return net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) }
func baseURL(port int) string { return "http://" + addr(port) }

func listen(port int) (net.Listener, error) { return net.Listen("tcp", addr(port)) }

func closeQuietly(ln net.Listener) {
	if ln != nil {
		_ = ln.Close()
	}
}

func (d *Daemon) serve(ln net.Listener) {
	srv := httpserver.NewServer(httpserver.Guard(d.routes, d.logger))
	d.mu.Lock()
	d.srv = srv
	d.mu.Unlock()
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			d.logger.Error("the MCP listener stopped", "error", err)
		}
	}()
}

// Stop ends the gateway with care: a pairing in progress is cancelled, sync
// is interrupted and given up to 20 seconds to close its store, the MCP stops
// answering, and the databases are closed.
func (d *Daemon) Stop() {
	d.cancel()
	accounts := d.Accounts()
	var wg sync.WaitGroup
	for _, a := range accounts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.cancel()
			<-a.syncDone
		}()
	}
	wg.Wait()
	<-d.watchDone
	d.mu.Lock()
	srv, rsrv := d.srv, d.remoteSrv
	d.srv, d.remoteSrv = nil, nil
	d.mu.Unlock()
	if srv != nil {
		httpserver.Shutdown(srv)
	}
	if rsrv != nil {
		httpserver.Shutdown(rsrv)
	}
	for _, a := range accounts {
		a.stop()
	}
}

// Accounts are the numbers the daemon runs, the main one first.
func (d *Daemon) Accounts() []*Account {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]*Account(nil), d.accounts...)
}

// Primary is the main number.
func (d *Daemon) Primary() *Account { return d.primary }

func (d *Daemon) accountCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.accounts)
}

// Port is the port the MCP is configured for.
func (d *Daemon) Port() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.port
}

// MCPURL is the MCP's address.
func (d *Daemon) MCPURL() string { return baseURL(d.Port()) + "/mcp" }

// PortProblem is why the MCP is not answering, or nil when it is.
func (d *Daemon) PortProblem() *PortProblem {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.listenErr
}

// HTTPHandler is what the loopback listener serves, guard included.
func (d *Daemon) HTTPHandler() http.Handler { return httpserver.Guard(d.routes, d.logger) }

// AppHandler is what the app's window loads, in memory: the same routes,
// trusted as the panel's own page.
func (d *Daemon) AppHandler() http.Handler { return panel.Internal(d.routes) }

// Supervisor is the main number's wacli supervisor, for the app's own
// lifecycle.
func (d *Daemon) Supervisor() *wacli.Supervisor { return d.primary.sup }

// Panel is the control panel.
func (d *Daemon) Panel() *panel.Panel { return d.panel }

// ErrPortInUse means another program listens on the port asked for.
type ErrPortInUse struct{ Port int }

func (e ErrPortInUse) Error() string {
	return i18n.Tf("a porta %d já está em uso por outro programa", e.Port)
}

// SetPort moves the MCP to another port without touching WhatsApp: the new
// listener opens before the old one closes, so the MCP is never left with
// none, and when the new port is taken the old one keeps answering.
func (d *Daemon) SetPort(port int) error {
	if port < 1024 || port > 65535 {
		return errors.New(i18n.T("a porta precisa ser um número de 1024 a 65535"))
	}
	d.mu.Lock()
	same := port == d.port && d.srv != nil
	d.mu.Unlock()
	if same {
		return nil
	}
	ln, err := listen(port)
	if err != nil {
		return ErrPortInUse{Port: port}
	}
	d.mu.Lock()
	old := d.srv
	d.port, d.srv, d.listenErr = port, nil, nil
	d.mu.Unlock()
	for _, a := range d.Accounts() {
		a.server.SetBaseURL(baseURL(port))
	}
	d.serve(ln)
	if old != nil {
		go httpserver.Shutdown(old)
	}
	d.logger.Info("MCP moved to another port", "url", d.MCPURL())
	d.watch.publish(d.status(context.Background()))
	return nil
}

// FreePortNear finds a port that can be listened on, starting after port.
func FreePortNear(port int) int {
	for p := port + 1; p < port+200 && p <= 65535; p++ {
		if ln, err := listen(p); err == nil {
			ln.Close()
			return p
		}
	}
	return 0
}

// describePortProblem says who holds the port: another WhatsApp MCP (the
// command-line service, typically) or some other program, and which port is
// free instead.
func describePortProblem(port int) *PortProblem {
	p := &PortProblem{Port: port, Suggest: FreePortNear(port)}
	client := &http.Client{Timeout: 2 * time.Second}
	if resp, err := client.Get(baseURL(port) + "/healthz"); err == nil {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 16))
		resp.Body.Close()
		p.OtherGateway = resp.StatusCode == http.StatusOK && strings.TrimSpace(string(body)) == "ok"
	}
	return p
}
