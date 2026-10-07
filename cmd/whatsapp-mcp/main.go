// Command whatsapp-mcp is WhatsApp MCP without a window: for servers, for a
// computer that stays on, and for installing with an AI agent. The desktop app
// runs the same gateway in its own process.
//
//	whatsapp-mcp serve               run the daemon (sync + MCP endpoint)
//	whatsapp-mcp bridge              stdio MCP for Claude Desktop, forwarding to the daemon
//	whatsapp-mcp service install     start the daemon at login and keep it running
//	whatsapp-mcp service uninstall
//	whatsapp-mcp service stop|start  stop and start it again, without removing it
//	whatsapp-mcp open                open the control panel
//	whatsapp-mcp config              print the client configuration
//	whatsapp-mcp version
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/appconfig"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/bridge"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/daemon"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/localasr"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/panel"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/platform"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/service"
)

var version = "dev"

type config struct {
	Port     int
	Token    string
	WacliBin string
	StoreDir string
	DataDir  string
}

func (c config) baseURL() string { return "http://127.0.0.1:" + strconv.Itoa(c.Port) }

func load() (config, error) {
	c := config{Port: appconfig.DefaultPort, Token: os.Getenv("WHATSAPP_MCP_TOKEN")}
	if p, ok, err := appconfig.PortFromEnv(); err != nil {
		return c, err
	} else if ok {
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
		c.DataDir = filepath.Join(home, ".whatsapp-mcp")
	}
	return c, nil
}

// findWacli prefers a wacli installed beside this program, then looks beyond
// PATH, because a login service starts with a minimal one.
func findWacli() string {
	if p := platform.Bundled("wacli"); p != "" {
		return p
	}
	if p := platform.LookPath("wacli"); p != "" {
		return p
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

// logPath is where the login service writes the daemon's log.
func logPath() string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Logs", "whatsapp-mcp.log")
	case "linux":
		return "journalctl --user -u whatsapp-mcp"
	}
	return ""
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
		err = bridge.Run(ctx, bridge.Options{
			URL:        func() string { return cfg.baseURL() + "/mcp" },
			Token:      cfg.Token,
			NotRunning: "the WhatsApp MCP daemon is not running; start it with `whatsapp-mcp service install` or `whatsapp-mcp serve`",
		}, os.Stdin, os.Stdout)
	case "service":
		err = serviceCmd(cfg, os.Args[2:])
	case "config":
		printConfig(cfg)
	case "transcription":
		err = transcriptionCmd(cfg, os.Args[2:])
	case "open":
		err = platform.Open(cfg.baseURL() + "/")
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

const usage = `whatsapp-mcp — WhatsApp for MCP clients, on localhost, over wacli

  serve               run the daemon: supervises wacli sync and serves MCP at http://127.0.0.1:47821/mcp
  bridge              stdio MCP server for Claude Desktop that forwards to the daemon
  service install     run the daemon at login and restart it if it stops (launchd / systemd --user)
  service uninstall   remove that service
  service stop|start  stop the service (as if the computer were off) and start it again
  config              print the configuration for Claude Code and Claude Desktop
  transcription install   set up voice-note transcription on this computer (whisper.cpp)
  open                open the control panel in the browser
  version             print the version

Environment: WHATSAPP_MCP_PORT, WHATSAPP_MCP_TOKEN, WHATSAPP_MCP_PROXY, WACLI_BIN, WACLI_STORE_DIR, WHATSAPP_MCP_DATA
`

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "whatsapp-mcp:", err)
	os.Exit(1)
}

func serve(cfg config) error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	d, err := daemon.Start(daemon.Config{
		Port: cfg.Port, RequirePort: true, Token: cfg.Token,
		WacliBin: cfg.WacliBin, StoreDir: cfg.StoreDir, DataDir: cfg.DataDir,
		LogPath: logPath(), Version: version, Logger: logger,
		Desktop: desktopCommand(cfg),
	})
	if errors.Is(err, daemon.ErrWacliMissing) {
		return fmt.Errorf("%w; install it with `brew install openclaw/tap/wacli` or set WACLI_BIN", err)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "\n  Painel: %s/\n  MCP:    %s/mcp\n\n", cfg.baseURL(), cfg.baseURL())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	d.Stop()
	return nil
}

// desktopCommand is how Claude Desktop reaches this daemon: this program's
// bridge subcommand, with the port when it is not the default.
func desktopCommand(cfg config) panel.DesktopCommand {
	c := panel.DesktopCommand{Command: platform.Executable(), Args: []string{"bridge"}}
	if c.Command == "" {
		c.Command = "whatsapp-mcp"
	}
	if cfg.Port != appconfig.DefaultPort {
		c.Env = map[string]string{"WHATSAPP_MCP_PORT": strconv.Itoa(cfg.Port)}
	}
	return c
}

func serviceCmd(cfg config, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: whatsapp-mcp service install|uninstall|stop|start")
	}
	switch args[0] {
	case "install":
		bin := platform.Executable()
		env := map[string]string{"WACLI_BIN": cfg.WacliBin, "WACLI_STORE_DIR": cfg.StoreDir,
			"WHATSAPP_MCP_DATA": cfg.DataDir, "WHATSAPP_MCP_PORT": strconv.Itoa(cfg.Port)}
		if cfg.Token != "" {
			env["WHATSAPP_MCP_TOKEN"] = cfg.Token
		}
		if proxy, ok := os.LookupEnv("WHATSAPP_MCP_PROXY"); ok {
			// Service definitions are readable by other users and systemd
			// expands percent specifiers in Environment=. Keep a proxy URL
			// (which can contain credentials) in the private config instead.
			if err := appconfig.SaveProxy(cfg.DataDir, proxy); err != nil {
				return err
			}
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
			return fmt.Errorf("the service was installed but is not answering yet: %w (see %s)", err, logPath())
		}
		fmt.Println("opening the panel in the browser: connect your WhatsApp there")
		if err := platform.Open(cfg.baseURL() + "/"); err != nil {
			fmt.Printf("open %s/ in the browser\n", cfg.baseURL())
		}
		return nil
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
		fmt.Println("service stopped: WhatsApp is not being received until `whatsapp-mcp service start`")
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
	url := cfg.baseURL() + "/mcp"
	fmt.Println("Claude Code:")
	if cfg.Token != "" {
		fmt.Printf("  claude mcp add --scope user --transport http whatsapp %s --header \"Authorization: Bearer %s\"\n\n", url, cfg.Token)
	} else {
		fmt.Printf("  claude mcp add --scope user --transport http whatsapp %s\n\n", url)
	}
	desktop := desktopCommand(cfg)
	server := map[string]any{"command": desktop.Command, "args": desktop.Args}
	env := map[string]string{}
	for k, v := range desktop.Env {
		env[k] = v
	}
	if cfg.Token != "" {
		env["WHATSAPP_MCP_TOKEN"] = cfg.Token
	}
	if len(env) > 0 {
		server["env"] = env
	}
	body, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{"whatsapp": server}}, "  ", "  ")
	fmt.Println("Claude Desktop (Chat and Cowork) — " + panel.DesktopConfigPath() + ":")
	fmt.Println("  " + string(body))
}

func transcriptionCmd(cfg config, args []string) error {
	if len(args) == 0 || args[0] != "install" {
		return errors.New("usage: whatsapp-mcp transcription install")
	}
	engine := localasr.New(cfg.DataDir)
	if st := engine.Status(); !st.Supported {
		return errors.New("local transcription is not available on this system")
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
