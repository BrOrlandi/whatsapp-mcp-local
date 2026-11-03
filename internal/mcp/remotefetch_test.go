package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/remote"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

func TestInternalAddr(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1": true, "::1": true, "10.0.0.5": true, "172.18.0.2": true, "192.168.1.10": true,
		"169.254.169.254": true, "100.100.100.200": true, "0.0.0.0": true, "::": true, "fd00:ec2::254": true,
		"fe80::1": true, "::ffff:127.0.0.1": true, "::ffff:192.168.0.1": true, "64:ff9b::a00:1": true,
		"224.0.0.1": true, "255.255.255.255": true,
		"8.8.8.8": false, "1.1.1.1": false, "2606:4700:4700::1111": false, "::ffff:8.8.8.8": false,
	} {
		if got := internalAddr(netip.MustParseAddr(addr)); got != want {
			t.Errorf("internalAddr(%s) = %v, want %v", addr, got, want)
		}
	}
}

func remoteCtx() context.Context {
	return remote.WithDevice(context.Background(), state.RemoteClient{ID: "abc", Scope: remote.ScopeTools})
}

// Through remote access, no path on the server is sent, not even one that
// exists, and the refusal says nothing about whether it does.
func TestRemoteSendTakesNoPath(t *testing.T) {
	file := filepath.Join(t.TempDir(), "secret.key")
	if err := os.WriteFile(file, []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{}
	for _, src := range []string{file, "file://" + file, file + ".missing"} {
		if _, _, err := s.fetchFile(remoteCtx(), src, ""); !errors.Is(err, errRemotePath) {
			t.Errorf("%s: err = %v, want the remote refusal", src, err)
		}
	}
	// Locally, the same path is still sent.
	path, cleanup, err := s.fetchFile(context.Background(), file, "")
	if err != nil || path != file {
		t.Fatalf("local send: %q, %v", path, err)
	}
	cleanup()
}

// A URL that reaches the server itself is refused through remote access,
// by address and by a name that resolves to it, and on a redirect's hop.
func TestRemoteSendReachesOnlyTheInternet(t *testing.T) {
	hit := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit++
		_, _ = w.Write([]byte("internal"))
	}))
	defer srv.Close()
	port := srv.URL[strings.LastIndex(srv.URL, ":"):]
	s := &Server{}
	for _, u := range []string{srv.URL + "/a.png", "http://localhost" + port + "/a.png", "http://[::ffff:127.0.0.1]" + port + "/a.png"} {
		if _, _, err := s.fetchFile(remoteCtx(), u, ""); err == nil || !strings.Contains(err.Error(), "refused") {
			t.Errorf("%s: err = %v, want a refusal", u, err)
		}
	}
	if hit != 0 {
		t.Fatalf("the internal server was reached %d times", hit)
	}
	// Locally, the same URL is downloaded.
	path, cleanup, err := s.fetchFile(context.Background(), srv.URL+"/a.png", "")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if b, _ := os.ReadFile(path); string(b) != "internal" {
		t.Fatalf("local download = %q", b)
	}
}

func TestRemoteRedirectGuarded(t *testing.T) {
	via := []*http.Request{{}}
	for _, target := range []string{"ftp://example.com/a", "file:///etc/passwd"} {
		req, _ := http.NewRequest(http.MethodGet, target, nil)
		if err := remoteClient.CheckRedirect(req, via); err == nil {
			t.Errorf("a redirect to %s should be refused", target)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://example.com/a", nil)
	if err := remoteClient.CheckRedirect(req, make([]*http.Request, 5)); err == nil {
		t.Error("a sixth redirect should be refused")
	}
	// Every hop dials through the same guard.
	if err := guardDial("tcp", "192.168.0.1:80", nil); err == nil {
		t.Error("a hop to the LAN should be refused")
	}
	if err := guardDial("tcp", "[2606:4700:4700::1111]:443", nil); err != nil {
		t.Errorf("a public address: %v", err)
	}
}
