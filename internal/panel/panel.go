// Package panel is the local control page: pair WhatsApp by QR code, see
// whether it is healthy, and connect Claude Code and Claude Desktop to the
// daemon, without a terminal.
//
// Its actions run commands and edit client configuration on this machine, so
// every API call must come from the page itself: same-origin, never from
// another site or another localhost app open in the browser.
package panel

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/mcp"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/wacli"
)

//go:embed index.html
var page []byte

// ServerName is how this MCP appears in the clients. It differs from the
// hosted v1's "whatsapp" so both can be installed side by side.
const ServerName = "whatsapp-local"

type Panel struct {
	Server      *mcp.Server
	Supervisor  *wacli.Supervisor
	MCPURL      string
	Token       string
	Binary      string // absolute path of this program, for the Desktop bridge
	Port        int
	DefaultPort int
}

func (p *Panel) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", p.index)
	mux.HandleFunc("GET /api/state", p.api(p.state))
	mux.HandleFunc("GET /api/pair/qr.png", p.qr)
	mux.HandleFunc("POST /api/pair", p.api(p.startPairing))
	mux.HandleFunc("POST /api/pair/cancel", p.api(p.cancelPairing))
	mux.HandleFunc("POST /api/logout", p.api(p.logout))
	mux.HandleFunc("GET /api/clients", p.api(p.clients))
	mux.HandleFunc("POST /api/clients/claude-code", p.api(p.addClaudeCode))
	mux.HandleFunc("POST /api/clients/claude-desktop", p.api(p.addClaudeDesktop))
}

func (p *Panel) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; frame-ancestors 'none'")
	_, _ = w.Write(page)
}

// sameOrigin reports whether a request was made by this page. Browsers mark
// every fetch with Sec-Fetch-Site and send Origin on POST; a request from any
// other site, or from another app on another localhost port, fails one of the
// two. The custom header additionally forces a CORS preflight that nothing
// here answers.
func sameOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
		return false
	}
	if r.Method == http.MethodPost && r.Header.Get("X-Panel") != "1" {
		return false
	}
	return true
}

type apiFunc func(*http.Request) (any, error)

// userError is a refusal worth showing as it is.
type userError struct{ msg string }

func (e userError) Error() string { return e.msg }

func (p *Panel) api(f apiFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !sameOrigin(r) {
			http.Error(w, "this API answers only its own page", http.StatusForbidden)
			return
		}
		v, err := f(r)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			status := http.StatusInternalServerError
			var ue userError
			if errors.As(err, &ue) {
				status = http.StatusConflict
			}
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(v)
	}
}

func (p *Panel) state(r *http.Request) (any, error) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	account, err := p.Supervisor.Account(ctx)
	if err != nil {
		account = wacli.Account{}
	}
	sync := p.Supervisor.Status()
	sync.Recent = nil
	return map[string]any{
		"account":  account,
		"sync":     sync,
		"pairing":  p.Supervisor.Pairing(),
		"health":   p.Server.Health(ctx, 0),
		"endpoint": p.MCPURL,
		"version":  mcp.Version,
	}, nil
}

func (p *Panel) qr(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	pairing := p.Supervisor.Pairing()
	if pairing.State != "qr" || pairing.QR == "" {
		http.Error(w, "no QR code right now", http.StatusNotFound)
		return
	}
	png, err := qrcode.Encode(pairing.QR, qrcode.Medium, 512)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}

