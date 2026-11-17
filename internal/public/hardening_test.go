package public

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Wrong passwords from one IPv6 /64 count together: a machine holds the
// whole /64, and rotating addresses inside it gives no fresh tries.
func TestWrongPasswordsCountByIPv6Prefix(t *testing.T) {
	s, _ := testServer(t, "https://wa.example.com", "")
	now := time.Now()
	s.throttle.now = func() time.Time { return now }
	codes := map[int]int{}
	for i := 1; i <= 10; i++ {
		rec := signIn(s, "127.0.0.1:40000", "wrong password!", map[string]string{"X-Forwarded-For": fmt.Sprintf("2001:db8:1:2::%x", i)})
		codes[rec.Code]++
	}
	if codes[http.StatusUnauthorized] > freeTries {
		t.Fatalf("10 addresses of one /64 had %d password checks, want at most %d: %v", codes[http.StatusUnauthorized], freeTries, codes)
	}
	// Another /64 is another address.
	if rec := signIn(s, "127.0.0.1:40000", testPassword, map[string]string{"X-Forwarded-For": "2001:db8:1:3::1"}); rec.Code != http.StatusSeeOther {
		t.Fatalf("another /64: %d", rec.Code)
	}
}

// documentServer serves a client document, counting the fetches; slow makes
// it hold every answer that long.
func documentServer(t *testing.T, s *Server, slow time.Duration) *atomic.Int32 {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		time.Sleep(slow)
		fmt.Fprintf(w, `{"client_id":"https://%s%s","client_name":"Doc","redirect_uris":["https://doc.example/cb"]}`, r.Host, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	addr := srv.Listener.Addr().String()
	s.documents.client = &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		}}}
	return &n
}

// A request that hangs up while the document is fetched does not leave a
// failure behind: the next request for that client gets it.
func TestADroppedRequestDoesNotBlockAClient(t *testing.T) {
	s, _, _ := agentServer(t)
	documentServer(t, s, 300*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, _ = s.resolveClient(ctx, "https://doc.example/client.json", "198.51.100.7")
	if _, err := s.resolveClient(context.Background(), "https://doc.example/client.json", "198.51.100.7"); err != nil {
		t.Fatalf("after a dropped request, the client is refused: %v", err)
	}
}

// A full cache drops its oldest entry, not every entry.
func TestTheDocumentCacheDropsOnlyTheOldest(t *testing.T) {
	s, _, _ := agentServer(t)
	fetches := documentServer(t, s, 0)
	ctx := context.Background()
	if _, err := s.documents.get(ctx, "https://doc.example/kept.json", "203.0.113.1"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1100; i++ {
		// Each from its own address, under the per-address limit.
		_, _ = s.documents.get(ctx, fmt.Sprintf("https://doc.example/%d.json", i), fmt.Sprintf("198.51.%d.%d", i/250, i%250+1))
		if i%100 == 0 {
			// Kept in use, so it is never the oldest.
			_, _ = s.documents.get(ctx, "https://doc.example/kept.json", "203.0.113.1")
		}
	}
	_, _ = s.documents.get(ctx, "https://doc.example/kept.json", "203.0.113.1")
	// Each document once: a wipe would have fetched the one in use again.
	if n := fetches.Load(); n != 1101 {
		t.Fatalf("%d fetches for 1101 documents: the cache was wiped", n)
	}
}

// globalLimitServer is a server whose password checks run on a stopped
// clock, with no waiting for a turn, counting the bcrypt checks and the
// warnings logged.
func globalLimitServer(t *testing.T) (*Server, *atomic.Int32, *bytes.Buffer) {
	t.Helper()
	s, _ := testServer(t, "https://wa.example.com", "")
	now := time.Now()
	s.checks.now = func() time.Time { return now }
	s.checks.maxWait = 0
	var log bytes.Buffer
	s.checks.logger = slog.New(slog.NewTextHandler(&log, nil))
	var checked atomic.Int32
	old := verifyPassword
	verifyPassword = func(hash, password string) bool { checked.Add(1); return old(hash, password) }
	t.Cleanup(func() { verifyPassword = old })
	return s, &checked, &log
}

func from(addr string) map[string]string { return map[string]string{"X-Forwarded-For": addr} }

// Many addresses together get at most the global number of password checks;
// the owner, from the address the right password came from before, still
// gets in, and the attack shows in the log once.
func TestPasswordChecksHaveAGlobalLimit(t *testing.T) {
	s, checked, log := globalLimitServer(t)
	const owner = "203.0.113.50"
	if rec := signIn(s, "127.0.0.1:40000", testPassword, from(owner)); rec.Code != http.StatusSeeOther {
		t.Fatalf("the owner's first sign-in: %d", rec.Code)
	}
	codes := map[int]int{}
	for i := 1; i <= 60; i++ {
		codes[signIn(s, "127.0.0.1:40000", "wrong password!", from(fmt.Sprintf("198.51.100.%d", i))).Code]++
	}
	if int(checked.Load()) != checksBurst || codes[http.StatusTooManyRequests] != 60-(checksBurst-1) {
		t.Fatalf("%d bcrypt checks for 61 tries, want %d: %v", checked.Load(), checksBurst, codes)
	}
	// The owner, from the same address as before, is not kept out.
	if rec := signIn(s, "127.0.0.1:40000", testPassword, from(owner)); rec.Code != http.StatusSeeOther {
		t.Fatalf("the owner during the flood: %d", rec.Code)
	}
	// Someone else still waits.
	if rec := signIn(s, "127.0.0.1:40000", testPassword, from("198.51.100.200")); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("a new address during the flood: %d", rec.Code)
	}
	if n := strings.Count(log.String(), "someone may be guessing"); n != 1 {
		t.Fatalf("%d warnings in the log, want 1:\n%s", n, log.String())
	}
}

// The global limit refills: one check every few seconds.
func TestTheGlobalLimitRefills(t *testing.T) {
	s, _, _ := globalLimitServer(t)
	now := time.Now()
	s.checks.now = func() time.Time { return now }
	for i := 1; i <= checksBurst; i++ {
		signIn(s, "127.0.0.1:40000", "wrong password!", from(fmt.Sprintf("198.51.100.%d", i)))
	}
	if rec := signIn(s, "127.0.0.1:40000", testPassword, from("198.51.100.99")); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("an empty bucket: %d", rec.Code)
	}
	now = now.Add(checksEvery)
	if rec := signIn(s, "127.0.0.1:40000", testPassword, from("198.51.100.99")); rec.Code != http.StatusSeeOther {
		t.Fatalf("after a refill: %d", rec.Code)
	}
}

