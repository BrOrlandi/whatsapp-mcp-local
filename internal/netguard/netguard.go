// Package netguard is an HTTP client that reaches only the internet: never
// this computer, its network, a cloud's metadata service or a reserved
// range. The server uses it when a request from outside makes it fetch an
// address: a file to send for an agent on another computer, or an OAuth
// client's metadata document.
package netguard

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// blocked are the ranges beyond what netip already names (loopback,
// private, link-local, multicast, unspecified) that are not the internet.
var blocked = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network"
	netip.MustParsePrefix("100.64.0.0/10"),   // carrier-grade NAT, also Tailscale
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // documentation
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // documentation
	netip.MustParsePrefix("203.0.113.0/24"),  // documentation
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved, and broadcast
	netip.MustParsePrefix("64:ff9b::/96"),    // NAT64, which reaches IPv4 inside
	netip.MustParsePrefix("64:ff9b:1::/48"),  // local NAT64
	netip.MustParsePrefix("2001:db8::/32"),   // documentation
	netip.MustParsePrefix("fc00::/7"),        // unique local
	netip.MustParsePrefix("::/96"),           // IPv4-compatible, which reaches IPv4 inside
	netip.MustParsePrefix("2002::/16"),       // 6to4, which carries an IPv4 address
	netip.MustParsePrefix("2001::/32"),       // Teredo, which carries an IPv4 address
}

// Internal reports whether an address is not on the internet.
func Internal(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() ||
		a.IsInterfaceLocalMulticast() || a.IsMulticast() || a.IsUnspecified() {
		return true
	}
	for _, p := range blocked {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// ErrInternal is the refusal of an address that is not on the internet.
var ErrInternal = errors.New("only addresses on the internet are reachable")

// control runs on the address actually dialled, after DNS, so a name that
// resolves (or re-resolves) to the network inside is refused too, on every
// redirect's hop as well.
func control(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("refused to connect to %s", address)
	}
	if Internal(ap.Addr()) {
		return fmt.Errorf("refused to connect to %s: %w", ap.Addr(), ErrInternal)
	}
	return nil
}

// Client is an HTTP client whose every connection is checked: no proxy
// from the environment (it would be dialled in the address's place), and
// at most redirects redirects, each to http(s). timeout bounds the whole
// request.
func Client(timeout time.Duration, redirects int) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           (&net.Dialer{Timeout: min(30*time.Second, timeout), Control: control}).DialContext,
			TLSHandshakeTimeout:   min(15*time.Second, timeout),
			ResponseHeaderTimeout: min(time.Minute, timeout),
			MaxIdleConns:          4,
			IdleConnTimeout:       30 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= redirects {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("refused a redirect to %s", req.URL.Scheme)
			}
			return nil
		},
	}
}
