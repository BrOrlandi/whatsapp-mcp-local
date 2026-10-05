package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/platform"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/state"
)

// ServerName is how this MCP appears in the clients.
const ServerName = "whatsapp"

// legacyNames are names earlier versions registered under, removed when the
// server is configured again so a client does not list it twice.
var legacyNames = []string{"whatsapp-local"}

// clientInfo is what the panel knows about one client's configuration.
type clientInfo struct {
	Found      bool   `json:"found"`
	Configured bool   `json:"configured"`
	Detail     string `json:"detail,omitempty"`
	// Other is where an existing server of the same name points, when it is
	// not this one: typically the hosted v1. Replacing it needs a yes.
	Other string `json:"other,omitempty"`
	// Stale is this same gateway configured with an older address: another
	// port, or the command line's bridge before the app. Updating it needs
	// no question.
	Stale bool `json:"stale,omitempty"`
}

// isLocalMCP reports whether an address is a WhatsApp MCP on this computer,
// whatever its port.
func isLocalMCP(addr string) bool {
	u, err := url.Parse(addr)
	if err != nil || u.Path != "/mcp" {
		return false
	}
	host := u.Hostname()
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

// isOurBridge reports whether a Claude Desktop entry starts one of this
// gateway's bridges: the app's program, or the command line's subcommand.
func isOurBridge(entry map[string]any) bool {
	command, _ := entry["command"].(string)
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(command)), ".exe")
	if base == "whatsapp-mcp-bridge" {
		return true
	}
	args, _ := entry["args"].([]any)
	// The command line's subcommand, or an AppImage started as the bridge.
	return strings.Contains(base, "whatsapp-mcp") && len(args) == 1 && args[0] == "bridge"
}

// errConflict is a server of the same name that is not this one.
type errConflict struct{ client, other string }

func (e errConflict) Error() string {
	return fmt.Sprintf("o %s já tem um servidor chamado %q, que aponta para %s", e.client, ServerName, e.other)
}

// parseMCPGet reads the address out of `claude mcp get`.
func parseMCPGet(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		for _, prefix := range []string{"URL:", "Command:"} {
			if v, ok := strings.CutPrefix(line, prefix); ok {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
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
	args := []string{"mcp", "add", "--scope", "user", "--transport", "http", ServerName, p.endpoint()}
	if p.Token != "" {
		args = append(args, "--header", "Authorization: Bearer "+p.Token)
	}
	return args
}

func (p *Panel) desktopEntry() map[string]any {
	entry := map[string]any{"command": p.Desktop.Command}
	if len(p.Desktop.Args) > 0 {
		entry["args"] = p.Desktop.Args
	}
	env := map[string]string{}
	for k, v := range p.Desktop.Env {
		env[k] = v
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
	// endpoint is the address the answer was compared with: a port change
	// makes it stale at once.
	endpoint string
}

func (p *Panel) codeInfo(ctx context.Context, fresh bool) clientInfo {
	p.code.mu.Lock()
	defer p.code.mu.Unlock()
	endpoint := p.endpoint()
	if !fresh && time.Since(p.code.at) < 30*time.Second && p.code.endpoint == endpoint {
		return p.code.info
	}
	info := clientInfo{}
	if bin := findClaude(); bin != "" {
		info.Found = true
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, "mcp", "get", ServerName)
		platform.Background(cmd)
		out, err := cmd.CombinedOutput()
		if err == nil {
			switch where := parseMCPGet(string(out)); {
			case where == endpoint:
				info.Configured = true
			case isLocalMCP(where):
				info.Stale = true
			default:
				info.Other = where
			}
		}
	} else {
		info.Detail = "o comando claude não foi encontrado neste computador"
	}
	p.code.info, p.code.at, p.code.endpoint = info, time.Now(), endpoint
	return info
}

func (p *Panel) desktopInfo() clientInfo {
	info := clientInfo{}
	cfg, err := readDesktopConfig()
	switch {
	case errors.Is(err, os.ErrNotExist):
		_, dirErr := os.Stat(filepath.Dir(DesktopConfigPath()))
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
				switch {
				case entry["command"] == p.Desktop.Command:
					info.Configured = true
				case isOurBridge(entry):
					info.Stale = true
				default:
					info.Other = describeEntry(entry)
				}
			}
		}
	}
	return info
}