func (p *Panel) startPairing(r *http.Request) (any, error) {
	var body struct {
		Phone string `json:"phone"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4096)).Decode(&body)
	account, _ := p.Supervisor.Account(r.Context())
	if account.Authenticated {
		return nil, userError{"este computador já está conectado a um WhatsApp; desconecte antes de conectar outro"}
	}
	phone := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, body.Phone)
	if body.Phone != "" && (len(phone) < 8 || len(phone) > 15) {
		return nil, userError{"informe o número com DDI e DDD, por exemplo +55 11 91234-5678"}
	}
	if err := p.Supervisor.StartPairing(phone); err != nil {
		if errors.Is(err, wacli.ErrPairingRunning) {
			return nil, userError{"já existe uma conexão em andamento"}
		}
		return nil, err
	}
	return p.Supervisor.Pairing(), nil
}

func (p *Panel) cancelPairing(r *http.Request) (any, error) {
	p.Supervisor.CancelPairing()
	return p.Supervisor.Pairing(), nil
}

func (p *Panel) logout(r *http.Request) (any, error) {
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if err := p.Supervisor.Logout(ctx); err != nil {
		return nil, err
	}
	return map[string]bool{"logged_out": true}, nil
}

// ---- clients ----

type clientState struct {
	ClaudeCode    clientInfo `json:"claude_code"`
	ClaudeDesktop clientInfo `json:"claude_desktop"`
	CodeCommand   string     `json:"claude_code_command"`
	DesktopJSON   string     `json:"claude_desktop_json"`
	DesktopPath   string     `json:"claude_desktop_path"`
}

type clientInfo struct {
	Found      bool   `json:"found"`
	Configured bool   `json:"configured"`
	Detail     string `json:"detail,omitempty"`
}

func (p *Panel) codeArgs() []string {
	args := []string{"mcp", "add", "--scope", "user", "--transport", "http", ServerName, p.MCPURL}
	if p.Token != "" {
		args = append(args, "--header", "Authorization: Bearer "+p.Token)
	}
	return args
}

func (p *Panel) desktopEntry() map[string]any {
	entry := map[string]any{"command": p.Binary, "args": []string{"bridge"}}
	env := map[string]string{}
	if p.Port != p.DefaultPort {
		env["WHATSAPP_MCP_PORT"] = fmt.Sprint(p.Port)
	}
	if p.Token != "" {
		env["WHATSAPP_MCP_TOKEN"] = p.Token
	}
	if len(env) > 0 {
		entry["env"] = env
	}
	return entry
}

func (p *Panel) clients(r *http.Request) (any, error) {
	st := clientState{DesktopPath: desktopConfigPath()}
	st.CodeCommand = "claude " + shellJoin(p.codeArgs())
	snippet, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{ServerName: p.desktopEntry()}}, "", "  ")
	st.DesktopJSON = string(snippet)

	if bin := findClaude(); bin != "" {
		st.ClaudeCode.Found = true
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, bin, "mcp", "get", ServerName).CombinedOutput()
		st.ClaudeCode.Configured = err == nil && strings.Contains(string(out), p.MCPURL)
	} else {
		st.ClaudeCode.Detail = "o comando claude não foi encontrado neste computador"
	}

	cfg, err := readDesktopConfig()
	switch {
	case errors.Is(err, os.ErrNotExist):
		_, dirErr := os.Stat(filepath.Dir(st.DesktopPath))
		st.ClaudeDesktop.Found = dirErr == nil
		if !st.ClaudeDesktop.Found {
			st.ClaudeDesktop.Detail = "o Claude Desktop não parece estar instalado"
		}
	case err != nil:
		st.ClaudeDesktop.Found = true
		st.ClaudeDesktop.Detail = "não foi possível ler a configuração: " + err.Error()
	default:
		st.ClaudeDesktop.Found = true
		if servers, ok := cfg["mcpServers"].(map[string]any); ok {
			if entry, ok := servers[ServerName].(map[string]any); ok {
				st.ClaudeDesktop.Configured = entry["command"] == p.Binary
			}
		}
	}
	return st, nil
}

func (p *Panel) addClaudeCode(r *http.Request) (any, error) {
	bin := findClaude()
	if bin == "" {
		return nil, userError{"o comando claude não foi encontrado; instale o Claude Code ou rode o comando mostrado abaixo"}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	// Replace rather than fail on an older entry with another URL.
	_ = exec.CommandContext(ctx, bin, "mcp", "remove", "--scope", "user", ServerName).Run()
	out, err := exec.CommandContext(ctx, bin, p.codeArgs()...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("claude mcp add falhou: %s", strings.TrimSpace(string(out)))
	}
	return p.clients(r)
}

func (p *Panel) addClaudeDesktop(r *http.Request) (any, error) {
	path := desktopConfigPath()
	cfg, err := readDesktopConfig()
	if errors.Is(err, os.ErrNotExist) {
		cfg = map[string]any{}
	} else if err != nil {
		return nil, userError{"a configuração do Claude Desktop não é um JSON válido; corrija " + path + " antes"}
	} else if raw, err := os.ReadFile(path); err == nil {
		backup := path + ".bak-" + time.Now().Format("20060102-150405")
		if err := os.WriteFile(backup, raw, 0o600); err != nil {
			return nil, fmt.Errorf("não foi possível salvar uma cópia da configuração: %w", err)
		}
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers[ServerName] = p.desktopEntry()
	cfg["mcpServers"] = servers
	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(body, '\n'), 0o600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return nil, err
	}
	return p.clients(r)
}

func desktopConfigPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Claude", "claude_desktop_config.json")
	}
	return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
}

// readDesktopConfig keeps numbers as written, so rewriting the file changes
// only the entry this panel adds.
func readDesktopConfig() (map[string]any, error) {
	raw, err := os.ReadFile(desktopConfigPath())
	if err != nil {
		return nil, err
	}
	cfg := map[string]any{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return cfg, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// findClaude looks beyond PATH, because a login service starts with a minimal
// one.
func findClaude() string {
	if p, err := exec.LookPath("claude"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		filepath.Join(home, ".local", "bin", "claude"),
		filepath.Join(home, ".claude", "local", "claude"),
		"/opt/homebrew/bin/claude",
		"/usr/local/bin/claude",
	} {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

func shellJoin(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \"'$`\\") {
			out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		} else {
			out[i] = a
		}
	}
	return strings.Join(out, " ")
}
