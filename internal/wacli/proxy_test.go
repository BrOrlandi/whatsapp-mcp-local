package wacli

import (
	"bufio"
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/egress"
)

func TestProxyReachesPairingSyncAndCommands(t *testing.T) {
	store, cli := fakeStore(t)
	cli.Proxy, _ = egress.Parse("socks5://100.101.102.103:1080")
	t.Setenv("FAKE_WACLI_RECORD_PROXY", "1")
	t.Setenv("HTTPS_PROXY", "http://old.example:8080")
	t.Setenv("NO_PROXY", "*")
	if !cli.Supports(context.Background(), "--webhook", "sync") {
		t.Fatal("help command failed")
	}
	s := NewSupervisor(cli, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	waitState(t, s, "not_paired")
	if err := s.StartPairing(""); err != nil {
		t.Fatal(err)
	}
	waitState(t, s, "connected")
	if _, err := cli.Run(ctx, "send", "text", "--to", "x", "--message", "hi"); err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(filepath.Join(store, "proxy.log"))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(log)), "\n") {
		var entry struct {
			Command string `json:"command"`
			Proxy   string `json:"https_proxy"`
			Bypass  string `json:"no_proxy"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		if entry.Proxy != string(cli.Proxy) || entry.Bypass != "localhost,127.0.0.1,::1" {
			t.Fatalf("%s did not get the chosen proxy", entry.Command)
		}
		for _, command := range []string{"sync --help", "auth --events", "sync --follow", "send text"} {
			if strings.HasPrefix(entry.Command, command) {
				if command == "send text" && !strings.Contains(entry.Command, "--no-preview") {
					t.Fatal("proxied sends must not fetch link previews through wacli's direct transport")
				}
				seen[command] = true
			}
		}
	}
	if len(seen) != 4 {
		t.Fatalf("did not exercise every subprocess entry: %v", seen)
	}
	if os.Getenv("HTTPS_PROXY") != "http://old.example:8080" || os.Getenv("NO_PROXY") != "*" {
		t.Fatal("WhatsApp changed the parent environment")
	}
}

// Use an HTTPS origin and real CONNECT/SOCKS5 tunnels to prove that the
// proxy environment directs the child transport, including remote DNS and
// proxy authentication. No request reaches the internet or a live account.
func TestChildHTTPSUsesHTTPAndSOCKS5Proxies(t *testing.T) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "example.com" || r.URL.Path != "/media" {
			t.Errorf("unexpected origin request: %s %s", r.Host, r.URL.Path)
		}
		fmt.Fprint(w, "through-proxy")
	}))
	defer origin.Close()
	target := strings.TrimPrefix(origin.URL, "https://")
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: origin.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_WACLI_PROBE_CA", caPath)
	t.Setenv("FAKE_WACLI_PROBE_URL", "https://example.com/media")
	t.Setenv("HTTPS_PROXY", "http://old.invalid:8080")
	t.Setenv("NO_PROXY", "*")
	t.Setenv("no_proxy", "*")

	for _, tlsProxy := range []bool{false, true} {
		name, scheme := "HTTP CONNECT with authentication", "http://"
		if tlsProxy {
			name, scheme = "HTTPS CONNECT with authentication", "https://"
		}
		t.Run(name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "CONNECT" || r.Host != "example.com:443" || r.Header.Get("Proxy-Authorization") != "Basic ZGVtbzp0ZXN0cGFzcw==" {
					t.Errorf("proxy did not receive the intended CONNECT or authentication")
					http.Error(w, "invalid CONNECT", 400)
					return
				}
				conn, buffer, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				fmt.Fprint(buffer, "HTTP/1.1 200 Connection Established\r\n\r\n")
				buffer.Flush()
				tunnel(conn, target)
			})
			var proxy *httptest.Server
			if tlsProxy {
				proxy = httptest.NewTLSServer(handler)
				certs := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: origin.Certificate().Raw})
				certs = append(certs, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: proxy.Certificate().Raw})...)
				proxyCA := filepath.Join(t.TempDir(), "proxy-ca.pem")
				if err := os.WriteFile(proxyCA, certs, 0o600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("FAKE_WACLI_PROBE_CA", proxyCA)
			} else {
				proxy = httptest.NewServer(handler)
			}
			defer proxy.Close()
			probeProxy(t, strings.Replace(proxy.URL, scheme, scheme+"demo:testpass@", 1))
		})
	}

	for _, scheme := range []string{"socks5", "socks5h"} {
		t.Run(scheme+" with remote DNS and authentication", func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(10 * time.Second))
				if err := socksHandshake(conn); err != nil {
					t.Error(err)
					return
				}
				tunnel(conn, target)
			}()
			defer func() { listener.Close(); <-done }()
			probeProxy(t, scheme+"://demo:testpass@"+listener.Addr().String())
		})
	}

	t.Run("failed CONNECT is an error", func(t *testing.T) {
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "proxy unavailable", http.StatusServiceUnavailable)
		}))
		defer proxy.Close()
		_, cli := fakeStore(t)
		cli.Proxy, _ = egress.Parse(strings.Replace(proxy.URL, "http://", "http://demo:private-password@", 1))
		if _, err := cli.Run(context.Background(), "network", "probe"); err == nil {
			t.Fatal("failed proxy must not succeed via a direct connection")
		} else if strings.Contains(err.Error(), "private-password") {
			t.Fatal("proxy failure exposed the configured password")
		}
	})
}

func probeProxy(t *testing.T, endpoint string) {
	t.Helper()
	_, cli := fakeStore(t)
	var err error
	cli.Proxy, err = egress.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	}
	if err := cli.Decode(context.Background(), &result, "network", "probe"); err != nil {
		t.Fatal(err)
	}
	if result.Status != 200 || result.Body != "through-proxy" {
		t.Fatalf("unexpected probe result: %+v", result)
	}
}

func tunnel(conn net.Conn, target string) {
	defer conn.Close()
	remote, err := net.DialTimeout("tcp", target, 5*time.Second)
	if err != nil {
		return
	}
	defer remote.Close()
	go io.Copy(conn, remote)
	io.Copy(remote, conn)
}

func socksHandshake(conn net.Conn) error {
	r := bufio.NewReader(conn)
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return err
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(r, methods); err != nil {
		return err
	}
	conn.Write([]byte{5, 2}) // username/password authentication
	if _, err := io.ReadFull(r, header); err != nil {
		return err
	}
	username := make([]byte, int(header[1]))
	io.ReadFull(r, username)
	length, err := r.ReadByte()
	if err != nil {
		return err
	}
	password := make([]byte, int(length))
	io.ReadFull(r, password)
	if string(username) != "demo" || string(password) != "testpass" {
		return fmt.Errorf("SOCKS5 authentication was not passed through")
	}
	conn.Write([]byte{1, 0})
	request := make([]byte, 5)
	if _, err := io.ReadFull(r, request); err != nil {
		return err
	}
	if request[0] != 5 || request[1] != 1 || request[3] != 3 {
		return fmt.Errorf("expected SOCKS5 CONNECT with a remote domain name")
	}
	address := make([]byte, int(request[4])+2)
	if _, err := io.ReadFull(r, address); err != nil {
		return err
	}
	if string(address[:len(address)-2]) != "example.com" || address[len(address)-2] != 1 || address[len(address)-1] != 187 {
		return fmt.Errorf("unexpected SOCKS5 destination")
	}
	_, err = conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
	return err
}
