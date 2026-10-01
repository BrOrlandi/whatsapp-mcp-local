// Command whatsapp-mcp-v2 serves WhatsApp to MCP clients on localhost, on top
// of wacli.
//
//	whatsapp-mcp-v2 serve               run the daemon (sync + MCP endpoint)
//	whatsapp-mcp-v2 bridge              stdio MCP for Claude Desktop, forwarding to the daemon
//	whatsapp-mcp-v2 service install     start the daemon at login and keep it running
//	whatsapp-mcp-v2 service uninstall
//	whatsapp-mcp-v2 service stop|start  stop and start it again, without removing it
//	whatsapp-mcp-v2 open                open the control panel
//	whatsapp-mcp-v2 config              print the client configuration
//	whatsapp-mcp-v2 version
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/bridge"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/httpserver"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/index"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/localasr"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/mcp"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/panel"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/service"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/state"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/wacli"
)

var version = "dev"

// defaultPort is uncommon on purpose: the daemon should not collide with the
// dev servers that live on 3000, 5173 or 8080.
const defaultPort = 47821

type config struct {
	Port     int
	Token    string
	WacliBin string
	StoreDir string
	DataDir  string
}

func (c config) addr() string { return net.JoinHostPort("127.0.0.1", strconv.Itoa(c.Port)) }
func (c config) baseURL() string {
	return "http://" + c.addr()
}

func load() (config, error) {
	c := config{Port: defaultPort, Token: os.Getenv("WHATSAPP_MCP_TOKEN")}
	if v := os.Getenv("WHATSAPP_MCP_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 || p > 65535 {
			return c, fmt.Errorf("WHATSAPP_MCP_PORT must be a port number, got %q", v)
		}
		c.Port = p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return c, err
	}
	c.WacliBin = os.Getenv("WACLI_BIN")
	if c.WacliBin == "" {
		c.WacliBin = findWacli()
	}
	c.StoreDir = os.Getenv("WACLI_STORE_DIR")
	if c.StoreDir == "" {
		c.StoreDir = defaultStore(home)
	}
	c.DataDir = os.Getenv("WHATSAPP_MCP_DATA")
	if c.DataDir == "" {
		c.DataDir = filepath.Join(home, ".whatsapp-mcp-v2")
	}
	return c, nil
}

// findWacli looks beyond PATH because a login service starts with a minimal
// one that does not include Homebrew.
func findWacli() string {
	if p, err := exec.LookPath("wacli"); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
		return p
	}
	for _, p := range []string{"/opt/homebrew/bin/wacli", "/usr/local/bin/wacli", "/home/linuxbrew/.linuxbrew/bin/wacli"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "wacli"
}

// defaultStore mirrors wacli's own default, so a store paired with plain
// `wacli auth` is the one the daemon uses.
func defaultStore(home string) string {
	legacy := filepath.Join(home, ".wacli")
	if runtime.GOOS != "linux" {
		return legacy
	}
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "wacli")
	}
	return filepath.Join(home, ".local", "state", "wacli")
}

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	cfg, err := load()
	if err != nil {
		fatal(err)
	}
	switch cmd {
	case "serve":
		err = serve(cfg)
	case "bridge":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		err = bridge.Run(ctx, cfg.baseURL()+"/mcp", cfg.Token, os.Stdin, os.Stdout)
	case "service":
		err = serviceCmd(cfg, os.Args[2:])
	case "config":
		printConfig(cfg)
	case "transcription":
		err = transcriptionCmd(cfg, os.Args[2:])
	case "open":
		err = openBrowser(cfg.baseURL() + "/")
	case "version", "--version", "-v":
		fmt.Println(version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fatal(err)
	}
}

