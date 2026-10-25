package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/appconfig"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/platform"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/service"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/updater"
)

// ---- status ----

// statusCmd says where things stand. Its exit code is for scripts: 0 when
// WhatsApp is ready, 4 when it is not yet (next says why), 3 when the
// daemon does not answer.
func statusCmd(cfg config, args []string) error {
	f := newFlags("status")
	if _, err := f.parse(args); err != nil {
		return err
	}
	st, raw, err := newAPI(cfg).status(context.Background())
	if err != nil {
		return err
	}
	if jsonOutput {
		fmt.Fprintln(stdout, string(raw))
	} else {
		printStatus(st)
	}
	if st.Next != "ready" {
		return exitError{exitNotReady, errSilent}
	}
	return nil
}

// errSilent ends the program with its exit code and no message: the output
// has already said it all.
var errSilent = errors.New("")

func printStatus(st status) {
	kind := i18n.T("linha de comando")
	if st.Desktop {
		kind = i18n.T("app")
	}
	row := func(label, value string) { fmt.Fprintf(stdout, "  %-12s %s\n", label, value) }
	fmt.Fprintf(stdout, "%s %s (%s)\n\n", st.App, st.Version, kind)
	switch {
	case st.WhatsApp.Connected:
		row(i18n.T("WhatsApp"), i18n.Tf("conectado: %s", who(st)))
	case st.WhatsApp.Paired:
		row(i18n.T("WhatsApp"), i18n.Tf("conectado a %s, mas sem receber agora (%s)", who(st), st.WhatsApp.State))
	default:
		row(i18n.T("WhatsApp"), i18n.T("nenhum número conectado"))
	}
	if st.WhatsApp.Label != "" {
		row(i18n.T("Nome"), st.WhatsApp.Label)
	}
	row(i18n.T("Saúde"), st.Health.Status+": "+st.Health.Summary)
	row("MCP", st.MCPURL)
	row(i18n.T("A seguir"), nextText(st))
	// The checks matter when sync is stuck; before pairing, the next step
	// above already says what to do.
	for _, c := range st.Health.Checks {
		if c.Status == "ok" || st.Next != "error" {
			continue
		}
		fmt.Fprintf(stdout, "\n  [%s] %s: %s\n", c.Status, c.Name, c.Detail)
		if c.Fix != "" {
			fmt.Fprintf(stdout, "         %s\n", c.Fix)
		}
	}
}

func nextText(st status) string {
	switch st.Next {
	case "ready":
		return i18n.T("pronto: o WhatsApp está conectado e as ferramentas de IA já podem usar o MCP")
	case "pair":
		return i18n.T("conecte o WhatsApp: whatsapp-mcp pair")
	case "scan":
		if st.Pairing.Code != "" {
			return i18n.Tf("digite o código %s no celular (Conectar com número de telefone)", st.Pairing.Code)
		}
		return i18n.T("esperando o celular ler o QR code: whatsapp-mcp pair mostra o código aqui")
	case "syncing":
		return i18n.T("conectado; o celular está mandando o histórico, o que leva alguns minutos")
	case "wait":
		return i18n.T("abrindo ou reconectando; pergunte de novo em alguns segundos")
	}
	return i18n.T("a sincronização parou; veja o que falhou abaixo")
}

// ---- logout ----

