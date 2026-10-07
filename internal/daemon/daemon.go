// Package daemon assembles the gateway: wacli's supervisor, the index, the
// state, transcription, the MCP server, the control panel and the HTTP
// listener. The desktop app and the command line both run it, the app inside
// its own process and the command line as `whatsapp-mcp serve`.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/httpserver"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/index"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/localasr"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/mcp"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/panel"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
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
}

// Daemon is a running gateway.
type Daemon struct {
	cfg    Config
	logger *slog.Logger
	cancel context.CancelFunc

	cli    *wacli.CLI
	idx    *index.Index
	st     *state.State
	sup    *wacli.Supervisor
	server *mcp.Server
	panel  *panel.Panel
	routes *http.ServeMux

	syncDone  chan struct{}
	watchDone chan struct{}

	mu        sync.Mutex
	port      int
	srv       *http.Server
	listenErr *PortProblem

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
	for _, dir := range []string{cfg.StoreDir, cfg.DataDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("cannot create %s: %w", dir, err)
		}
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

	d := &Daemon{cfg: cfg, logger: logger, port: cfg.Port, listenErr: problem,
		syncDone: make(chan struct{}), watchDone: make(chan struct{})}
	d.cli = &wacli.CLI{Bin: cfg.WacliBin, StoreDir: cfg.StoreDir}
	if d.idx, err = index.Open(filepath.Join(cfg.StoreDir, "wacli.db")); err != nil {
		closeQuietly(ln)
		return nil, err
	}
	if d.st, err = state.Open(cfg.DataDir); err != nil {
		d.idx.Close()
		closeQuietly(ln)
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	d.sup = wacli.NewSupervisor(d.cli, logger)
	go func() {
		d.sup.Run(ctx)
		close(d.syncDone)
	}()

	asr := localasr.New(cfg.DataDir)
	d.server = mcp.New(mcp.Config{CLI: d.cli, Supervisor: d.sup, Index: d.idx, State: d.st, Logger: logger,
		BaseURL: baseURL(cfg.Port), MediaDir: filepath.Join(cfg.DataDir, "media"), ExportDir: filepath.Join(cfg.DataDir, "exports"), ASR: asr})
	d.panel = &panel.Panel{Server: d.server, Supervisor: d.sup, Index: d.idx, State: d.st, Token: cfg.Token,
		Endpoint: d.MCPURL, Desktop: cfg.Desktop, LogPath: cfg.LogPath, MCPProblem: d.PortProblem}
	if cfg.Panel != nil {
		cfg.Panel(d.panel)
	}
	d.routes = httpserver.Routes(d.server, httpserver.Options{Token: cfg.Token, Logger: logger, Register: d.panel.Register})

	if ln != nil {
		d.serve(ln)
		logger.Info("serving MCP", "url", d.MCPURL(), "wacli", cfg.WacliBin, "store", cfg.StoreDir, "token", cfg.Token != "")
	}
	go d.watchLoop(ctx)
	return d, nil
}

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
	<-d.syncDone
	<-d.watchDone
	d.mu.Lock()
	srv := d.srv
	d.srv = nil
	d.mu.Unlock()
	if srv != nil {
		httpserver.Shutdown(srv)
	}
	d.st.Close()
	d.idx.Close()
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

// Supervisor is wacli's supervisor, for the app's own lifecycle.
func (d *Daemon) Supervisor() *wacli.Supervisor { return d.sup }

// Panel is the control panel.
func (d *Daemon) Panel() *panel.Panel { return d.panel }

// ErrPortInUse means another program listens on the port asked for.
type ErrPortInUse struct{ Port int }

func (e ErrPortInUse) Error() string {
	return fmt.Sprintf("a porta %d já está em uso por outro programa", e.Port)
}

// SetPort moves the MCP to another port without touching WhatsApp: the new
// listener opens before the old one closes, so the MCP is never left with
// none, and when the new port is taken the old one keeps answering.
func (d *Daemon) SetPort(port int) error {
	if port < 1024 || port > 65535 {
		return fmt.Errorf("a porta precisa ser um número de 1024 a 65535")
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
	d.server.SetBaseURL(baseURL(port))
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