const usage = `whatsapp-mcp-v2 — WhatsApp for MCP clients, on localhost, over wacli

  serve               run the daemon: supervises wacli sync and serves MCP at http://127.0.0.1:47821/mcp
  bridge              stdio MCP server for Claude Desktop that forwards to the daemon
  service install     run the daemon at login and restart it if it stops (launchd / systemd --user)
  service uninstall   remove that service
  service stop|start  stop the service (as if the computer were off) and start it again
  config              print the configuration for Claude Code and Claude Desktop
  transcription install   set up free voice-note transcription on this Mac (whisper.cpp)
  open                open the control panel in the browser
  version             print the version

Environment: WHATSAPP_MCP_PORT, WHATSAPP_MCP_TOKEN, WACLI_BIN, WACLI_STORE_DIR, WHATSAPP_MCP_DATA
`

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "whatsapp-mcp-v2:", err)
	os.Exit(1)
}

func serve(cfg config) error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	mcp.Version = version

	if _, err := os.Stat(cfg.WacliBin); err != nil {
		if _, lookErr := exec.LookPath(cfg.WacliBin); lookErr != nil {
			return fmt.Errorf("wacli not found (%s); install it with `brew install openclaw/tap/wacli` or set WACLI_BIN", cfg.WacliBin)
		}
	}
	if err := os.MkdirAll(cfg.StoreDir, 0o700); err != nil {
		return err
	}
	// Claim the port before starting sync: a second daemon must fail here
	// rather than fight the first one for the WhatsApp session.
	ln, err := net.Listen("tcp", cfg.addr())
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w (is another whatsapp-mcp-v2 already running?)", cfg.addr(), err)
	}
	ln.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cli := &wacli.CLI{Bin: cfg.WacliBin, StoreDir: cfg.StoreDir}
	idx, err := index.Open(filepath.Join(cfg.StoreDir, "wacli.db"))
	if err != nil {
		return err
	}
	defer idx.Close()
	st, err := state.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()

	supervisor := wacli.NewSupervisor(cli, logger)
	syncDone := make(chan struct{})
	go func() {
		supervisor.Run(ctx)
		close(syncDone)
	}()

	asr := localasr.New(cfg.DataDir)
	server := mcp.New(mcp.Config{CLI: cli, Supervisor: supervisor, Index: idx, State: st, Logger: logger,
		BaseURL: cfg.baseURL(), MediaDir: filepath.Join(cfg.DataDir, "media"), ASR: asr})
	go server.RunAutoTranscription(ctx)
	control := &panel.Panel{Server: server, Supervisor: supervisor, Index: idx, State: st, MCPURL: cfg.baseURL() + "/mcp", Token: cfg.Token,
		Binary: executable(), Port: cfg.Port, DefaultPort: defaultPort}
	handler := httpserver.Handler(server, httpserver.Options{Addr: cfg.addr(), Token: cfg.Token, Logger: logger, Register: control.Register})
	logger.Info("serving MCP", "url", cfg.baseURL()+"/mcp", "panel", cfg.baseURL()+"/", "wacli", cfg.WacliBin, "store", cfg.StoreDir, "token", cfg.Token != "")
	fmt.Fprintf(os.Stderr, "\n  Painel: %s/\n  MCP:    %s/mcp\n\n", cfg.baseURL(), cfg.baseURL())
	err = httpserver.Serve(ctx, handler, cfg.addr())
	stop()
	<-syncDone
	return err
}