// At most a couple of bcrypt checks run at once, however many come in.
func TestFewPasswordChecksRunAtOnce(t *testing.T) {
	s, _ := testServer(t, "https://wa.example.com", "")
	var running, most atomic.Int32
	old := verifyPassword
	verifyPassword = func(hash, password string) bool {
		n := running.Add(1)
		for m := most.Load(); n > m && !most.CompareAndSwap(m, n); m = most.Load() {
		}
		time.Sleep(50 * time.Millisecond)
		running.Add(-1)
		return false
	}
	t.Cleanup(func() { verifyPassword = old })
	var wg sync.WaitGroup
	for i := 1; i <= 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			signIn(s, "127.0.0.1:40000", "wrong password!", from(fmt.Sprintf("198.51.100.%d", i)))
		}()
	}
	wg.Wait()
	if m := most.Load(); m > concurrent || m == 0 {
		t.Fatalf("%d bcrypt checks ran at once, want at most %d", m, concurrent)
	}
}

// A refresh replaces the access token: the old one stops, and its row goes.
func TestARefreshReplacesTheAccessToken(t *testing.T) {
	s, _, _ := agentServer(t)
	c := registerClient(t, s, "https://claude.ai/api/mcp/auth_callback")
	verifier, challenge := pkce("r")
	q := c.authorizeQuery(challenge, c.redirect)
	_, back := c.approve(q, testPassword, "full")
	_, tok := c.token(url.Values{"grant_type": {"authorization_code"}, "code": {back.Get("code")}, "client_id": {c.id},
		"redirect_uri": {c.redirect}, "code_verifier": {verifier}})
	old := tok["access_token"].(string)
	_, tok2 := c.token(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tok["refresh_token"].(string)}, "client_id": {c.id}})
	if tok2["access_token"] == nil {
		t.Fatalf("refresh: %v", tok2)
	}
	if rec := call2(s, old); rec.Code != http.StatusUnauthorized {
		t.Fatalf("the access token before the refresh still works: %d", rec.Code)
	}
	if rec := call2(s, tok2["access_token"].(string)); rec.Code != http.StatusOK {
		t.Fatalf("the new access token: %d", rec.Code)
	}
}

// What a request without a credential makes the server remember is
// bounded, and a full table forgets only its oldest entry.
func TestTablesWithoutACredentialAreBounded(t *testing.T) {
	th := newThrottle()
	for i := 0; i < throttleSize*3; i++ {
		th.try(fmt.Sprintf("10.%d.%d.%d", i>>16&255, i>>8&255, i&255), false)
	}
	if n := th.m.len(); n > throttleSize {
		t.Fatalf("the throttle holds %d addresses", n)
	}
	w := newWindow(100)
	w.allow("192.0.2.1", 1)
	for i := 0; i < 500; i++ {
		w.allow(fmt.Sprintf("198.51.%d.%d", i/250, i%250+1), 1)
		w.allow("192.0.2.1", 1) // kept recent
	}
	if n := w.hits.len(); n > 100 {
		t.Fatalf("the window holds %d addresses", n)
	}
	if w.allow("192.0.2.1", 1) {
		t.Fatal("a full table forgot an address it still had in use")
	}
	// IPv6 addresses count by their /64.
	for i := 0; i < 50; i++ {
		w.allow(fmt.Sprintf("2001:db8:5:6::%x", i+1), 1000)
	}
	if w.allow("2001:db8:5:6::ffff", 50) {
		t.Fatal("the /64 did not count its addresses together")
	}
}
