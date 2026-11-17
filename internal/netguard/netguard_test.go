package netguard

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestInternal(t *testing.T) {
	for _, a := range []string{"127.0.0.1", "::1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.100.100.100",
		"fd00::1", "fe80::1", "224.0.0.1", "0.0.0.0", "::", "::ffff:127.0.0.1", "64:ff9b::7f00:1", "198.51.100.1"} {
		if !Internal(netip.MustParseAddr(a)) {
			t.Errorf("%s is internal", a)
		}
	}
	for _, a := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		if Internal(netip.MustParseAddr(a)) {
			t.Errorf("%s is on the internet", a)
		}
	}
}

// The check runs on the address dialled: a server on loopback is refused
// even when asked for by name.
func TestTheClientDoesNotReachThisComputer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	for _, u := range []string{srv.URL, "http://localhost:" + srv.Listener.Addr().(interface{ String() string }).String()[len("127.0.0.1:"):]} {
		_, err := Client(5*time.Second, 0).Get(u)
		if err == nil || !errors.Is(err, ErrInternal) {
			t.Errorf("%s: %v", u, err)
		}
	}
}
