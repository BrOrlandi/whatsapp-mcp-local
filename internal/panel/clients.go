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

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/platform"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
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
	Endpoint     string
	Command      string
	JSON         string
	DesktopPath  string
	AgentPrompt  string
	CodexCommand string
	CodexTOML    string
	CodexPath    string
	CursorJSON   string
	CursorPath   string
	Desktop      clientInfo
	Code         clientInfo
	Codex        clientInfo
	Cursor       clientInfo
}

// Connection is one row of "Suas conexões": a client that is configured here,
// has used the server, or both.
type Connection struct {
	Key  string
	Tool string
	// Mark is the tool's logo; empty for any other tool, which shows the
	// initial of its name, and whose name the person may change.
	Mark       string
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

func (p *Panel) codexArgs() []string {
	args := []string{"mcp", "add", ServerName, "--url", p.endpoint()}
	if p.Token != "" {
		// Codex sends the key from its own environment, never from its file.
		args = append(args, "--bearer-token-env-var", "WHATSAPP_MCP_TOKEN")
	}
	return args
}

func (p *Panel) codexTOML() string {
	out := "[mcp_servers." + ServerName + "]\nurl = \"" + p.endpoint() + "\"\n"
	if p.Token != "" {
		out += "bearer_token_env_var = \"WHATSAPP_MCP_TOKEN\"\n"
	}
	return out
}

// cursorEntry is the server as Cursor's mcp.json lists it: by address.
func (p *Panel) cursorEntry() map[string]any {
	entry := map[string]any{"url": p.endpoint()}
	if p.Token != "" {
		entry["headers"] = map[string]string{"Authorization": "Bearer " + p.Token}
	}
	return entry
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

// codeCache keeps what a client's command line said about this server: it
// takes a second or two to answer, so the answer is kept briefly.
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

// codexInfo asks the codex CLI whether it has this server.
func (p *Panel) codexInfo(ctx context.Context, fresh bool) clientInfo {
	p.codex.mu.Lock()
	defer p.codex.mu.Unlock()
	endpoint := p.endpoint()
	if !fresh && time.Since(p.codex.at) < 30*time.Second && p.codex.endpoint == endpoint {
		return p.codex.info
	}
	info := clientInfo{}
	if bin := findCodex(); bin != "" {
		info.Found = true
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, "mcp", "get", ServerName, "--json")
		platform.Background(cmd)
		if out, err := cmd.Output(); err == nil {
			switch where := parseCodexGet(out); {
			case where == endpoint:
				info.Configured = true
			case isLocalMCP(where):
				info.Stale = true
			case where != "":
				info.Other = where
			}
		}
	} else {
		info.Detail = "o comando codex não foi encontrado neste computador"
	}
	p.codex.info, p.codex.at, p.codex.endpoint = info, time.Now(), endpoint
	return info
}

// parseCodexGet reads the address out of `codex mcp get --json`.
func parseCodexGet(out []byte) string {
	var got struct {
		Transport struct {
			URL     string   `json:"url"`
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"transport"`
	}
	if json.Unmarshal(out, &got) != nil {
		return ""
	}
	if got.Transport.URL != "" {
		return got.Transport.URL
	}
	return strings.TrimSpace(strings.Join(append([]string{got.Transport.Command}, got.Transport.Args...), " "))
}

// cursorInfo reads Cursor's mcp.json for this server.
func (p *Panel) cursorInfo() clientInfo {
	info := clientInfo{}
	cfg, err := readJSONConfig(CursorConfigPath())
	switch {
	case errors.Is(err, os.ErrNotExist):
		info.Found = cursorInstalled()
		if !info.Found {
			info.Detail = "o Cursor não parece estar instalado"
		}
	case err != nil:
		info.Found = true
		info.Detail = "não foi possível ler a configuração: " + err.Error()
	default:
		info.Found = true
		if servers, ok := cfg["mcpServers"].(map[string]any); ok {
			if entry, ok := servers[ServerName].(map[string]any); ok {
				switch where := describeEntry(entry); {
				case where == p.endpoint():
					info.Configured = true
				case isLocalMCP(where):
					info.Stale = true
				default:
					info.Other = where
				}
			}
		}
	}
	return info
}

func cursorInstalled() bool {
	home, _ := os.UserHomeDir()
	for _, p := range []string{filepath.Join(home, ".cursor"), "/Applications/Cursor.app", filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "cursor")} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func (p *Panel) desktopInfo() clientInfo {
	info := clientInfo{}
	cfg, err := readJSONConfig(DesktopConfigPath())
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
	cursor, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{ServerName: p.cursorEntry()}}, "", "  ")
	return setup{
		Endpoint:     p.endpoint(),
		Command:      "claude " + shellJoin(p.codeArgs()),
		JSON:         string(snippet),
		DesktopPath:  DesktopConfigPath(),
		AgentPrompt:  agentPrompt(p.endpoint(), string(snippet)),
		CodexCommand: "codex " + shellJoin(p.codexArgs()),
		CodexTOML:    p.codexTOML(),
		CodexPath:    CodexConfigPath(),
		CursorJSON:   string(cursor),
		CursorPath:   CursorConfigPath(),
		Desktop:      p.desktopInfo(),
		Code:         p.codeInfo(ctx, false),
		Codex:        p.codexInfo(ctx, false),
		Cursor:       p.cursorInfo(),
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
	// A tool is known by the start of the name it gives: the rest may carry
	// the editor or the version.
	add := func(key, tool string, configured bool, prefix string) {
		var c state.Client
		ok := false
		for name, seen := range byName {
			if strings.HasPrefix(name, prefix) && (!ok || seen.LastSeen.After(c.LastSeen)) {
				c, ok = seen, true
			}
		}
		if !configured && !ok {
			return
		}
		for name := range byName {
			if strings.HasPrefix(name, prefix) {
				delete(byName, name)
			}
		}
		out = append(out, Connection{Key: key, Tool: tool, Mark: key, Version: c.Version, Configured: configured, Live: ok, LastUsed: c.LastSeen})
	}
	add("claude-desktop", "Claude Desktop", s.Desktop.Configured, "claude-ai")
	add("claude-code", "Claude Code", s.Code.Configured, "claude-code")
	add("codex", "Codex", s.Codex.Configured, codexClient)
	add("cursor", "Cursor", s.Cursor.Configured, cursorClient)
	for name, c := range byName {
		tool := toolName(name)
		if label, _ := p.State.Setting(ctx, clientLabel+name); label != "" {
			tool = label
		}
		out = append(out, Connection{Key: "client:" + name, Tool: tool, Version: c.Version, Live: true, LastUsed: c.LastSeen})
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

// clientLabel keys the name a person gave another tool's connection.
const clientLabel = "client_label:"

// codexClient and cursorClient are how Codex and Cursor's names start when
// they connect.
const (
	codexClient  = "codex-mcp-client"
	cursorClient = "cursor"
)

// addCursor writes the server into Cursor's mcp.json, which every project
// sees, keeping a copy of the file first.
func (p *Panel) addCursor(replace bool) error {
	if info := p.cursorInfo(); info.Other != "" && !replace {
		return errConflict{"Cursor", info.Other}
	}
	return editServers(CursorConfigPath(), "Cursor", func(servers map[string]any) {
		for _, name := range legacyNames {
			delete(servers, name)
		}
		servers[ServerName] = p.cursorEntry()
	})
}

func (p *Panel) removeCursor() error {
	if p.cursorInfo().Other != "" {
		return nil
	}
	if _, err := os.Stat(CursorConfigPath()); err != nil {
		return nil
	}
	return editServers(CursorConfigPath(), "Cursor", func(servers map[string]any) {
		delete(servers, ServerName)
		for _, name := range legacyNames {
			delete(servers, name)
		}
	})
}

// CursorConfigPath is Cursor's configuration for every project.
func CursorConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cursor", "mcp.json")
}

// addCodex registers the server with Codex, whose app, command line and
// editor extension share one configuration. A server of the same name
// pointing elsewhere is replaced only when replace is true.
func (p *Panel) addCodex(ctx context.Context, replace bool) error {
	bin := findCodex()
	if bin == "" {
		return userError{"o comando codex não foi encontrado; instale o Codex ou configure à mão"}
	}
	if info := p.codexInfo(ctx, true); info.Other != "" && !replace {
		return errConflict{"Codex", info.Other}
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	p.runCodexRemove(ctx, bin)
	add := exec.CommandContext(ctx, bin, p.codexArgs()...)
	platform.Background(add)
	out, err := add.CombinedOutput()
	p.codexInfo(ctx, true)
	if err != nil {
		return fmt.Errorf("codex mcp add falhou: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (p *Panel) removeCodex(ctx context.Context) error {
	bin := findCodex()
	if bin == "" {
		return nil
	}
	if p.codexInfo(ctx, true).Other != "" {
		return nil // the "whatsapp" there is not this one: leave it alone
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	p.runCodexRemove(ctx, bin)
	p.codexInfo(ctx, true)
	return nil
}

func (p *Panel) runCodexRemove(ctx context.Context, bin string) {
	for _, name := range append([]string{ServerName}, legacyNames...) {
		rm := exec.CommandContext(ctx, bin, "mcp", "remove", name)
		platform.Background(rm)
		_ = rm.Run()
	}
}

// CodexConfigPath is where Codex keeps its configuration.
func CodexConfigPath() string {
	if dir := os.Getenv("CODEX_HOME"); dir != "" {
		return filepath.Join(dir, "config.toml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "config.toml")
}

// findCodex looks beyond PATH, for the same reason as findClaude.
func findCodex() string {
	if p, err := exec.LookPath("codex"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		"/opt/homebrew/bin/codex",
		"/usr/local/bin/codex",
		// The ChatGPT app carries its own Codex, which reads the same file.
		"/Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex",
		filepath.Join(home, ".local", "bin", "codex"),
		filepath.Join(home, ".npm-global", "bin", "codex"),
	}
	if runtime.GOOS == "windows" {
		candidates = []string{
			filepath.Join(os.Getenv("APPDATA"), "npm", "codex.cmd"),
			filepath.Join(home, ".local", "bin", "codex.exe"),
		}
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// editDesktop rewrites the Desktop configuration through f.
func editDesktop(f func(servers map[string]any)) error {
	return editServers(DesktopConfigPath(), "Claude Desktop", f)
}

// editServers rewrites the mcpServers of a JSON configuration through f,
// after keeping a copy of the file as it was.
func editServers(path, tool string, f func(servers map[string]any)) error {
	cfg, err := readJSONConfig(path)
	if errors.Is(err, os.ErrNotExist) {
		cfg = map[string]any{}
	} else if err != nil {
		return userError{"a configuração do " + tool + " não é um JSON válido; corrija " + path + " antes"}
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

// readJSONConfig keeps numbers as written, so rewriting the file changes only
// the entry this panel adds.
func readJSONConfig(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
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