func logoutCmd(cfg config, args []string) error {
	f := newFlags("logout")
	yes := f.Bool("yes", false, "")
	if _, err := f.parse(args); err != nil {
		return err
	}
	a := newAPI(cfg)
	st, _, err := a.status(context.Background())
	if err != nil {
		return err
	}
	if !st.WhatsApp.Paired {
		if jsonOutput {
			printJSON(map[string]any{"ok": true, "paired": false})
		}
		say("%s", i18n.T("Nenhum WhatsApp está conectado a este computador."))
		return nil
	}
	if !*yes {
		if jsonOutput || !isTerminal(os.Stdin) {
			return fail(exitUsage, "%s", i18n.T("desconectar pede confirmação: rode de novo com --yes"))
		}
		fmt.Fprintf(stdout, "%s [s/N] ", i18n.Tf("Desconectar o WhatsApp de %s deste computador? Para voltar, será preciso conectar de novo.", who(st)))
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "s", "sim", "y", "yes", "si", "sí":
		default:
			say("%s", i18n.T("Nada mudou."))
			return nil
		}
	}
	if err := a.call(context.Background(), http.MethodPost, "/api/logout", nil, nil); err != nil {
		return err
	}
	if jsonOutput {
		printJSON(map[string]any{"ok": true, "paired": false})
	}
	say("%s", i18n.T("WhatsApp desconectado. Para conectar de novo: whatsapp-mcp pair"))
	return nil
}

// ---- port ----

func portCmd(cfg config, args []string) error {
	f := newFlags("port")
	rest, err := f.parse(args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		if jsonOutput {
			printJSON(map[string]any{"port": cfg.Port, "source": cfg.PortSource, "mcp_url": cfg.baseURL() + "/mcp"})
			return nil
		}
		say("%d", cfg.Port)
		return nil
	}
	port, err := strconv.Atoi(rest[0])
	if err != nil || port < 1024 || port > 65535 || len(rest) > 1 {
		return fail(exitUsage, "%s", i18n.T("a porta é um número de 1024 a 65535"))
	}
	if cfg.PortSource == "env" {
		return fail(exitConflict, "%s", i18n.T("a porta está definida pela variável de ambiente WHATSAPP_MCP_PORT; mude a variável"))
	}
	if port != cfg.Port {
		ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err != nil {
			return fail(exitConflict, "%s", i18n.Tf("a porta %d já está em uso por outro programa", port))
		}
		ln.Close()
		c, _ := appconfig.Load(cfg.DataDir)
		c.Port = port
		if err := appconfig.Save(cfg.DataDir, c); err != nil {
			return err
		}
		cfg.Port, cfg.PortSource = port, "config"
		if err := applyToDaemon(cfg); err != nil {
			return err
		}
	}
	if jsonOutput {
		printJSON(map[string]any{"port": cfg.Port, "source": cfg.PortSource, "mcp_url": cfg.baseURL() + "/mcp"})
		return nil
	}
	say("%s", i18n.Tf("O MCP responde em %s/mcp.", cfg.baseURL()))
	say("%s", i18n.T("As ferramentas de IA ligadas ao endereço antigo precisam do novo: whatsapp-mcp config mostra como configurar cada uma."))
	return nil
}

// applyToDaemon restarts the daemon for a changed setting (the port, the
// token) to take effect: the login service is written again, which also
// drops a setting an older version wrote into it. In Docker and under
// `serve`, the person restarts it.
func applyToDaemon(cfg config) error {
	switch {
	case inContainer():
		say("%s", i18n.T("Reinicie o contêiner para valer (docker compose restart)."))
		return nil
	case !serviceInstalled():
		say("%s", i18n.T("Se o whatsapp-mcp serve estiver rodando, reinicie-o para valer."))
		return nil
	}
	if _, err := installService(cfg); err != nil {
		return err
	}
	if err := waitHealthy(cfg.baseURL()+"/healthz", 20*time.Second); err != nil {
		return fail(exitNotRunning, "%s", i18n.Tf("o serviço foi reiniciado, mas ainda não responde: %v (whatsapp-mcp logs)", err))
	}
	say("%s", i18n.T("Serviço reiniciado."))
	return nil
}

// serviceInstalled is service.Installed; tests replace it, so they never
// rewrite the login service of the computer they run on.
var serviceInstalled = service.Installed

// ---- token ----

