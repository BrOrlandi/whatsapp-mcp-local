package public

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/netguard"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

// client is an OAuth client asking to sign in: one that registered here
// (RFC 7591), or one that names itself by the address of its metadata
// document (a Client ID Metadata Document).
type client struct {
	ID           string
	Name         string
	RedirectURIs []string
	// Document is set for a client known by its metadata document.
	Document bool
}

const (
	cimdTimeout   = 5 * time.Second
	cimdMaxBytes  = 64 << 10
	cimdCacheOK   = time.Hour
	cimdCacheFail = time.Minute
)

// documents fetches and caches clients' metadata documents. The fetch goes
// only to the internet (netguard checks the address dialled), follows no
// redirect, and is cut at 5 seconds and 64 KB.
type documents struct {
	client  *http.Client
	now     func() time.Time
	mu      sync.Mutex
	cache   map[string]cachedDoc
	fetches map[string][]time.Time // by client address
}

type cachedDoc struct {
	c       client
	err     error
	expires time.Time
}

func newDocuments() *documents {
	return &documents{client: netguard.Client(cimdTimeout, 0), now: time.Now, cache: map[string]cachedDoc{}, fetches: map[string][]time.Time{}}
}

// isDocumentID reports whether a client_id is a metadata document's address.
func isDocumentID(id string) bool { return strings.HasPrefix(id, "https://") }

var errDocument = errors.New("document")

// fetchesPerHour bounds how many documents one address may make the server
// fetch, since anyone may ask.
const fetchesPerHour = 30

func (d *documents) get(ctx context.Context, id, addr string) (client, error) {
	d.mu.Lock()
	if c, ok := d.cache[id]; ok && d.now().Before(c.expires) {
		d.mu.Unlock()
		return c.c, c.err
	}
	now := d.now()
	recent := d.fetches[addr][:0:0]
	for _, t := range d.fetches[addr] {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	if len(recent) >= fetchesPerHour {
		d.mu.Unlock()
		return client{}, errDocument
	}
	if len(d.fetches) > 10000 {
		d.fetches = map[string][]time.Time{}
	}
	d.fetches[addr] = append(recent, now)
	d.mu.Unlock()
	c, err := d.fetch(ctx, id)
	ttl := cimdCacheOK
	if err != nil {
		ttl = cimdCacheFail
	}
	d.mu.Lock()
	if len(d.cache) > 1000 {
		d.cache = map[string]cachedDoc{}
	}
	d.cache[id] = cachedDoc{c: c, err: err, expires: d.now().Add(ttl)}
	d.mu.Unlock()
	return c, err
}

func (d *documents) fetch(ctx context.Context, id string) (client, error) {
	u, err := url.Parse(id)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || u.Path == "" || u.Path == "/" ||
		strings.Contains(u.Path, "/./") || strings.Contains(u.Path, "/../") || strings.HasSuffix(u.Path, "/..") || strings.HasSuffix(u.Path, "/.") {
		return client{}, errDocument
	}
	// A literal address is checked here too, for a clearer refusal.
	if ip, err := netip.ParseAddr(strings.Trim(u.Hostname(), "[]")); err == nil && netguard.Internal(ip) {
		return client{}, errDocument
	}
	ctx, cancel := context.WithTimeout(ctx, cimdTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, id, nil)
	if err != nil {
		return client{}, errDocument
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "whatsapp-mcp (OAuth client metadata)")
	resp, err := d.client.Do(req)
	if err != nil {
		return client{}, errDocument
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return client{}, errDocument
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, cimdMaxBytes+1))
	if err != nil || len(body) > cimdMaxBytes {
		return client{}, errDocument
	}
	var doc struct {
		ClientID     string   `json:"client_id"`
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
		AuthMethod   string   `json:"token_endpoint_auth_method"`
		AuthMethods  []string `json:"token_endpoint_auth_methods_supported"`
	}
	if json.Unmarshal(body, &doc) != nil || doc.ClientID != id || len(doc.RedirectURIs) == 0 || len(doc.RedirectURIs) > 20 {
		return client{}, errDocument
	}
	// This server has public clients only: the client must be able to be
	// one. ChatGPT's document names private_key_jwt as its method and lists
	// none among those it supports.
	if doc.AuthMethod != "" && doc.AuthMethod != "none" && !slices.Contains(doc.AuthMethods, "none") {
		return client{}, errDocument
	}
	for _, r := range doc.RedirectURIs {
		if !validRedirect(r) {
			return client{}, errDocument
		}
	}
	return client{ID: id, Name: cleanName(doc.ClientName, u.Hostname()), RedirectURIs: doc.RedirectURIs, Document: true}, nil
}

// cleanName is a client's name as the consent page shows it: printable,
// short, or the host it came from.
func cleanName(name, fallback string) string {
	name = strings.Join(strings.FieldsFunc(name, func(r rune) bool { return r < 0x20 || r == 0x7f }), " ")
	name = strings.TrimSpace(name)
	if r := []rune(name); len(r) > 60 {
		name = string(r[:60])
	}
	if name == "" {
		return fallback
	}
	return name
}

// resolveClient is the client a client_id names.
func (s *Server) resolveClient(ctx context.Context, id, addr string) (client, error) {
	if isDocumentID(id) {
		c, err := s.documents.get(ctx, id, addr)
		if err != nil {
			return c, errors.New(i18n.T("não foi possível ler a descrição do aplicativo que pediu acesso; tente de novo em um minuto"))
		}
		return c, nil
	}
	r, err := s.state.OAuthClient(ctx, id)
	if err != nil {
		return client{}, errors.New(i18n.T("este aplicativo não está registrado neste servidor; conecte de novo a partir do aplicativo"))
	}
	return client{ID: r.ID, Name: r.Name, RedirectURIs: r.RedirectURIs}, nil
}

// validRedirect is a redirect address a client may register: https, or
// http on a loopback address for a program on the person's computer. No
// fragment.
func validRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.Host == "" || u.User != nil {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		return loopbackHost(u.Hostname())
	}
	return false
}

func loopbackHost(h string) bool {
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(h, "[]"))
	return ip != nil && ip.IsLoopback()
}

// redirectAllowed reports whether a redirect address is one the client
// registered: exactly, except that on a loopback http address the port is
// not compared (RFC 8252 section 7.3), as a program there takes any free
// port.
func redirectAllowed(registered []string, raw string) bool {
	for _, r := range registered {
		if r == raw {
			return true
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || !loopbackHost(u.Hostname()) || u.Fragment != "" || u.User != nil {
		return false
	}
	for _, r := range registered {
		v, err := url.Parse(r)
		if err != nil || v.Scheme != "http" || !strings.EqualFold(v.Hostname(), u.Hostname()) {
			continue
		}
		if v.EscapedPath() == u.EscapedPath() && v.RawQuery == u.RawQuery {
			return true
		}
	}
	return false
}

// loopbackOnly reports whether every redirect a client has goes to the
// person's own computer: any program there could claim to be it, so the
// consent page warns.
func loopbackOnly(c client) bool {
	for _, r := range c.RedirectURIs {
		if u, err := url.Parse(r); err == nil && !loopbackHost(u.Hostname()) {
			return false
		}
	}
	return true
}

// registered is a new client registration's record.
func registeredClient(name string, uris []string, now time.Time) state.OAuthClient {
	return state.OAuthClient{ID: shortID(clientPrefix) + shortID(""), Name: name, RedirectURIs: uris, CreatedAt: now.UTC()}
}
