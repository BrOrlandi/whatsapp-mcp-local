package public

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// proxies says which peers are the proxy or tunnel in front of the public
// listener, whose X-Forwarded-For and X-Forwarded-Proto are believed.
//
// With WHATSAPP_MCP_TRUSTED_PROXY unset, a loopback peer is that proxy: with
// the listener on 127.0.0.1, every tunnel (cloudflared, tailscale) arrives
// from there, and a program on loopback already reaches the local listener,
// which asks for nothing. With it set, only the peers it names are believed,
// and no other peer may talk to the listener at all.
type proxies struct {
	set      bool
	prefixes []netip.Prefix
	names    []string

	mu       sync.Mutex
	resolved []netip.Addr
	at       time.Time
}

// parseProxies reads WHATSAPP_MCP_TRUSTED_PROXY: IPs, CIDR ranges or host
// names (a container's name on a Docker network), comma separated. Names
// are looked up again every 30 seconds, so a proxy recreated with another
// address stays trusted. A list with something invalid in it is refused,
// rather than trusting less, or more, than the person meant.
func parseProxies(spec string) (*proxies, error) {
	p := &proxies{}
	if strings.Trim(spec, " ,") == "" {
		return p, nil
	}
	p.set = true
	for _, item := range strings.Split(spec, ",") {
		item = strings.TrimSpace(item)
		switch {
		case item == "":
		case strings.Contains(item, "/"):
			prefix, err := netip.ParsePrefix(item)
			if err != nil {
				return nil, errors.New("WHATSAPP_MCP_TRUSTED_PROXY: " + item + " is not a valid CIDR range")
			}
			p.prefixes = append(p.prefixes, prefix.Masked())
		default:
			if a, err := netip.ParseAddr(item); err == nil {
				p.prefixes = append(p.prefixes, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()))
			} else if validName(item) {
				p.names = append(p.names, item)
			} else {
				return nil, errors.New("WHATSAPP_MCP_TRUSTED_PROXY: " + item + " is not an address, a CIDR range or a host name")
			}
		}
	}
	return p, nil
}

func validName(s string) bool {
	if len(s) > 253 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.' || r == '_') {
			return false
		}
	}
	return true
}

// trusted reports whether a peer (an IP, with or without a port) is the
// proxy.
func (p *proxies) trusted(peer string) bool {
	addr, ok := parseAddr(peer)
	if !ok {
		return false
	}
	if !p.set {
		return addr.IsLoopback()
	}
	for _, prefix := range p.prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	for _, a := range p.lookup() {
		if a.Unmap() == addr {
			return true
		}
	}
	return false
}

func (p *proxies) lookup() []netip.Addr {
	if len(p.names) == 0 {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if time.Since(p.at) < 30*time.Second {
		return p.resolved
	}
	var out []netip.Addr
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, n := range p.names {
		if addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", n); err == nil {
			out = append(out, addrs...)
		}
	}
	p.resolved, p.at = out, time.Now()
	return p.resolved
}

func parseAddr(hostport string) (netip.Addr, bool) {
	host := strings.TrimSpace(hostport)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	addr, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

// peer is what the public listener knows about where a request came from.
type peer struct {
	// proxy is set when the socket's peer is the trusted proxy.
	proxy bool
	// client is the address the request is from: the socket's, or, through
	// the proxy, the last address in X-Forwarded-For that is not a proxy.
	client string
	// shared is set when client is the proxy's own address, because the
	// proxy did not say whom it forwards: every visitor then shares it.
	shared bool
	// https is set when the request reached the proxy, or the listener,
	// over TLS.
	https bool
}

func (p *proxies) read(r *http.Request) peer {
	sock, _ := parseAddr(r.RemoteAddr)
	pr := peer{client: sock.String(), https: r.TLS != nil}
	if !p.trusted(r.RemoteAddr) {
		return pr
	}
	pr.proxy = true
	proto := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]))
	if proto == "https" || proto == "wss" {
		pr.https = true
	}
	// X-Forwarded-For lists each hop; the proxy appends the address it saw.
	// Walking from the right, the first address that is not itself a
	// proxy is the client; anything to its left the client could have
	// written.
	var hops []string
	for _, h := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(h, ",")...)
	}
	for i := len(hops) - 1; i >= 0; i-- {
		addr, ok := parseAddr(hops[i])
		if !ok {
			break
		}
		pr.client = addr.String()
		if !p.trusted(addr.String()) {
			return pr
		}
	}
	// Every address was a proxy's, or there was none: the visitor is not
	// known, and shares the proxy's address with everyone else.
	pr.shared = true
	return pr
}