// tokenCmd sets the key that /mcp, /health and the API's actions ask for.
// It lives in the data folder (token, readable only by this user), unless
// WHATSAPP_MCP_TOKEN sets it.
func tokenCmd(cfg config, args []string) error {
	f := newFlags("token")
	rest, err := f.parse(args)
	if err != nil {
		return err
	}
	action := "status"
	if len(rest) > 0 {
		action = rest[0]
	}
	if action != "status" && action != "show" && cfg.TokenSource == "env" {
		return fail(exitConflict, "%s", i18n.T("o token está definido pela variável de ambiente WHATSAPP_MCP_TOKEN; mude a variável"))
	}
	report := func(reveal bool) {
		out := map[string]any{"enabled": cfg.Token != "", "source": cfg.TokenSource}
		if reveal && cfg.Token != "" {
			out["token"] = cfg.Token
		}
		if jsonOutput {
			printJSON(out)
			return
		}
		switch {
		case cfg.Token == "":
			say("%s", i18n.T("Sem token: qualquer programa deste computador usa o MCP. Para pedir um: whatsapp-mcp token new"))
		case reveal:
			say("%s", cfg.Token)
		default:
			say("%s", i18n.Tf("Com token (%s…): o MCP, o /health e as ações da API pedem Authorization: Bearer <token>. whatsapp-mcp token show mostra o token.", cfg.Token[:min(4, len(cfg.Token))]))
		}
	}
	switch action {
	case "status":
		report(false)
		return nil
	case "show":
		report(true)
		return nil
	case "new", "set":
		token := ""
		if action == "new" {
			b := make([]byte, 24)
			_, _ = rand.Read(b)
			token = hex.EncodeToString(b)
		} else {
			if len(rest) != 2 {
				return fail(exitUsage, "%s", i18n.T("uso: whatsapp-mcp token set <token>, ou - para ler da entrada"))
			}
			token = rest[1]
			if token == "-" {
				line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
				token = line
			}
			token = strings.TrimSpace(token)
			if len(token) < 16 || strings.ContainsAny(token, " \t\r\n") {
				return fail(exitUsage, "%s", i18n.T("o token precisa ter ao menos 16 caracteres, sem espaços"))
			}
		}
		if err := writeToken(cfg.DataDir, token); err != nil {
			return err
		}
		cfg.Token, cfg.TokenSource = token, "file"
	case "clear":
		if err := os.Remove(tokenPath(cfg.DataDir)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		cfg.Token, cfg.TokenSource = "", ""
	default:
		return fail(exitUsage, "%s", i18n.T("uso: whatsapp-mcp token [show|new|set <token>|clear]"))
	}
	if err := applyToDaemon(cfg); err != nil {
		return err
	}
	report(action == "new")
	say("%s", i18n.T("As ferramentas de IA precisam da mudança: whatsapp-mcp config mostra como configurar cada uma."))
	return nil
}

func tokenPath(dataDir string) string { return filepath.Join(dataDir, "token") }

func readToken(dataDir string) string {
	raw, err := os.ReadFile(tokenPath(dataDir))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func writeToken(dataDir, token string) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	tmp := tokenPath(dataDir) + ".new"
	if err := os.WriteFile(tmp, []byte(token+"\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, tokenPath(dataDir))
}

// ---- webhooks ----

type webhookInfo struct {
	ID             string   `json:"id"`
	URL            string   `json:"url"`
	Events         []string `json:"events"`
	IncludeOwn     bool     `json:"include_own"`
	Chats          []string `json:"chats"`
	Enabled        bool     `json:"enabled"`
	DisabledReason string   `json:"disabled_reason"`
	LastResult     string   `json:"last_result"`
	Delivered      int64    `json:"delivered"`
	Queued         int      `json:"queued"`
}

func webhooksCmd(cfg config, args []string) error {
	f := newFlags("webhooks")
	events := f.String("events", "", "")
	chats := f.String("chats", "", "")
	includeOwn := f.Bool("include-own", false, "")
	rest, err := f.parse(args)
	if err != nil {
		return err
	}
	action := "list"
	if len(rest) > 0 {
		action, rest = rest[0], rest[1:]
	}
	a := newAPI(cfg)
	ctx := context.Background()
	needID := func() (string, error) {
		if len(rest) != 1 {
			return "", fail(exitUsage, "%s", i18n.Tf("uso: whatsapp-mcp webhooks %s <id>", action))
		}
		return url.PathEscape(rest[0]), nil
	}
	switch action {
	case "list":
		var out struct {
			Available bool          `json:"available"`
			Webhooks  []webhookInfo `json:"webhooks"`
		}
		if err := a.call(ctx, http.MethodGet, "/api/webhooks", nil, &out); err != nil {
			return err
		}
		if jsonOutput {
			printJSON(out)
			return nil
		}
		if len(out.Webhooks) == 0 {
			say("%s", i18n.T("Nenhum webhook. Para criar: whatsapp-mcp webhooks add <url>"))
		}
		for _, h := range out.Webhooks {
			printWebhook(h)
		}
		if !out.Available {
			say("\n%s", i18n.T("o wacli em uso não entrega mensagens novas a webhooks; atualize-o para que este webhook receba algo"))
		}
		return nil
	case "add":
		if len(rest) != 1 {
			return fail(exitUsage, "%s", i18n.T("uso: whatsapp-mcp webhooks add <url> [--events message,reaction,receipt] [--include-own] [--chats <jid>,<jid>]"))
		}
		body := map[string]any{"url": rest[0], "include_own": *includeOwn}
		if *events != "" {
			body["events"] = splitList(*events)
		}
		if *chats != "" {
			body["chats"] = splitList(*chats)
		}
		var out struct {
			Webhook webhookInfo `json:"webhook"`
			Secret  string      `json:"secret"`
			Warning string      `json:"warning"`
		}
		var raw json.RawMessage
		if err := a.call(ctx, http.MethodPost, "/api/webhooks", body, &raw); err != nil {
			return err
		}
		if jsonOutput {
			fmt.Fprintln(stdout, string(raw))
			return nil
		}
		_ = json.Unmarshal(raw, &out)
		printWebhook(out.Webhook)
		say("\n%s\n  %s", i18n.T("O segredo que assina cada entrega (cabeçalho X-WhatsApp-MCP-Signature). Guarde: ele não aparece de novo."), out.Secret)
		if out.Warning != "" {
			say("\n%s", out.Warning)
		}
		return nil
	case "remove", "rm", "delete":
		id, err := needID()
		if err != nil {
			return err
		}
		if err := a.call(ctx, http.MethodDelete, "/api/webhooks/"+id, nil, nil); err != nil {
			return err
		}
		if jsonOutput {
			printJSON(map[string]bool{"deleted": true})
		}
		say("%s", i18n.T("Webhook apagado."))
		return nil
	case "enable", "disable":
		id, err := needID()
		if err != nil {
			return err
		}
		var h webhookInfo
		if err := a.call(ctx, http.MethodPatch, "/api/webhooks/"+id, map[string]bool{"enabled": action == "enable"}, &h); err != nil {
			return err
		}
		if jsonOutput {
			printJSON(h)
			return nil
		}
		printWebhook(h)
		return nil
	case "test":
		id, err := needID()
		if err != nil {
			return err
		}
		var out struct {
			OK     bool   `json:"ok"`
			Result string `json:"result"`
			MS     int64  `json:"ms"`
			Error  string `json:"error"`
		}
		if err := a.call(ctx, http.MethodPost, "/api/webhooks/"+id+"/test", nil, &out); err != nil {
			return err
		}
		if jsonOutput {
			printJSON(out)
		} else if out.OK {
			say("%s", i18n.Tf("Entregue em %d ms: %s", out.MS, out.Result))
		} else {
			say("%s", i18n.Tf("Não foi entregue: %s", firstNonEmpty(out.Error, out.Result)))
		}
		if !out.OK {
			return exitError{exitFailed, errSilent}
		}
		return nil
	}
	return fail(exitUsage, "%s", i18n.T("uso: whatsapp-mcp webhooks [list|add <url>|remove <id>|enable <id>|disable <id>|test <id>]"))
}

func printWebhook(h webhookInfo) {
	state := i18n.T("ligado")
	if !h.Enabled {
		state = i18n.T("desligado")
		if h.DisabledReason != "" {
			state += ": " + h.DisabledReason
		}
	}
	say("%s  %s  [%s]", h.ID, h.URL, state)
	detail := strings.Join(h.Events, ", ")
	if h.IncludeOwn {
		detail += " · " + i18n.T("inclui as próprias mensagens")
	}
	if len(h.Chats) > 0 {
		detail += " · " + i18n.Tf("só %d conversas", len(h.Chats))
	}
	say("    %s · %s", detail, i18n.Tf("%d entregues", h.Delivered))
	if h.LastResult != "" {
		say("    %s", h.LastResult)
	}
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// ---- logs ----

func logsCmd(cfg config, args []string) error {
	f := newFlags("logs")
	follow := f.Bool("f", false, "")
	f.BoolVar(follow, "follow", false, "")
	lines := f.Int("n", 100, "")
	if _, err := f.parse(args); err != nil {
		return err
	}
	if *lines < 0 {
		return fail(exitUsage, "logs: %s", i18n.T("-n é um número de linhas, 0 ou mais"))
	}
	switch {
	case inContainer():
		return fail(exitConflict, "%s", i18n.T("no Docker, o registro sai na saída do contêiner: docker compose logs -f"))
	case runtime.GOOS == "linux":
		args := []string{"--user", "-u", "whatsapp-mcp.service", "--no-pager", "-n", strconv.Itoa(*lines)}
		if *follow {
			args = append(args, "-f")
		}
		cmd := exec.Command("journalctl", args...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fail(exitFailed, "journalctl: %v", err)
		}
		return nil
	case runtime.GOOS == "darwin":
		return tail(logPath(), *lines, *follow)
	}
	return fail(exitConflict, "%s", i18n.T("sem serviço neste sistema, o registro sai no terminal do whatsapp-mcp serve"))
}

// tail prints the last n lines of a file and, with follow, what is written
// to it afterwards.
func tail(path string, n int, follow bool) error {
	f, err := os.Open(path)
	if err != nil {
		return fail(exitFailed, "%s", i18n.Tf("o registro ainda não existe (%s): o serviço está instalado?", path))
	}
	defer f.Close()
	// The last 1 MB holds far more than the lines asked for.
	if fi, err := f.Stat(); err == nil && fi.Size() > 1<<20 {
		_, _ = f.Seek(-1<<20, io.SeekEnd)
	}
	raw, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	all := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(all) > n {
		all = all[len(all)-n:]
	}
	if len(all) > 0 && all[0] != "" {
		fmt.Fprintln(stdout, strings.Join(all, "\n"))
	}
	for follow {
		time.Sleep(500 * time.Millisecond)
		if _, err := io.Copy(stdout, f); err != nil {
			return err
		}
	}
	return nil
}

// ---- update ----

func updateCmd(cfg config, args []string) error {
	f := newFlags("update")
	check := f.Bool("check", false, "")
	if _, err := f.parse(args); err != nil {
		return err
	}
	if inContainer() {
		return fail(exitConflict, "%s", i18n.T("no Docker, a atualização é a da imagem: docker compose build --pull && docker compose up -d (docs/servidor.md)"))
	}
	u := &updater.Updater{Repo: "BrOrlandi/whatsapp-mcp-local", Version: version,
		Dir: filepath.Join(cfg.DataDir, "updates"), Target: updater.Target{Kind: "cli", Path: platform.Executable()}}
	if !u.Enabled() {
		return fail(exitConflict, "%s", i18n.Tf("esta versão (%s) não se atualiza sozinha: instale uma versão publicada com o install.sh", version))
	}
	ctx := context.Background()
	found, err := u.Check(ctx)
	if err != nil {
		return err
	}
	st := u.State()
	out := map[string]any{"current": strings.TrimPrefix(version, "v"), "latest": st.Latest, "available": found}
	if !found {
		if jsonOutput {
			printJSON(out)
		}
		say("%s", i18n.Tf("Já está na versão mais recente (%s).", strings.TrimPrefix(version, "v")))
		return nil
	}
	if *check {
		if jsonOutput {
			printJSON(out)
		}
		say("%s", i18n.Tf("A versão %s está disponível (esta é a %s). Para instalar: whatsapp-mcp update", st.Latest, strings.TrimPrefix(version, "v")))
		return nil
	}
	say("%s", i18n.Tf("Baixando a versão %s…", st.Latest))
	if err := u.Apply(ctx); err != nil {
		return err
	}
	out["updated"] = true
	if serviceInstalled() {
		if err := service.Restart(); err != nil {
			return err
		}
		if err := waitHealthy(cfg.baseURL()+"/healthz", 20*time.Second); err != nil {
			return fail(exitNotRunning, "%s", i18n.Tf("o serviço foi reiniciado, mas ainda não responde: %v (whatsapp-mcp logs)", err))
		}
		say("%s", i18n.T("Serviço reiniciado."))
	} else {
		say("%s", i18n.T("Se o whatsapp-mcp serve estiver rodando, reinicie-o para valer."))
	}
	if jsonOutput {
		printJSON(out)
	}
	say("%s", i18n.Tf("Atualizado para a versão %s.", st.Latest))
	return nil
}

// ---- remote access ----

// remoteCmd manages the devices that use this daemon from another computer
// (internal/remote, docs/servidor.md).
func remoteCmd(cfg config, args []string) error {
	f := newFlags("remote")
	name := f.String("name", "", "")
	scope := f.String("scope", "admin", "")
	rest, err := f.parse(args)
	if err != nil {
		return err
	}
	action := "devices"
	if len(rest) > 0 {
		action, rest = rest[0], rest[1:]
	}
	a := newAPI(cfg)
	ctx := context.Background()
	switch action {
	case "code":
		var code struct {
			Code        string    `json:"code"`
			Scope       string    `json:"scope"`
			ExpiresAt   time.Time `json:"expires_at"`
			Fingerprint string    `json:"server_fingerprint"`
		}
		var raw json.RawMessage
		if err := a.call(ctx, http.MethodPost, "/api/remote/codigo", map[string]string{"name": *name, "scope": *scope}, &raw); err != nil {
			return err
		}
		if jsonOutput {
			fmt.Fprintln(stdout, string(raw))
			return nil
		}
		_ = json.Unmarshal(raw, &code)
		say("%s\n\n  %s\n", i18n.T("Código de pareamento (vale uma vez, por 10 minutos; não aparece de novo):"), code.Code)
		say("%s %s", i18n.T("Impressão digital do servidor:"), code.Fingerprint)
		say("%s", i18n.T("No app do outro computador: Configurações › Acesso remoto › Usar um servidor, com o endereço https:// deste servidor e o código."))
		return nil
	case "devices":
		var out struct {
			Devices []struct {
				ID        string     `json:"id"`
				Name      string     `json:"name"`
				Scope     string     `json:"scope"`
				LastSeen  *time.Time `json:"last_seen_at"`
				RevokedAt *time.Time `json:"revoked_at"`
			} `json:"devices"`
			Fingerprint string `json:"server_fingerprint"`
		}
		var raw json.RawMessage
		if err := a.call(ctx, http.MethodGet, "/api/remote/dispositivos", nil, &raw); err != nil {
			return err
		}
		if jsonOutput {
			fmt.Fprintln(stdout, string(raw))
			return nil
		}
		_ = json.Unmarshal(raw, &out)
		if len(out.Devices) == 0 {
			say("%s", i18n.T("Nenhum dispositivo pareado. Para parear o app de outro computador: whatsapp-mcp remote code"))
		}
		for _, d := range out.Devices {
			state := i18n.T("nunca conectado")
			switch {
			case d.RevokedAt != nil:
				state = i18n.T("revogado")
			case d.LastSeen != nil:
				state = i18n.Tf("visto em %s", d.LastSeen.Local().Format("2006-01-02 15:04"))
			}
			say("%s  %-24s %-6s %s", d.ID, d.Name, d.Scope, state)
		}
		if out.Fingerprint != "" {
			say("\n%s %s", i18n.T("Impressão digital do servidor:"), out.Fingerprint)
		}
		return nil
	case "revoke":
		if len(rest) != 1 {
			return fail(exitUsage, "usage: whatsapp-mcp remote revoke <id>")
		}
		if err := a.call(ctx, http.MethodDelete, "/api/remote/dispositivos/"+url.PathEscape(rest[0]), nil, nil); err != nil {
			return err
		}
		if jsonOutput {
			printJSON(map[string]bool{"revoked": true})
		}
		say("%s", i18n.T("Dispositivo revogado: ele perde o acesso na hora e precisa ser pareado de novo."))
		return nil
	}
	return fail(exitUsage, "usage: whatsapp-mcp remote [devices|code [--name N] [--scope admin|tools]|revoke <id>]")
}

// ---- API keys ----

// apikeyCmd manages the keys agents on other machines use the MCP by the
// server's address with (internal/remote, docs/servidor.md).
func apikeyCmd(cfg config, args []string) error {
	f := newFlags("apikey")
	rest, err := f.parse(args)
	if err != nil {
		return err
	}
	action := "list"
	if len(rest) > 0 {
		action, rest = rest[0], rest[1:]
	}
	a := newAPI(cfg)
	ctx := context.Background()
	switch action {
	case "list":
		var out struct {
			Keys []struct {
				ID      string    `json:"id"`
				Name    string    `json:"name"`
				Created time.Time `json:"created_at"`
			} `json:"keys"`
		}
		var raw json.RawMessage
		if err := a.call(ctx, http.MethodGet, "/api/chaves", nil, &raw); err != nil {
			return err
		}
		if jsonOutput {
			fmt.Fprintln(stdout, string(raw))
			return nil
		}
		_ = json.Unmarshal(raw, &out)
		if len(out.Keys) == 0 {
			say("%s", i18n.T("Nenhuma chave de API: o MCP pelo endereço do servidor está desligado. Para criar uma: whatsapp-mcp apikey new <nome>"))
		}
		for _, k := range out.Keys {
			created := ""
			if !k.Created.IsZero() {
				created = k.Created.Local().Format("2006-01-02 15:04")
			}
			say("%s  %-24s %s", k.ID, k.Name, created)
		}
		return nil
	case "new":
		if len(rest) == 0 {
			return fail(exitUsage, "usage: whatsapp-mcp apikey new <name>")
		}
		var out struct {
			Key  string `json:"key"`
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		var raw json.RawMessage
		if err := a.call(ctx, http.MethodPost, "/api/chaves", map[string]string{"name": strings.Join(rest, " ")}, &raw); err != nil {
			return err
		}
		if jsonOutput {
			fmt.Fprintln(stdout, string(raw))
			return nil
		}
		_ = json.Unmarshal(raw, &out)
		say("%s\n\n  %s\n", i18n.T("Chave de API (aparece uma vez só; guarde no lugar de segredos do agente):"), out.Key)
		say("%s", i18n.T("O agente usa o MCP em https://<endereço do servidor>/mcp, com o cabeçalho Authorization: Bearer <chave>."))
		return nil
	case "revoke":
		if len(rest) != 1 {
			return fail(exitUsage, "usage: whatsapp-mcp apikey revoke <id>")
		}
		if err := a.call(ctx, http.MethodDelete, "/api/chaves/"+url.PathEscape(rest[0]), nil, nil); err != nil {
			return err
		}
		if jsonOutput {
			printJSON(map[string]bool{"revoked": true})
		}
		say("%s", i18n.T("Chave revogada: o agente que a usava perde o acesso na hora."))
		return nil
	}
	return fail(exitUsage, "usage: whatsapp-mcp apikey [list|new <name>|revoke <id>]")
}
