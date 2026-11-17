package netguard

import (
	"net/netip"
	"testing"
)

// The IPv6 forms that carry an IPv4 address inside are not the internet
// either: IPv4-compatible, 6to4 and Teredo.
func TestIPv6FormsCarryingIPv4AreInternal(t *testing.T) {
	for _, a := range []string{"::7f00:1", "::a00:1", "2002:7f00:1::1", "2002:c0a8:101::1", "2001:0:4136:e378:8000:63bf:3fff:fdd2"} {
		if !Internal(netip.MustParseAddr(a)) {
			t.Errorf("%s is internal", a)
		}
	}
	if Internal(netip.MustParseAddr("2001:4860:4860::8888")) {
		t.Error("a public IPv6 address in 2001::/16 outside Teredo is on the internet")
	}
}
