package webhook

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
)

// A webhook reaches this computer and its network, as automations beside
// the app need, but never a cloud's metadata service; a test from outside
// says what went wrong in general words, the details only in the log.
func TestWhatAWebhookReachesAndWhatItsTestSays(t *testing.T) {
	m, _ := manager(t)
	var log bytes.Buffer
	m.logger = slog.New(slog.NewTextHandler(&log, nil))
	outside := caller.With(context.Background(), caller.Caller{Kind: caller.Panel})
	test := func(ctx context.Context, url string) map[string]any {
		t.Helper()
		h, _, err := m.Create(context.Background(), Settings{URL: ptr(url), Events: ptr([]string{"message"})})
		if err != nil {
			t.Fatal(err)
		}
		out, err := m.Test(ctx, h.ID)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	// This computer still receives.
	r := newReceiver(t, func(int32) int { return 200 })
	if out := test(outside, r.srv.URL); out["ok"] != true {
		t.Fatalf("a receiver on this computer: %v", out)
	}

	// The metadata service is refused before any connection.
	for _, url := range []string{"http://169.254.169.254/latest/meta-data/", "http://[fe80::1]/", "http://[fd00:ec2::254]/"} {
		out := test(outside, url)
		if out["ok"] != false || out["result"] != "esse endereço não pode receber webhooks" {
			t.Errorf("%s: %v", url, out)
		}
	}

	// A failure the panel has no words for is said in general ones, from
	// outside; the error's own text goes to the log.
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	t.Cleanup(tlsSrv.Close)
	odd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hj, _ := w.(http.Hijacker)
		c, _, _ := hj.Hijack()
		_, _ = c.Write([]byte("not http at all\r\n\r\n"))
		c.Close()
	}))
	t.Cleanup(odd.Close)
	for url, want := range map[string]string{tlsSrv.URL: "o certificado do endereço não foi aceito", odd.URL: "não foi possível entregar nesse endereço"} {
		out := test(outside, url)
		if out["result"] != want || out["error"] != want {
			t.Errorf("%s from outside: %v", url, out)
		}
	}
	if !strings.Contains(log.String(), "malformed HTTP") {
		t.Errorf("the log lacks the error's own words:\n%s", log.String())
	}
	// On this computer the error's own words come back too.
	if out := test(context.Background(), odd.URL); out["result"] != "não foi possível entregar nesse endereço" || !strings.Contains(out["error"].(string), "malformed HTTP") {
		t.Errorf("a test on this computer: %v", out)
	}
}

// With a proxy from the environment, the proxy dials, so the address is
// never seen here: a metadata host in the URL is refused before it leaves.
func TestThroughAProxyMetadataIsStillRefused(t *testing.T) {
	m, _ := manager(t)
	proxy, _ := url.Parse("http://127.0.0.1:1")
	m.client.Transport.(*http.Transport).Proxy = func(*http.Request) (*url.URL, error) { return proxy, nil }
	for target, want := range map[string]string{
		"http://169.254.169.254/latest/meta-data/": "esse endereço não pode receber webhooks",
		"http://metadata.google.internal/":         "esse endereço não pode receber webhooks",
		"http://[fd00:ec2::254]/":                  "esse endereço não pode receber webhooks",
		"http://wa.example.com/hook":               "nenhum programa respondeu nesse endereço", // the proxy itself
	} {
		h, _, err := m.Create(context.Background(), Settings{URL: ptr(target), Events: ptr([]string{"message"})})
		if err != nil {
			t.Fatal(err)
		}
		out, _ := m.Test(context.Background(), h.ID)
		if out["result"] != want {
			t.Errorf("%s: %v", target, out)
		}
	}
}