func serviceCmd(cfg config, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: whatsapp-mcp-v2 service install|uninstall|stop|start")
	}
	switch args[0] {
	case "install":
		bin := executable()
		env := map[string]string{"WACLI_BIN": cfg.WacliBin, "WACLI_STORE_DIR": cfg.StoreDir,
			"WHATSAPP_MCP_DATA": cfg.DataDir, "WHATSAPP_MCP_PORT": strconv.Itoa(cfg.Port)}
		if cfg.Token != "" {
			env["WHATSAPP_MCP_TOKEN"] = cfg.Token
		}
		home, _ := os.UserHomeDir()
		path, err := service.Install(service.Spec{Binary: bin, Args: []string{"serve"}, Env: env,
			LogDir: filepath.Join(home, "Library", "Logs")})
		if err != nil {
			return err
		}
		fmt.Printf("installed %s\nthe daemon now runs at login and restarts if it stops\n\n  Painel: %s/\n  MCP:    %s/mcp\n\n", path, cfg.baseURL(), cfg.baseURL())
		if len(args) > 1 && args[1] == "--no-open" {
			return nil
		}
		if err := waitHealthy(cfg.baseURL()+"/healthz", 15*time.Second); err != nil {
			return fmt.Errorf("the service was installed but is not answering yet: %w (see ~/Library/Logs/whatsapp-mcp-v2.log)", err)
		}
		fmt.Println("opening the panel in the browser: connect your WhatsApp there")
		return openBrowser(cfg.baseURL() + "/")
	case "uninstall":
		if err := service.Uninstall(); err != nil {
			return err
		}
		fmt.Println("service removed")
		return nil
	case "stop":
		if err := service.Stop(); err != nil {
			return err
		}
		fmt.Println("service stopped: WhatsApp is not being received until `whatsapp-mcp-v2 service start`")
		return nil
	case "start":
		if err := service.Start(); err != nil {
			return err
		}
		fmt.Printf("service started; panel at %s/\n", cfg.baseURL())
		return nil
	}
	return fmt.Errorf("unknown service command %q", args[0])
}

// executable is this program's resolved path, which the service and the
// Desktop configuration point at.
func executable() string {
	bin, err := os.Executable()
	if err != nil {
		return "whatsapp-mcp-v2"
	}
	if resolved, err := filepath.EvalSymlinks(bin); err == nil {
		return resolved
	}
	return bin
}

func openBrowser(url string) error {
	name, args := "xdg-open", []string{url}
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	}
	if err := exec.Command(name, args...).Start(); err != nil {
		fmt.Printf("open %s in the browser\n", url)
	}
	return nil
}

func waitHealthy(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			last = fmt.Errorf("answered %s", resp.Status)
		} else {
			last = err
		}
		time.Sleep(300 * time.Millisecond)
	}
	return last
}

func printConfig(cfg config) {
	bin := executable()
	url := cfg.baseURL() + "/mcp"
	fmt.Println("Claude Code:")
	if cfg.Token != "" {
		fmt.Printf("  claude mcp add --scope user --transport http whatsapp %s --header \"Authorization: Bearer %s\"\n\n", url, cfg.Token)
	} else {
		fmt.Printf("  claude mcp add --scope user --transport http whatsapp %s\n\n", url)
	}
	server := map[string]any{"command": bin, "args": []string{"bridge"}}
	env := map[string]string{}
	if cfg.Port != defaultPort {
		env["WHATSAPP_MCP_PORT"] = strconv.Itoa(cfg.Port)
	}
	if cfg.Token != "" {
		env["WHATSAPP_MCP_TOKEN"] = cfg.Token
	}
	if len(env) > 0 {
		server["env"] = env
	}
	body, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{"whatsapp": server}}, "  ", "  ")
	fmt.Println("Claude Desktop (Chat and Cowork) — ~/Library/Application Support/Claude/claude_desktop_config.json:")
	fmt.Println("  " + string(body))
}

func transcriptionCmd(cfg config, args []string) error {
	if len(args) == 0 || args[0] != "install" {
		return errors.New("usage: whatsapp-mcp-v2 transcription install")
	}
	engine := localasr.New(cfg.DataDir)
	if st := engine.Status(); !st.Supported {
		return errors.New("local transcription needs a Mac with Apple Silicon; elsewhere, save an OpenAI key in the panel")
	} else if st.Ready {
		fmt.Println("local transcription is already installed")
		return nil
	}
	fmt.Println("setting up local transcription (whisper.cpp, about 600 MB)…")
	if err := engine.InstallNow(context.Background(), os.Stdout); err != nil {
		return err
	}
	fmt.Println("local transcription is ready: voice notes are now transcribed on this computer")
	return nil
}
