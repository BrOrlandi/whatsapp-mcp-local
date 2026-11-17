// Package public is the server mode's second listener: the one a proxy or a
// tunnel points at, where everything asks for a sign-in. It exists only when
// WHATSAPP_MCP_PUBLIC_URL is set; without it nothing here runs, and the
// local listener is the only one, as always.
package public

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
)

// The server mode's settings.
const (
	EnvURL          = "WHATSAPP_MCP_PUBLIC_URL"
	EnvPasswordHash = "WHATSAPP_MCP_PANEL_PASSWORD_HASH"
	EnvListen       = "WHATSAPP_MCP_PUBLIC_LISTEN"
	EnvTrustedProxy = "WHATSAPP_MCP_TRUSTED_PROXY"
)

// DefaultListen is where the public listener answers when
// WHATSAPP_MCP_PUBLIC_LISTEN does not say.
const DefaultListen = "127.0.0.1:47822"

// Config is the server mode.
type Config struct {
	// URL is the server's public address; Origin is its scheme and host,
	// what a browser on the panel sends as Origin.
	URL    *url.URL
	Origin string
	// Listen is the public listener's host:port.
	Listen   string
	Password *Password
	proxies  *proxies
}

// HTTPS reports whether the public address is https.
func (c *Config) HTTPS() bool { return c.URL.Scheme == "https" }

// FromEnv reads the server mode from the environment. It is nil, with no
// error, when the server mode is off: neither the public address nor the
// password hash is set. With only one of them, or with a password that is
// missing or not a strong bcrypt hash, it says what is wrong, and the
// service does not start.
func FromEnv(getenv func(string) string, dataDir string) (*Config, error) {
	rawURL := strings.TrimSpace(getenv(EnvURL))
	envHash := strings.TrimSpace(getenv(EnvPasswordHash))
	if rawURL == "" && envHash == "" {
		return nil, nil
	}
	if rawURL == "" {
		return nil, errors.New(i18n.Tf("%s está definida, mas %s não: o modo servidor precisa das duas", EnvPasswordHash, EnvURL))
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New(i18n.Tf("%s precisa ser um endereço como https://whatsapp.exemplo.com, sem caminho", EnvURL))
	}
	u.Path = ""
	c := &Config{URL: u, Origin: u.Scheme + "://" + strings.ToLower(u.Host)}
	c.Password = NewPassword(envHash, PasswordPath(dataDir))
	if _, err := c.Password.Current(); errors.Is(err, errNoPassword) {
		return nil, errors.New(i18n.Tf("o modo servidor precisa da senha do painel: defina %s (gere o hash com whatsapp-mcp password hash) ou rode whatsapp-mcp password set", EnvPasswordHash))
	} else if err != nil {
		return nil, err
	}
	c.Listen = strings.TrimSpace(getenv(EnvListen))
	if c.Listen == "" {
		c.Listen = DefaultListen
	}
	host, port, err := net.SplitHostPort(c.Listen)
	if n, perr := strconv.Atoi(port); err != nil || perr != nil || n < 1 || n > 65535 || (host != "" && net.ParseIP(host) == nil && host != "localhost") {
		return nil, errors.New(i18n.Tf("%s precisa ser um endereço e uma porta, como 127.0.0.1:47822 ou 0.0.0.0:47822", EnvListen))
	}
	if c.proxies, err = parseProxies(getenv(EnvTrustedProxy)); err != nil {
		return nil, err
	}
	return c, nil
}
