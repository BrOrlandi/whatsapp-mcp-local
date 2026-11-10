package remote

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// WindowHeader marks a request the app's own window made, which the server
// may treat as its panel's page (with its token). The app sets it only on
// what its window sends, and strips it from whatever reaches it on its
// loopback port, so another program cannot pass for the window.
const WindowHeader = "X-WhatsApp-MCP-Window"

// LocalBaseHeader is the address the app serves the server at on its own
// computer (http://127.0.0.1:<port>), which the app sets on everything it
// sends: the links the server hands out (download_media's) must point
// there, not at the server's own loopback.
const LocalBaseHeader = "X-WhatsApp-MCP-Local-Base"

// TrustProxies is TrustedProxy for a list of IPs, CIDRs or host names (a
// container's name on a Docker network, such as traefik), comma separated.
// Names are looked up again every 30 seconds, so a proxy recreated with
// another address stays trusted. An empty list returns nil: anything that
// reaches the listener is trusted. A list with nothing valid in it (a typo
// such as 172.18.0.0/33) trusts nothing: a mistake in it must not open the
// listener to everyone.
func TrustProxies(spec string) func(remoteAddr string) bool {
	var prefixes []netip.Prefix
	var names []string
	for _, item := range strings.Split(spec, ",") {
		item = strings.TrimSpace(item)
		switch {
		case item == "":
		case strings.Contains(item, "/"):
			if p, err := netip.ParsePrefix(item); err == nil {
				prefixes = append(prefixes, p.Masked())
			}
		default:
			if a, err := netip.ParseAddr(item); err == nil {
				prefixes = append(prefixes, netip.PrefixFrom(a, a.BitLen()))
			} else {
				names = append(names, item)
			}
		}
	}
	if len(prefixes) == 0 && len(names) == 0 {
		if strings.Trim(spec, " ,") != "" {
			return func(string) bool { return false }
		}
		return nil
	}
	var mu sync.Mutex
	var resolved []netip.Addr
	var at time.Time
	lookup := func() []netip.Addr {
		mu.Lock()
		defer mu.Unlock()
		if time.Since(at) < 30*time.Second {
			return resolved
		}
		var out []netip.Addr
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		for _, n := range names {
			addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", n)
			if err == nil {
				out = append(out, addrs...)
			}
		}
		resolved, at = out, time.Now()
		return resolved
	}
	return func(remoteAddr string) bool {
		host, _, err := net.SplitHostPort(remoteAddr)
		if err != nil {
			host = remoteAddr
		}
		addr, err := netip.ParseAddr(host)
		if err != nil {
			return false
		}
		addr = addr.Unmap()
		for _, p := range prefixes {
			if p.Contains(addr) {
				return true
			}
		}
		for _, a := range lookup() {
			if a.Unmap() == addr {
				return true
			}
		}
		return false
	}
}
