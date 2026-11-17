package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/bridge"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/public"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

// EnvAPIKey is where bridge --url takes the server's API key from.
const EnvAPIKey = "WHATSAPP_MCP_API_KEY"

// apikeyCmd makes, lists and revokes the keys agents on other computers use
// to reach the MCP at the server's public address. It works on the data
// folder directly, with the service running or not; a revoked key stops at
// once.
func apikeyCmd(cfg config, args []string) error {
	f := newFlags("apikey")
	name := f.String("name", "", "")
	readOnly := f.Bool("read-only", false, "")
	rest, err := f.parse(args)
	if err != nil {
		return err
	}
	action := "list"
	if len(rest) > 0 {
		action = rest[0]
	}
	st, err := state.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	ctx, now := context.Background(), time.Now()
	switch action {
	case "list":
		keys, err := public.Keys(ctx, st, now)
		if err != nil {
			return err
		}
		if jsonOutput {
			printJSON(map[string]any{"keys": keys})
			return nil
		}
		if len(keys) == 0 {
			say("%s", i18n.T("Nenhuma chave. Para criar: whatsapp-mcp apikey create --name <nome>"))
			return nil
		}
		for _, k := range keys {
			access := i18n.T("tudo")
			if k.ReadOnly {
				access = i18n.T("só ler")
			}
			used := i18n.T("ainda não usada")
			switch {
			case k.Expired:
				used = i18n.T("expirou: ninguém usou nas primeiras 24 horas")
			case k.LastUsedAt != nil:
				used = i18n.Tf("usada em %s", k.LastUsedAt.Local().Format("2006-01-02 15:04"))
			}
			say("%s  %s  (%s; %s; %s)", k.ID, k.Name, access, i18n.Tf("criada em %s", k.CreatedAt.Local().Format("2006-01-02 15:04")), used)
		}
		return nil
	case "create", "new":
		if len(rest) > 1 && *name == "" {
			*name = strings.Join(rest[1:], " ")
		}
		key, k, err := public.CreateKey(ctx, st, *name, *readOnly, now)
		if err != nil {
			return fail(exitUsage, "%v", err)
		}
		address := ""
		if u := strings.TrimRight(strings.TrimSpace(os.Getenv(public.EnvURL)), "/"); u != "" {
			address = u + "/mcp"
		}
		if jsonOutput {
			printJSON(map[string]any{"id": k.ID, "name": k.Name, "read_only": k.ReadOnly, "key": key, "mcp_url": address})
			return nil
		}
		say("%s", key)
		say("")
		say("%s", i18n.T("Esta é a chave: ela não aparece de novo. Se ninguém a usar em 24 horas, ela deixa de valer."))
		if address != "" {
			say("%s", i18n.Tf("Endereço do MCP: %s, com o cabeçalho Authorization: Bearer <chave>.", address))
		}
		return nil
	case "revoke", "rm", "delete":
		if len(rest) != 2 {
			return fail(exitUsage, "%s", i18n.T("uso: whatsapp-mcp apikey revoke <id>"))
		}
		if err := public.RevokeKey(ctx, st, rest[1], now); errors.Is(err, state.ErrNoAccess) {
			return fail(exitUsage, "%s", i18n.Tf("nenhuma chave ativa tem o id %s (whatsapp-mcp apikey list)", rest[1]))
		} else if err != nil {
			return err
		}
		if jsonOutput {
			printJSON(map[string]bool{"ok": true})
			return nil
		}
		say("%s", i18n.T("Chave revogada: parou de valer agora."))
		return nil
	}
	return fail(exitUsage, "%s", i18n.T("uso: whatsapp-mcp apikey [list|create --name <nome> [--read-only]|revoke <id>]"))
}

// bridgeOptions is where the bridge forwards to: the local daemon, or, with
// --url, a server's public address with an API key taken from
// WHATSAPP_MCP_API_KEY or --key-file (never an argument, which other users
// of the computer can see).
func bridgeOptions(cfg config, args []string) (bridge.Options, error) {
	f := newFlags("bridge")
	remote := f.String("url", "", "")
	keyFile := f.String("key-file", "", "")
	if _, err := f.parse(args); err != nil {
		return bridge.Options{}, err
	}
	if *remote == "" {
		if *keyFile != "" {
			return bridge.Options{}, fail(exitUsage, "%s", i18n.T("--key-file vale só com --url"))
		}
		return bridge.Options{
			URL:        func() string { return cfg.baseURL() + "/mcp" },
			Token:      cfg.Token,
			NotRunning: "the WhatsApp MCP daemon is not running; start it with `whatsapp-mcp service install` or `whatsapp-mcp serve`",
		}, nil
	}
	u, err := url.Parse(*remote)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && loopback(u.Hostname()))) {
		return bridge.Options{}, fail(exitUsage, "%s", i18n.T("--url precisa ser o endereço https do MCP no servidor, como https://whatsapp.exemplo.com/mcp"))
	}
	if !strings.HasSuffix(u.Path, "/mcp") {
		u.Path = strings.TrimRight(u.Path, "/") + "/mcp"
	}
	key := strings.TrimSpace(os.Getenv(EnvAPIKey))
	if *keyFile != "" {
		raw, err := os.ReadFile(*keyFile)
		if err != nil {
			return bridge.Options{}, fail(exitUsage, "%s", i18n.Tf("não foi possível ler a chave em %s: %v", *keyFile, err))
		}
		key = strings.TrimSpace(string(raw))
	}
	if key == "" {
		return bridge.Options{}, fail(exitUsage, "%s", i18n.Tf("--url precisa da chave do servidor: defina %s ou use --key-file", EnvAPIKey))
	}
	address := u.String()
	return bridge.Options{
		URL:        func() string { return address },
		Token:      key,
		NotRunning: fmt.Sprintf("the WhatsApp MCP server at %s does not answer; check the address and that the server is up", address),
	}, nil
}

func loopback(h string) bool {
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
