package mcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/remote"
)

// A tool called through remote access runs on the server for an agent on
// another computer. To that agent the server's files and its network are
// not "this machine": send_media_message then takes no path, and its URL
// may only reach the internet, never the server itself, its LAN or a
// cloud's metadata service. Called locally, nothing changes.

// fromRemote reports whether a tool call came from a paired device.
func fromRemote(ctx context.Context) bool {
	_, ok := remote.DeviceFrom(ctx)
	return ok
}

var errRemotePath = errors.New("through remote access, a file is sent by its https URL: paths on the server are not reachable")

// blockedNets are the ranges a remote device's download may not reach,
// beyond what netip already names (loopback, private, link-local,
// multicast, unspecified).
var blockedNets = []netip.Prefix{
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
}

// internalAddr reports whether an address is one a remote device's
// download may not reach.
func internalAddr(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() ||
		a.IsInterfaceLocalMulticast() || a.IsMulticast() || a.IsUnspecified() {
		return true
	}
	for _, p := range blockedNets {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// guardDial refuses a connection to an internal address. It runs on the
// address actually dialled, after DNS, so a name that resolves (or
// re-resolves) to the LAN is refused too, and on every redirect's hop.
func guardDial(network, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("refused to download from %s", address)
	}
	if internalAddr(ap.Addr()) {
		return fmt.Errorf("refused to download from %s: through remote access, only addresses on the internet are reachable", ap.Addr())
	}
	return nil
}

// remoteClient downloads for a remote device: no proxy from the
// environment (it would be dialled in the URL's place), every dial
// guarded, and at most 5 redirects, each to http(s).
var remoteClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, Control: guardDial}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: time.Minute,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return fmt.Errorf("refused a redirect to %s", req.URL.Scheme)
		}
		return nil
	},
}

// downloadClient is the client a send's URL is fetched with.
func downloadClient(ctx context.Context) *http.Client {
	if fromRemote(ctx) {
		return remoteClient
	}
	return http.DefaultClient
}
