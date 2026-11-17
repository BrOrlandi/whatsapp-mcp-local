package main

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/appconfig"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/public"
)

// serverEnv is the server mode's settings as serve reads them: each
// WHATSAPP_MCP_* variable, or else what `server setup` kept in config.json.
func serverEnv(dataDir string) func(string) string {
	var saved *appconfig.Server
	if c, err := appconfig.Load(dataDir); err == nil {
		saved = c.Server
	}
	return func(key string) string {
		if v := os.Getenv(key); v != "" || saved == nil {
			return v
		}
		switch key {
		case public.EnvURL:
			return saved.PublicURL
		case public.EnvListen:
			return saved.Listen
		case public.EnvTrustedProxy:
			return saved.TrustedProxy
		}
		return ""
	}
}

// installServiceFn is installService; tests replace it, so they never
// install a login service on the computer they run on.
var installServiceFn = installService

// askLine asks one question in a terminal; from a pipe, it gives up.
var askLine = func(question string) string {
	if !isTerminal(os.Stdin) {
		return ""
	}
	fmt.Fprint(os.Stderr, question)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line)
}

// serverCmd sets up the server mode on a computer without Docker: the
// public address, the panel's password, where the public listener answers
// and which proxy is believed, kept in the data folder for the service.
func serverCmd(cfg config, args []string) error {
	f := newFlags("server")
	publicURL := f.String("url", "", "")
	listen := f.String("listen", "", "")
	trusted := f.String("trusted-proxy", "", "")
	noService := f.Bool("no-service", false, "")
	newPassword := f.Bool("new-password", false, "")
	rest, err := f.parse(args)
	if err != nil {
		return err
	}
	action := "status"
	if len(rest) > 0 {
		action = rest[0]
	}
	saved, _ := appconfig.Load(cfg.DataDir)
	switch action {
	case "status":
		pub, err := public.FromEnv(serverEnv(cfg.DataDir), cfg.DataDir)
		out := map[string]any{"enabled": pub != nil}
		if pub != nil {
			out["public_url"], out["listen"] = pub.URL.String(), pub.Listen
		}
		if err != nil {
			out["error"] = err.Error()
		}
		if jsonOutput {
			printJSON(out)
			return nil
		}
		switch {
		case err != nil:
			say("%s", i18n.Tf("O modo servidor está pela metade: %v", err))
		case pub == nil:
			say("%s", i18n.T("O modo servidor está desligado. Para ligar: whatsapp-mcp server setup"))
		default:
			say("%s", i18n.Tf("Modo servidor ligado: %s, atendendo em %s.", pub.URL.String(), pub.Listen))
		}
		return nil
	case "setup":
	case "off":
		if inContainer() {
			return fail(exitConflict, "%s", i18n.T("no Docker, o modo servidor vem do .env e do compose.yml; mude lá"))
		}
		saved.Server = nil
		if err := appconfig.Save(cfg.DataDir, saved); err != nil {
			return err
		}
		say("%s", i18n.T("Modo servidor desligado: só a escuta local continua."))
		if *noService {
			return nil
		}
		return applyToDaemon(cfg)
	default:
		return fail(exitUsage, "%s", i18n.T("uso: whatsapp-mcp server [status|setup|off]"))
	}

	if inContainer() {
		return fail(exitConflict, "%s", i18n.T("no Docker, o modo servidor vem do .env e do compose.yml (install-docker.sh)"))
	}
	if *publicURL == "" && saved.Server != nil {
		*publicURL = saved.Server.PublicURL
	}
	if *publicURL == "" {
		*publicURL = askLine(i18n.T("Endereço público do servidor (como https://whatsapp.example.com): "))
	}
	if *publicURL == "" {
		return fail(exitUsage, "%s", i18n.T("diga o endereço público: whatsapp-mcp server setup --url https://whatsapp.example.com"))
	}
	server := appconfig.Server{PublicURL: strings.TrimRight(strings.TrimSpace(*publicURL), "/"), Listen: *listen, TrustedProxy: *trusted}
	if server.Listen == "" && saved.Server != nil {
		server.Listen = saved.Server.Listen
	}
	if server.TrustedProxy == "" && saved.Server != nil {
		server.TrustedProxy = saved.Server.TrustedProxy
	}
	// The password: a new one when there is none yet or when asked for.
	if os.Getenv(public.EnvPasswordHash) == "" {
		if _, err := os.Stat(public.PasswordPath(cfg.DataDir)); err != nil || *newPassword {
			password, err := readPassword()
			if err != nil {
				return err
			}
			hash, err := public.HashPassword(password)
			if err != nil {
				return fail(exitUsage, "%v", err)
			}
			if err := writePasswordHash(cfg.DataDir, hash); err != nil {
				return err
			}
		}
	}
	// Checked as serve will check it, before anything is kept.
	env := func(key string) string {
		switch key {
		case public.EnvURL:
			return server.PublicURL
		case public.EnvListen:
			return server.Listen
		case public.EnvTrustedProxy:
			return server.TrustedProxy
		}
		return os.Getenv(key)
	}
	pub, err := public.FromEnv(env, cfg.DataDir)
	if err != nil {
		return fail(exitUsage, "%v", err)
	}
	saved.Server = &server
	if err := appconfig.Save(cfg.DataDir, saved); err != nil {
		return err
	}
	if !*noService {
		if _, err := installServiceFn(cfg); err != nil {
			return err
		}
		if err := waitHealthy(cfg.baseURL()+"/healthz", 20*time.Second); err != nil {
			return fail(exitNotRunning, "%s", i18n.Tf("o serviço foi instalado, mas ainda não responde: %v (whatsapp-mcp logs)", err))
		}
	}
	if jsonOutput {
		printJSON(map[string]any{"enabled": true, "public_url": pub.URL.String(), "listen": pub.Listen})
		return nil
	}
	say("%s", i18n.Tf("Modo servidor ligado: %s", pub.URL.String()))
	say("%s", i18n.Tf("A escuta pública atende em %s. Aponte para ela o seu proxy ou túnel com HTTPS (docs/servidor.md).", pub.Listen))
	if *noService {
		say("%s", i18n.T("O serviço não foi mexido: rode whatsapp-mcp service install, ou reinicie o whatsapp-mcp serve, para valer."))
	}
	return nil
}

// healthcheckCmd says whether the daemon answers on this computer, for a
// container's healthcheck: exit 0 when it does.
func healthcheckCmd(cfg config, _ []string) error {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(cfg.baseURL() + "/healthz")
	if err != nil {
		return fail(exitNotRunning, "%v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fail(exitNotRunning, "%s", resp.Status)
	}
	return nil
}
