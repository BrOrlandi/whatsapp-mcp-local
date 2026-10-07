// Package egress configures the network of WhatsApp's child processes. It
// leaves the MCP listener, updates, and transcription downloads alone.
package egress

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// Proxy is empty to inherit the system's proxy environment, "direct" to
// disable it, or a validated HTTP(S)/SOCKS5 URL. A system Tailscale exit node
// applies to all three modes; this package never changes system routing.
type Proxy string

// Parse refuses malformed endpoints before WhatsApp starts. Errors omit the
// input because proxy URLs can carry passwords.
func Parse(value string) (Proxy, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "direct" {
		return Proxy(value), nil
	}
	u, err := url.Parse(value)
	if err != nil || u.Opaque != "" || u.Hostname() == "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") || strings.ContainsAny(value, "\r\n\t ") {
		return "", errors.New("proxy: use a URL with a host and port, without a path, query or fragment")
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return "", errors.New("proxy: supported schemes are http, https, socks5 and socks5h")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return "", errors.New("proxy: specify a port between 1 and 65535")
	}
	u.Path = ""
	return Proxy(u.String()), nil
}

// Environment isolates the choice to wacli. Explicit proxies replace
// inherited NO_PROXY so WhatsApp cannot accidentally bypass the chosen exit;
// local delegation and the webhook relay still use loopback directly.
func (p Proxy) Environment(base []string) []string {
	if p == "" {
		return append([]string(nil), base...)
	}
	env := make([]string, 0, len(base)+6)
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY":
			continue
		}
		env = append(env, entry)
	}
	if p != "direct" {
		for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
			env = append(env, key+"="+string(p))
		}
	}
	return append(env, "NO_PROXY=localhost,127.0.0.1,::1", "no_proxy=localhost,127.0.0.1,::1")
}