func (p *Panel) setup(ctx context.Context) setup {
	snippet, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{ServerName: p.desktopEntry()}}, "", "  ")
	return setup{
		Endpoint:    p.endpoint(),
		Command:     "claude " + shellJoin(p.codeArgs()),
		JSON:        string(snippet),
		DesktopPath: DesktopConfigPath(),
		AgentPrompt: agentPrompt(p.endpoint(), string(snippet)),
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

// describeEntry says where a Desktop server entry points.
func describeEntry(entry map[string]any) string {
	if url, ok := entry["url"].(string); ok {
		return url
	}
	parts := []string{fmt.Sprint(entry["command"])}
	if args, ok := entry["args"].([]any); ok {
		for _, a := range args {
			parts = append(parts, fmt.Sprint(a))
		}
	}
	return strings.Join(parts, " ")
}

// addClaudeCode registers the server with Claude Code. A server of the same
// name pointing elsewhere is replaced only when replace is true.
func (p *Panel) addClaudeCode(ctx context.Context, replace bool) error {
	bin := findClaude()
	if bin == "" {
		return userError{"o comando claude não foi encontrado; instale o Claude Code ou rode o comando mostrado"}
	}
	if info := p.codeInfo(ctx, true); info.Other != "" && !replace {
		return errConflict{"Claude Code", info.Other}
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for _, name := range append([]string{ServerName}, legacyNames...) {
		rm := exec.CommandContext(ctx, bin, "mcp", "remove", "--scope", "user", name)
		platform.Background(rm)
		_ = rm.Run()
	}
	add := exec.CommandContext(ctx, bin, p.codeArgs()...)
	platform.Background(add)
	out, err := add.CombinedOutput()
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
	for _, name := range append([]string{ServerName}, legacyNames...) {
		rm := exec.CommandContext(ctx, bin, "mcp", "remove", "--scope", "user", name)
		platform.Background(rm)
		_ = rm.Run()
	}
	p.codeInfo(ctx, true)
	return nil
}

// editDesktop rewrites the Desktop configuration through f, after keeping a
// copy of the file as it was.
func editDesktop(f func(servers map[string]any)) error {
	path := DesktopConfigPath()
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

// addClaudeDesktop writes the server into the Desktop configuration, keeping
// a copy of the file first. A server of the same name pointing elsewhere is
// replaced only when replace is true.
func (p *Panel) addClaudeDesktop(replace bool) error {
	if info := p.desktopInfo(); info.Other != "" && !replace {
		return errConflict{"Claude Desktop", info.Other}
	}
	return editDesktop(func(servers map[string]any) {
		for _, name := range legacyNames {
			delete(servers, name)
		}
		servers[ServerName] = p.desktopEntry()
	})
}

func (p *Panel) removeClaudeDesktop() error {
	if p.desktopInfo().Other != "" {
		return nil // the "whatsapp" there is not this one: leave it alone
	}
	if _, err := os.Stat(DesktopConfigPath()); err != nil {
		return nil
	}
	return editDesktop(func(servers map[string]any) {
		delete(servers, ServerName)
		for _, name := range legacyNames {
			delete(servers, name)
		}
	})
}

// DesktopConfigPath is where Claude Desktop keeps its configuration.
func DesktopConfigPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	case "windows":
		classic := filepath.Join(os.Getenv("APPDATA"), "Claude")
		if _, err := os.Stat(classic); err == nil {
			return filepath.Join(classic, "claude_desktop_config.json")
		}
		// Installed from the Microsoft Store, Claude Desktop reads a
		// virtualised copy of %APPDATA% inside its package folder.
		if store, _ := filepath.Glob(filepath.Join(os.Getenv("LOCALAPPDATA"), "Packages", "Claude_*", "LocalCache", "Roaming", "Claude")); len(store) > 0 {
			return filepath.Join(store[0], "claude_desktop_config.json")
		}
		return filepath.Join(classic, "claude_desktop_config.json")
	}
	return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
}

// readDesktopConfig keeps numbers as written, so rewriting the file changes
// only the entry this panel adds.
func readDesktopConfig() (map[string]any, error) {
	raw, err := os.ReadFile(DesktopConfigPath())
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

// findClaude looks beyond PATH, because an app opened at login, or from the
// Dock, starts with a minimal one.
func findClaude() string {
	if p, err := exec.LookPath("claude"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".local", "bin", "claude"),
		filepath.Join(home, ".claude", "local", "claude"),
		"/opt/homebrew/bin/claude",
		"/usr/local/bin/claude",
	}
	if runtime.GOOS == "windows" {
		candidates = []string{
			filepath.Join(home, ".local", "bin", "claude.exe"),
			filepath.Join(os.Getenv("APPDATA"), "npm", "claude.cmd"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "claude", "claude.exe"),
		}
	}
	for _, p := range candidates {
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
