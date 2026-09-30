package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/state"
)

// ServerName is how this MCP appears in the clients. It differs from the
// hosted v1's "whatsapp" so both can be installed side by side.
const ServerName = "whatsapp-local"

// clientInfo is what the panel knows about one client's configuration.
type clientInfo struct {
	Found      bool   `json:"found"`
	Configured bool   `json:"configured"`
	Detail     string `json:"detail,omitempty"`
}

// setup is everything the "connect a tool" instructions show, filled in.
type setup struct {
	Endpoint    string
	Command     string
	JSON        string
	DesktopPath string
	AgentPrompt string
	Desktop     clientInfo
	Code        clientInfo
}

// Connection is one row of "Suas conexões": a client that is configured here,
// has used the server, or both.
type Connection struct {
	Key        string
	Tool       string
	Version    string
	Configured bool
	Live       bool
	LastUsed   time.Time
}

// clientNames turns what a client calls itself at initialize into what a
// person calls it.
var clientNames = map[string]string{
	"claude-ai":   "Claude Desktop",
	"claude-code": "Claude Code",
	"cursor":      "Cursor",
	"windsurf":    "Windsurf",
	"codex":       "Codex",
}

func toolName(name string) string {
	if n, ok := clientNames[strings.ToLower(name)]; ok {
		return n
	}
	for prefix, n := range clientNames {
		if strings.HasPrefix(strings.ToLower(name), prefix) {
			return n
		}
	}
	return name
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

// codeState asks the claude CLI whether it has this server. The CLI takes a
// second or two to answer, so the answer is kept briefly.
type codeCache struct {
	mu   sync.Mutex
	at   time.Time
	info clientInfo
}

func (p *Panel) codeInfo(ctx context.Context, fresh bool) clientInfo {
	p.code.mu.Lock()
	defer p.code.mu.Unlock()
	if !fresh && time.Since(p.code.at) < 30*time.Second {
		return p.code.info
	}
	info := clientInfo{}
	if bin := findClaude(); bin != "" {
		info.Found = true
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, bin, "mcp", "get", ServerName).CombinedOutput()
		info.Configured = err == nil && strings.Contains(string(out), p.MCPURL)
	} else {
		info.Detail = "o comando claude não foi encontrado neste computador"
	}
	p.code.info, p.code.at = info, time.Now()
	return info
}

func (p *Panel) desktopInfo() clientInfo {
	info := clientInfo{}
	cfg, err := readDesktopConfig()
	switch {
	case errors.Is(err, os.ErrNotExist):
		_, dirErr := os.Stat(filepath.Dir(desktopConfigPath()))
		info.Found = dirErr == nil
		if !info.Found {
			info.Detail = "o Claude Desktop não parece estar instalado"
		}
	case err != nil:
		info.Found = true
		info.Detail = "não foi possível ler a configuração: " + err.Error()
	default:
		info.Found = true
		if servers, ok := cfg["mcpServers"].(map[string]any); ok {
			if entry, ok := servers[ServerName].(map[string]any); ok {
				info.Configured = entry["command"] == p.Binary
			}
		}
	}
	return info
}

func (p *Panel) setup(ctx context.Context) setup {
	snippet, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{ServerName: p.desktopEntry()}}, "", "  ")
	return setup{
		Endpoint:    p.MCPURL,
		Command:     "claude " + shellJoin(p.codeArgs()),
		JSON:        string(snippet),
		DesktopPath: desktopConfigPath(),
		AgentPrompt: agentPrompt(p.MCPURL, string(snippet)),
		Desktop:     p.desktopInfo(),
		Code:        p.codeInfo(ctx, false),
	}
}

// connections merges what is configured with what has actually connected.
func (p *Panel) connections(ctx context.Context, s setup) []Connection {
	seen, _ := p.State.Clients(ctx)
	byName := map[string]state.Client{}
	for _, c := range seen {
		byName[c.Name] = c
	}
	var out []Connection
	add := func(key, tool string, configured bool, client string) {
		c, ok := byName[client]
		if !configured && !ok {
			return
		}
		delete(byName, client)
		out = append(out, Connection{Key: key, Tool: tool, Version: c.Version, Configured: configured, Live: ok, LastUsed: c.LastSeen})
	}
	add("claude-desktop", "Claude Desktop", s.Desktop.Configured, "claude-ai")
	add("claude-code", "Claude Code", s.Code.Configured, "claude-code")
	for name, c := range byName {
		out = append(out, Connection{Key: "client:" + name, Tool: toolName(name), Version: c.Version, Live: true, LastUsed: c.LastSeen})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].LastUsed.After(out[j].LastUsed) })
	return out
}

func (p *Panel) addClaudeCode(ctx context.Context) error {
	bin := findClaude()
	if bin == "" {
		return userError{"o comando claude não foi encontrado; instale o Claude Code ou rode o comando mostrado"}
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	// Replace rather than fail on an older entry with another URL.
	_ = exec.CommandContext(ctx, bin, "mcp", "remove", "--scope", "user", ServerName).Run()
	out, err := exec.CommandContext(ctx, bin, p.codeArgs()...).CombinedOutput()
	p.codeInfo(ctx, true)
	if err != nil {
		return fmt.Errorf("claude mcp add falhou: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (p *Panel) removeClaudeCode(ctx context.Context) error {
	bin := findClaude()
	if bin == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, bin, "mcp", "remove", "--scope", "user", ServerName).Run()
	p.codeInfo(ctx, true)
	return nil
}

// editDesktop rewrites the Desktop configuration through f, after keeping a
// copy of the file as it was.
func editDesktop(f func(servers map[string]any)) error {
	path := desktopConfigPath()
	cfg, err := readDesktopConfig()
	if errors.Is(err, os.ErrNotExist) {
		cfg = map[string]any{}
	} else if err != nil {
		return userError{"a configuração do Claude Desktop não é um JSON válido; corrija " + path + " antes"}
	} else if raw, err := os.ReadFile(path); err == nil {
		backup := path + ".bak-" + time.Now().Format("20060102-150405")
		if err := os.WriteFile(backup, raw, 0o600); err != nil {
			return fmt.Errorf("não foi possível salvar uma cópia da configuração: %w", err)
		}
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	f(servers)
	cfg["mcpServers"] = servers
	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(body, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (p *Panel) addClaudeDesktop() error {
	return editDesktop(func(servers map[string]any) { servers[ServerName] = p.desktopEntry() })
}

func (p *Panel) removeClaudeDesktop() error {
	if _, err := os.Stat(desktopConfigPath()); err != nil {
		return nil
	}
	return editDesktop(func(servers map[string]any) { delete(servers, ServerName) })
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
