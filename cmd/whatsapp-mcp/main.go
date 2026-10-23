// Command whatsapp-mcp is WhatsApp MCP without a window: for servers, for a
// computer that stays on, and for installing with an AI agent. The desktop app
// runs the same gateway in its own process. Everything the panel sets up can
// be done from a terminal too, over SSH: `usage` below lists the commands,
// and docs/servidor.md the deploy on a server.
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
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/localasr"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/panel"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/platform"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/service"
)

var version = "dev"

type config struct {
	Port int
	// PortSource is where the port came from: env (WHATSAPP_MCP_PORT, which
	// wins), config (config.json) or default.
	PortSource string
	Token      string
	// TokenSource is env (WHATSAPP_MCP_TOKEN), file (the token file in the
	// data folder, which `token` writes) or empty, with no token.
	TokenSource string
	WacliBin    string
	StoreDir    string
	DataDir     string
}

func (c config) baseURL() string { return "http://127.0.0.1:" + strconv.Itoa(c.Port) }

func load() (config, error) {
	c := config{Port: appconfig.DefaultPort, PortSource: "default"}
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
	if p, ok, err := appconfig.PortFromEnv(); err != nil {
		return c, err
	} else if ok {
		c.Port, c.PortSource = p, "env"
	} else if _, err := os.Stat(appconfig.Path(c.DataDir)); err == nil {
		// `port` keeps the port in config.json, where the bridge finds it too.
		if saved, err := appconfig.Load(c.DataDir); err == nil {
			c.Port, c.PortSource = saved.Port, "config"
		}
	}
	if c.Token = os.Getenv("WHATSAPP_MCP_TOKEN"); c.Token != "" {
		c.TokenSource = "env"
	} else if c.Token = readToken(c.DataDir); c.Token != "" {
		c.TokenSource = "file"
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
		fatal(fail(exitUsage, "%v", err))
	}
	i18n.Set(language(cfg.DataDir))
	args := []string{}
	if len(os.Args) > 2 {
		args = os.Args[2:]
	}
	switch cmd {
	case "serve":
		err = serve(cfg)
	case "bridge":
		if len(args) > 0 && args[0] == "--remote" {
			err = bridgeRemote(cfg)
			break
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		err = bridge.Run(ctx, bridge.Options{
			URL:        func() string { return cfg.baseURL() + "/mcp" },
			Token:      cfg.Token,
			NotRunning: "the WhatsApp MCP daemon is not running; start it with `whatsapp-mcp service install` or `whatsapp-mcp serve`",
		}, os.Stdin, os.Stdout)
	case "service":
		err = serviceCmd(cfg, args)
	case "status":
		err = statusCmd(cfg, args)
	case "pair", "connect":
		err = pairCmd(cfg, args)
	case "logout", "disconnect":
		err = logoutCmd(cfg, args)
	case "port":
		err = portCmd(cfg, args)
	case "token":
		err = tokenCmd(cfg, args)
	case "webhooks", "webhook":
		err = webhooksCmd(cfg, args)
	case "logs":
		err = logsCmd(cfg, args)
	case "update":
		err = updateCmd(cfg, args)
	case "remote":
		err = remoteCmd(cfg, args)
	case "config":
		printConfig(cfg)
	case "transcription":
		err = transcriptionCmd(cfg, args)
	case "open":
		if headless() {
			err = fail(exitConflict, "%s", i18n.Tf("sem tela para abrir o navegador; o painel fica em %s/ (por um túnel SSH, por exemplo), e tudo o que ele faz tem comando: whatsapp-mcp help", cfg.baseURL()))
		} else {
			err = platform.Open(cfg.baseURL() + "/")
		}
	case "version", "--version", "-v":
		fmt.Println(version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(exitUsage)
	}
	if err != nil {
		fatal(err)
	}
}

const usage = `whatsapp-mcp — WhatsApp for MCP clients, on localhost, over wacli

Setup and service:
  serve                     run the daemon: supervises wacli sync and serves MCP at http://127.0.0.1:47821/mcp
  service install [--no-open]  run the daemon at login and restart it if it stops (launchd / systemd --user)
  service uninstall         remove that service
  service stop|start|restart  stop the service (as if the computer were off), start it, restart it
  update [--check]          install the latest release of this program and restart the service
  logs [-f] [-n 100]        show the daemon's log

WhatsApp:
  status                    where things stand and what comes next (exit 0 when ready, 4 when not yet)
  pair                      connect WhatsApp with a QR code drawn in the terminal, kept current
  pair --phone '+55 11 91234-5678'   connect with a code typed on the phone instead
      [--no-wait] [--timeout 5m] [--qr-file qr.png] [--invert]
  logout [--yes]            disconnect WhatsApp from this computer

Settings:
  port [<port>]             show or change the MCP's port (restarts the service)
  token [show|new|set <token>|clear]   the key /mcp and the API's actions ask for (restarts the service)
  webhooks [list|add <url>|remove <id>|enable <id>|disable <id>|test <id>]
      add: [--events message,reaction,receipt] [--include-own] [--chats <jid>,<jid>]
  remote [devices]          the devices (the app on another computer) that use this daemon remotely
  remote code [--name N]    a one-time pairing code for one more device (valid 10 minutes)
  remote revoke <id>        cut a device off at once
  config                    print the configuration for Claude Code and Claude Desktop
  transcription install     set up voice-note transcription on this computer (whisper.cpp)
  open                      open the control panel in the browser

  bridge                    stdio MCP server for Claude Desktop that forwards to the daemon
  bridge --remote           the same, to another daemon's MCP over its remote access (after remote pair)
  remote pair https://server <code> [--name N]   pair this command line with a server (a code from: remote code --scope tools)
  version                   print the version

status, pair, logout, port, token, webhooks, update and remote take --json: one JSON object
(pair: one per line) on stdout, errors included as {"error": "...", "exit_code": N}. Exit codes: 0 done, 1 failed,
2 wrong usage, 3 the daemon does not answer, 4 WhatsApp not ready or not connected, 5 timeout,
6 not possible here (a setting fixed by the environment, a busy port, Docker).

Environment: WHATSAPP_MCP_PORT, WHATSAPP_MCP_TOKEN, WHATSAPP_MCP_REMOTE_LISTEN (host:port for the app's remote access, behind an HTTPS proxy), WHATSAPP_MCP_REMOTE_TRUSTED_PROXY (that proxy: names, IPs or CIDRs), WHATSAPP_MCP_LANG (pt-BR, es, en), WACLI_BIN, WACLI_STORE_DIR, WHATSAPP_MCP_DATA
`

// language is the one the panel and the messages show: WHATSAPP_MCP_LANG,
// else the one kept in the data folder (appconfig.Chosen).
func language(dataDir string) string {
	if lang := os.Getenv("WHATSAPP_MCP_LANG"); lang != "" {
		return lang
	}
	return appconfig.Chosen(dataDir)
}

// fatal ends the program with err's exit code, saying why on stderr, or on
// stdout as JSON after --json.
func fatal(err error) {
	code := exitCode(err)
	if errors.Is(err, errSilent) {
		os.Exit(code)
	}
	if jsonOutput {
		printJSON(map[string]any{"error": err.Error(), "exit_code": code})
	} else {
		fmt.Fprintln(os.Stderr, "whatsapp-mcp:", err)
	}
	os.Exit(code)
}

// remoteSetting is a remote access setting: the environment's, else the
// one `service install` kept in config.json.
func remoteSetting(name, kept string) string {
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	return kept
}

func serve(cfg config) error {
	privateFiles()
	saved, _ := appconfig.Load(cfg.DataDir)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := appconfig.MarkNew(cfg.DataDir); err != nil {
		logger.Warn("could not write config.json", "error", err)
	}
	d, err := daemon.Start(daemon.Config{
		Port: cfg.Port, RequirePort: true, Token: cfg.Token,
		WacliBin: cfg.WacliBin, StoreDir: cfg.StoreDir, DataDir: cfg.DataDir,
		LogPath: logPath(), Version: version, Logger: logger,
		Desktop: desktopCommand(cfg), RemoteListen: remoteSetting("WHATSAPP_MCP_REMOTE_LISTEN", saved.RemoteListen),
		RemoteTrustedProxy: remoteSetting("WHATSAPP_MCP_REMOTE_TRUSTED_PROXY", saved.RemoteTrustedProxy),
	})
	if errors.Is(err, daemon.ErrWacliMissing) {
		return fmt.Errorf("%w; install it with `brew install openclaw/tap/wacli` or set WACLI_BIN", err)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "\n  Panel:  %s/\n  MCP:    %s/mcp\n\n", cfg.baseURL(), cfg.baseURL())
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

// keepSettings writes into config.json what the service must find there:
// the port, and the remote access settings when the environment gives them.
func keepSettings(cfg config) error {
	listen, proxy := os.LookupEnv("WHATSAPP_MCP_REMOTE_LISTEN")
	trusted, trust := os.LookupEnv("WHATSAPP_MCP_REMOTE_TRUSTED_PROXY")
	if cfg.PortSource != "config" || proxy || trust {
		c, _ := appconfig.Load(cfg.DataDir)
		if cfg.PortSource != "config" {
			c.Port = cfg.Port
		}
		// Remote access, when given, is kept too: the service does not
		// carry the shell's environment, and a later reinstall (a new port,
		// a new token) must not lose it. An empty value turns it off.
		if proxy {
			c.RemoteListen = listen
		}
		if trust {
			c.RemoteTrustedProxy = trusted
		}
		if err := appconfig.Save(cfg.DataDir, c); err != nil {
			return err
		}
	}
	return nil
}

// installService writes the login service for this program and (re)starts
// it. The port goes into config.json, where every command finds it; the
// token stays in its file, unless it came from the environment.
func installService(cfg config) (string, error) {
	if err := keepSettings(cfg); err != nil {
		return "", err
	}
	env := map[string]string{"WACLI_BIN": cfg.WacliBin, "WACLI_STORE_DIR": cfg.StoreDir,
		"WHATSAPP_MCP_DATA": cfg.DataDir}
	if cfg.TokenSource == "env" {
		env["WHATSAPP_MCP_TOKEN"] = cfg.Token
	}
	if lang := os.Getenv("WHATSAPP_MCP_LANG"); lang != "" {
		env["WHATSAPP_MCP_LANG"] = lang
	}
	home, _ := os.UserHomeDir()
	return service.Install(service.Spec{Binary: platform.Executable(), Args: []string{"serve"}, Env: env,
		LogDir: filepath.Join(home, "Library", "Logs")})
}

func serviceCmd(cfg config, args []string) error {
	if len(args) == 0 {
		return fail(exitUsage, "usage: whatsapp-mcp service install|uninstall|stop|start|restart")
	}
	if inContainer() {
		return fail(exitConflict, "%s", i18n.T("no Docker, o contêiner é o serviço: docker compose up -d, stop, restart"))
	}
	switch args[0] {
	case "install":
		path, err := installService(cfg)
		if err != nil {
			return err
		}
		fmt.Printf("installed %s\nthe daemon now runs at login and restarts if it stops\n\n  Panel:  %s/\n  MCP:    %s/mcp\n\n", path, cfg.baseURL(), cfg.baseURL())
		if !service.Lingering() {
			// On a server, a user's services end with their last SSH session
			// unless systemd is told to keep them.
			if err := service.Linger(); err != nil {
				fmt.Println(i18n.T("Atenção: sem o linger do systemd, o serviço para quando a sua última sessão SSH termina. Rode: sudo loginctl enable-linger $USER"))
			} else {
				fmt.Println(i18n.T("O serviço continua rodando depois que você sai da sessão (loginctl enable-linger)."))
			}
		}
		if err := waitHealthy(cfg.baseURL()+"/healthz", 15*time.Second); err != nil {
			return fail(exitNotRunning, "the service was installed but is not answering yet: %v (see %s)", err, logPath())
		}
		if (len(args) > 1 && args[1] == "--no-open") || headless() {
			fmt.Println(i18n.T("Para conectar o WhatsApp por aqui: whatsapp-mcp pair"))
			return nil
		}
		fmt.Println("opening the panel in the browser: connect your WhatsApp there")
		if err := platform.Open(cfg.baseURL() + "/"); err != nil {
			fmt.Printf("open %s/ in the browser\n", cfg.baseURL())
		}
		return nil
	case "restart":
		if err := service.Restart(); err != nil {
			return err
		}
		if err := waitHealthy(cfg.baseURL()+"/healthz", 20*time.Second); err != nil {
			return fail(exitNotRunning, "the service restarted but is not answering yet: %v (see %s)", err, logPath())
		}
		fmt.Printf("service restarted; panel at %s/\n", cfg.baseURL())
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
	return fail(exitUsage, "unknown service command %q", args[0])
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
