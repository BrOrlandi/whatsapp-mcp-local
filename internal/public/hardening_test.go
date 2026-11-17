package public

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
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

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
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
// the owner, from the browser the right password was typed in before, still
// gets in, and the attack shows in the log once.
func TestPasswordChecksHaveAGlobalLimit(t *testing.T) {
	s, checked, log := globalLimitServer(t)
	const owner = "203.0.113.50"
	first := signIn(s, "127.0.0.1:40000", testPassword, from(owner))
	if first.Code != http.StatusSeeOther {
		t.Fatalf("the owner's first sign-in: %d", first.Code)
	}
	known := browserCookie(t, first)
	codes := map[int]int{}
	for i := 1; i <= 60; i++ {
		codes[signIn(s, "127.0.0.1:40000", "wrong password!", from(fmt.Sprintf("198.51.100.%d", i))).Code]++
	}
	if int(checked.Load()) != checksBurst || codes[http.StatusTooManyRequests] != 60-(checksBurst-1) {
		t.Fatalf("%d bcrypt checks for 61 tries, want %d: %v", checked.Load(), checksBurst, codes)
	}
	// The owner, from the same browser as before, is not kept out.
	if rec := signIn(s, "127.0.0.1:40000", testPassword, map[string]string{"X-Forwarded-For": owner, "Cookie": known}); rec.Code != http.StatusSeeOther {
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

// A code replayed while its first exchange is still keeping the connection
// ends that connection: the replay never misses it.
func TestACodeReplayedDuringItsExchange(t *testing.T) {
	s, _, _ := agentServer(t)
	c := registerClient(t, s, "https://claude.ai/api/mcp/auth_callback")
	verifier, challenge := pkce("x")
	_, back := c.approve(c.authorizeQuery(challenge, c.redirect), testPassword, "full")
	form := url.Values{"grant_type": {"authorization_code"}, "code": {back.Get("code")}, "client_id": {c.id},
		"redirect_uri": {c.redirect}, "code_verifier": {verifier}}
	replayed := 0
	afterCodeClaimed = func() {
		replayed++
		if rec, _ := c.token(form); rec.Code != http.StatusBadRequest {
			t.Errorf("the replay: %d", rec.Code)
		}
	}
	defer func() { afterCodeClaimed = func() {} }()
	_, tok := c.token(form)
	if replayed != 1 {
		t.Fatalf("the replay ran %d times", replayed)
	}
	if at, _ := tok["access_token"].(string); at != "" {
		if rec := call2(s, at); rec.Code != http.StatusUnauthorized {
			t.Fatalf("the connection survived a replay during its exchange: %d", rec.Code)
		}
	}
}

// A registered client chooses its own name: the consent page leads with the
// host the person is sent back to, shows the name as the app's own claim,
// asks for the password the first time that client and host are approved
// (even when signed in), and preselects no access.
func TestTheConsentPageCannotBeImpersonated(t *testing.T) {
	s, _, _ := agentServer(t)
	c := registerClient(t, s, "https://claude-ai.attacker.example/cb")
	_, challenge := pkce("i")
	q := c.authorizeQuery(challenge, c.redirect)
	cookie := sessionCookie(t, signIn(s, "127.0.0.1:40000", testPassword, nil))
	session := map[string]string{"Cookie": CookieName + "=" + cookie.Value}
	page := serve(s, fromTunnel("GET", "/oauth/authorize?"+q.Encode(), session, "")).Body.String()
	if strings.Contains(page, "Claude quer usar") || !strings.Contains(page, "claude-ai.attacker.example quer usar") {
		t.Errorf("the headline is not the host the person goes back to:\n%s", page)
	}
	if !strings.Contains(page, "não é verificado") {
		t.Error("the app's own name is not marked as unverified")
	}
	if strings.Contains(page, "checked") {
		t.Error("an access level is preselected")
	}
	if !strings.Contains(page, `type="password"`) {
		t.Error("the first approval of this client does not ask for the password")
	}
	post := func(extra map[string]string, withPassword bool) *httptest.ResponseRecorder {
		form := url.Values{"decision": {"allow"}}
		for k, v := range q {
			form[k] = v
		}
		for k, v := range extra {
			form.Set(k, v)
		}
		if withPassword {
			form.Set("password", testPassword)
		}
		return serve(s, fromTunnel("POST", "/oauth/authorize", map[string]string{"Origin": origin, "Cookie": CookieName + "=" + cookie.Value}, form.Encode()))
	}
	// Signed in, without the password, the first time: no code.
	if rec := post(map[string]string{"access": "full"}, false); rec.Header().Get("Location") != "" {
		t.Errorf("a first approval with the session alone gave a code: %q", rec.Header().Get("Location"))
	}
	// No access chosen: no code.
	if rec := post(nil, true); rec.Header().Get("Location") != "" {
		t.Errorf("an approval with no access chosen gave a code: %q", rec.Header().Get("Location"))
	}
	if rec := post(map[string]string{"access": "everything"}, true); rec.Header().Get("Location") != "" {
		t.Errorf("an unknown access gave a code: %q", rec.Header().Get("Location"))
	}
	// With the password and a choice: a code. After that, the session is
	// enough for this client and host.
	if rec := post(map[string]string{"access": "read"}, true); !strings.Contains(rec.Header().Get("Location"), "code=") {
		t.Fatalf("a full approval: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := post(map[string]string{"access": "read"}, false); !strings.Contains(rec.Header().Get("Location"), "code=") {
		t.Fatalf("a later approval with the session: %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

// A client known by its metadata document leads with its document's host.
func TestTheConsentPageNamesTheDocumentHost(t *testing.T) {
	s, _, _ := agentServer(t)
	documentServer(t, s, 0)
	c := oauthClient{t: t, s: s, id: "https://doc.example/client.json", redirect: "https://doc.example/cb"}
	_, challenge := pkce("j")
	page := serve(s, fromTunnel("GET", "/oauth/authorize?"+c.authorizeQuery(challenge, c.redirect).Encode(), nil, "")).Body.String()
	if !strings.Contains(page, "doc.example quer usar") || !strings.Contains(page, "Doc") {
		t.Errorf("the headline is not the document's host:\n%s", page)
	}
}

// The consent page lets its own form post with its origin: a no-referrer
// page would make the browser send Origin: null, which is refused.
func TestTheConsentPageKeepsItsOrigin(t *testing.T) {
	s, _, _ := agentServer(t)
	c := registerClient(t, s, "https://claude.ai/api/mcp/auth_callback")
	_, challenge := pkce("o")
	rec := serve(s, fromTunnel("GET", "/oauth/authorize?"+c.authorizeQuery(challenge, c.redirect).Encode(), nil, ""))
	if p := rec.Header().Get("Referrer-Policy"); p != "same-origin" {
		t.Fatalf("Referrer-Policy: %q", p)
	}
}

// The public server reads a request within its read timeout, then lets a
// long tool call run; a body too large is refused, and a client that stalls
// mid-body is cut off.
func TestThePublicServerBoundsSlowRequests(t *testing.T) {
	old := readTimeout
	readTimeout = 300 * time.Millisecond
	t.Cleanup(func() { readTimeout = old })
	s, st, _ := agentServer(t)
	key, _, _ := CreateKey(context.Background(), st, "x", false, time.Now())
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			http.Error(w, "too large", http.StatusRequestEntityTooLarge)
			return
		}
		select {
		case <-time.After(800 * time.Millisecond):
			_, _ = w.Write([]byte("alive"))
		case <-r.Context().Done():
			_, _ = w.Write([]byte("cancelled"))
		}
	})
	s.routes = mux
	srv := NewHTTPServer(s)
	if srv.MaxHeaderBytes > 64<<10 || srv.IdleTimeout == 0 || srv.ReadTimeout == 0 || srv.WriteTimeout != 0 {
		t.Fatalf("the server: %+v", srv)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	base := "http://" + ln.Addr().String()
	post := func(body string) (*http.Response, error) {
		req, _ := http.NewRequest("POST", base+"/mcp", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("X-Forwarded-Proto", "https")
		return http.DefaultClient.Do(req)
	}
	// A call longer than the read timeout runs to its end.
	resp, err := post(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != "alive" {
		t.Fatalf("a long call: %d %s", resp.StatusCode, b)
	}
	// A body over the limit.
	resp, err = post(strings.Repeat("x", maxBody+10))
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode != http.StatusRequestEntityTooLarge {
			t.Fatalf("a large body: %d", resp.StatusCode)
		}
	}
	// A client that stops halfway through its body is cut off.
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "POST /mcp HTTP/1.1\r\nHost: x\r\nX-Forwarded-Proto: https\r\nAuthorization: Bearer %s\r\nContent-Length: 1000\r\n\r\nabc", key)
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	start := time.Now()
	reply, _ := io.ReadAll(conn)
	if time.Since(start) > 2*time.Second || strings.Contains(string(reply), "alive") {
		t.Fatalf("a stalled body was waited for %v: %q", time.Since(start), reply)
	}
}

// A Bearer that is not shaped like a key or token is not looked up; wrong
// ones are logged once a minute, and never with a media link's secret.
func TestWrongBearersAreCheapAndQuiet(t *testing.T) {
	s, st, _ := agentServer(t)
	var log bytes.Buffer
	s.logger = slog.New(slog.NewTextHandler(&log, nil))
	var lookups atomic.Int32
	old := authenticate
	authenticate = func(a *access, ctx context.Context, bearer string) (caller.Caller, bool) {
		lookups.Add(1)
		return old(a, ctx, bearer)
	}
	t.Cleanup(func() { authenticate = old })
	for _, b := range []string{"nonsense", KeyPrefix + "short", KeyPrefix + strings.Repeat("!", 43), "wamcp_rt_" + strings.Repeat("a", 43)} {
		serve(s, fromTunnel("POST", "/mcp", map[string]string{"Authorization": "Bearer " + b, "X-Forwarded-For": "203.0.113.1"}, "{}"))
	}
	if n := lookups.Load(); n != 0 {
		t.Fatalf("%d lookups for credentials that cannot be valid", n)
	}
	// A well-formed one is looked up, even from a slowed-down address, so a
	// valid key there still works.
	key, _, _ := CreateKey(context.Background(), st, "x", false, time.Now())
	for i := 0; i < 10; i++ {
		serve(s, fromTunnel("POST", "/mcp", map[string]string{"Authorization": "Bearer " + KeyPrefix + strings.Repeat("a", 43), "X-Forwarded-For": "203.0.113.2"}, "{}"))
	}
	if rec := serve(s, fromTunnel("POST", "/mcp", map[string]string{"Authorization": "Bearer " + key, "X-Forwarded-For": "203.0.113.2"}, "{}")); rec.Code != 200 {
		t.Fatalf("a valid key from a slowed-down address: %d", rec.Code)
	}
	secretLink := "/media/" + strings.Repeat("ab", 24)
	serve(s, fromTunnel("GET", secretLink, map[string]string{"Authorization": "Bearer nonsense"}, ""))
	if n := strings.Count(log.String(), "refused a wrong key or token"); n != 1 {
		t.Fatalf("%d warnings for 15 wrong credentials, want 1:\n%s", n, log.String())
	}
	if strings.Contains(log.String(), strings.Repeat("ab", 5)) {
		t.Fatal("the log holds part of a media link's secret")
	}
	// Once the minute is up, the next line says how many were left out, and
	// a media link's path is shown without its secret.
	now := time.Now().Add(2 * time.Minute)
	s.now = func() time.Time { return now }
	serve(s, fromTunnel("GET", secretLink, map[string]string{"Authorization": "Bearer nonsense"}, ""))
	if !strings.Contains(log.String(), "path=/media/…") || strings.Contains(log.String(), strings.Repeat("ab", 5)) {
		t.Fatalf("the media path in the log:\n%s", log.String())
	}
}

// Being turned away while the server is busy is not a wrong password: an
// owner who retries during a flood is not pushed into the long backoff.
func TestBusyIsNotAWrongPassword(t *testing.T) {
	s, _, _ := globalLimitServer(t)
	now := time.Now()
	s.checks.now = func() time.Time { return now }
	s.throttle.now = func() time.Time { return now }
	for i := 1; i <= checksBurst; i++ {
		signIn(s, "127.0.0.1:40000", "wrong password!", from(fmt.Sprintf("198.51.100.%d", i)))
	}
	for i := 0; i < 10; i++ {
		if rec := signIn(s, "127.0.0.1:40000", testPassword, from("203.0.113.77")); rec.Code != http.StatusTooManyRequests {
			t.Fatalf("a busy server: %d", rec.Code)
		}
	}
	if w := s.throttle.wait("203.0.113.77"); w > 0 {
		t.Fatalf("ten busy answers put the address in a %v wait", w)
	}
	now = now.Add(checksEvery)
	if rec := signIn(s, "127.0.0.1:40000", testPassword, from("203.0.113.77")); rec.Code != http.StatusSeeOther {
		t.Fatalf("the right password once a turn came: %d", rec.Code)
	}
}

// Refusing needs no access choice: the deny button skips the browser's
// required check, and the server sends the refusal back.
func TestRefusingNeedsNoChoice(t *testing.T) {
	s, _, _ := agentServer(t)
	c := registerClient(t, s, "https://claude.ai/api/mcp/auth_callback")
	_, challenge := pkce("d")
	q := c.authorizeQuery(challenge, c.redirect)
	page := serve(s, fromTunnel("GET", "/oauth/authorize?"+q.Encode(), nil, "")).Body.String()
	if !strings.Contains(page, `value="deny" formnovalidate`) {
		t.Fatal("the deny button is held back by the required choice")
	}
	form := url.Values{"decision": {"deny"}}
	for k, v := range q {
		form[k] = v
	}
	rec := serve(s, fromTunnel("POST", "/oauth/authorize", map[string]string{"Origin": origin}, form.Encode()))
	if loc, _ := url.Parse(rec.Header().Get("Location")); loc == nil || loc.Query().Get("error") != "access_denied" {
		t.Fatalf("a refusal with no choice: %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

// browserCookie is the known-browser cookie a right password set, as a
// Cookie header.
func browserCookie(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == BrowserCookie && c.Value != "" {
			return c.Name + "=" + c.Value
		}
	}
	t.Fatalf("no known-browser cookie: %d", rec.Code)
	return ""
}
