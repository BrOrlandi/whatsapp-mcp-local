package remote

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/flynn/noise"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

type fixture struct {
	t      *testing.T
	srv    *Server
	st     *state.State
	https  *httptest.Server
	served []string
}

// newFixture runs a remote server behind a test HTTPS server whose
// certificate the client is set to trust, as it would trust a real one.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	srv, err := NewServer(t.TempDir(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, srv: srv, st: st}
	srv.Admin = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d, _ := DeviceFrom(r.Context())
		body, _ := io.ReadAll(r.Body)
		f.served = append(f.served, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"path": r.URL.Path, "device": d.Name, "body": string(body),
			"auth": r.Header.Get("Authorization")})
	})
	srv.Tools = srv.Admin
	f.https = httptest.NewTLSServer(srv.Handler())
	t.Cleanup(f.https.Close)
	Transport = f.https.Client().Transport
	t.Cleanup(func() { Transport = nil })
	return f
}

func (f *fixture) url() string { return f.https.URL }

func (f *fixture) code(scope string) string {
	f.t.Helper()
	c, err := f.srv.NewCode("test", scope)
	if err != nil {
		f.t.Fatal(err)
	}
	return c.Code
}

func (f *fixture) pair(scope string) (Device, []byte) {
	f.t.Helper()
	d, key, err := Pair(context.Background(), f.url(), f.code(scope), "Mac de teste")
	if err != nil {
		f.t.Fatalf("pair: %v", err)
	}
	return d, key
}

// connected starts a client and waits for its session.
func connected(t *testing.T, d Device, key []byte) (*Client, context.CancelFunc) {
	t.Helper()
	c, err := NewClient(d, key)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go c.Run(ctx)
	waitState(t, c, StateConnected)
	return c, cancel
}

func waitState(t *testing.T, c *Client, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if c.Status().State == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("state %+v, want %s", c.Status(), want)
}

func get(t *testing.T, c *Client, path string) (int, map[string]string, error) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, "http://local"+path, strings.NewReader(`{"x":1}`))
	req.Header.Set("Authorization", "Bearer from-the-local-side")
	resp, err := c.RoundTrip(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	var out map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out, nil
}

func TestPairAndUseTheServer(t *testing.T) {
	f := newFixture(t)
	d, key := f.pair(ScopeAdmin)
	if d.ID == "" || d.Scope != ScopeAdmin || len(d.ServerKey) != 32 || !strings.HasPrefix(d.Server, "https://") {
		t.Fatalf("device %+v", d)
	}
	c, stop := connected(t, d, key)
	defer stop()
	code, out, err := get(t, c, "/api/status?x=1")
	if err != nil || code != 200 || out["path"] != "/api/status" || out["device"] != "Mac de teste" || out["body"] != `{"x":1}` {
		t.Fatalf("a request over the session: %d %v %v", code, out, err)
	}
	// A program on the app's computer presents the server's token as it
	// would on the server: it crosses, for the server to check.
	if out["auth"] != "Bearer from-the-local-side" {
		t.Fatalf("the program's credential did not reach the server: %q", out["auth"])
	}
	devices, _ := f.st.RemoteClients(context.Background())
	if len(devices) != 1 || devices[0].LastSeen == nil {
		t.Fatalf("devices %+v", devices)
	}
}

func TestAPairingCodeWorksOnceAndOnlyWhole(t *testing.T) {
	f := newFixture(t)
	code := f.code(ScopeAdmin)
	if _, _, err := Pair(context.Background(), f.url(), code, "a"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Pair(context.Background(), f.url(), code, "b"); err == nil {
		t.Fatal("a code must work once")
	}
	// A code with another secret: the handshake fails, nothing is recorded.
	other := f.code(ScopeAdmin)
	id, secret, fp, _ := decodeCode(other)
	idb, _ := hexDecode(id)
	secret[0] ^= 1
	if _, _, err := Pair(context.Background(), f.url(), encodeCode(idb, secret, append(fp, make([]byte, 24)...)), "c"); err == nil {
		t.Fatal("a wrong secret must be refused")
	}
	// An expired code.
	late := f.code(ScopeAdmin)
	f.srv.Now = func() time.Time { return time.Now().Add(pairTTL + time.Minute) }
	if _, _, err := Pair(context.Background(), f.url(), late, "d"); err == nil {
		t.Fatal("an expired code must be refused")
	}
	devices, _ := f.st.RemoteClients(context.Background())
	if len(devices) != 1 {
		t.Fatalf("%d devices recorded, want only the first", len(devices))
	}
}

// A code whose fingerprint is not the server's key: the device refuses
// the server, and the server records nothing.
func TestThePairingCodePinsTheServerKey(t *testing.T) {
	f := newFixture(t)
	id, secret, fp, _ := decodeCode(f.code(ScopeAdmin))
	idb, _ := hexDecode(id)
	fp[0] ^= 0xff
	if _, _, err := Pair(context.Background(), f.url(), encodeCode(idb, secret, append(fp, make([]byte, 24)...)), "x"); err == nil ||
		!strings.Contains(err.Error(), "does not match") {
		t.Fatalf("a mismatched server key: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if devices, _ := f.st.RemoteClients(context.Background()); len(devices) != 0 {
		t.Fatal("an unconfirmed pairing must not be recorded")
	}
}

func TestHTTPSOnly(t *testing.T) {
	for _, bad := range []string{"http://example.com", "example.com", "ws://example.com", "https://user:pw@example.com", "https://example.com/?x=1"} {
		if _, err := ServerURL(bad); err == nil {
			t.Errorf("%s was accepted", bad)
		}
	}
	if _, _, err := Pair(context.Background(), "http://127.0.0.1:1", "wamcp1-aaaa", "x"); err == nil || !strings.Contains(err.Error(), "https://") {
		t.Errorf("pairing over http: %v", err)
	}
	if _, err := NewClient(Device{Server: "http://example.com", ServerKey: make([]byte, 32)}, make([]byte, 32)); err == nil {
		t.Error("a client for an http:// server")
	}

	// A server that redirects (to plain HTTP, here) is not followed.
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com"+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	Transport = redirect.Client().Transport
	defer func() { Transport = nil }()
	f := &fixture{t: t}
	_ = f
	st, _ := state.Open(t.TempDir())
	defer st.Close()
	srv, _ := NewServer(t.TempDir(), st, nil)
	c, _ := srv.NewCode("x", ScopeAdmin)
	if _, _, err := Pair(context.Background(), redirect.URL, c.Code, "x"); err == nil {
		t.Error("a redirect was followed")
	}

	// A certificate the system does not trust is refused, with no way
	// around it.
	untrusted := httptest.NewTLSServer(srv.Handler())
	defer untrusted.Close()
	Transport = nil
	if _, _, err := Pair(context.Background(), untrusted.URL, c.Code, "x"); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Errorf("an untrusted certificate: %v", err)
	}
}

// Behind a proxy, the remote routes answer only what came over HTTPS, and
// never a web page.
func TestTheRoutesRefusePlainHTTPAndPages(t *testing.T) {
	st, _ := state.Open(t.TempDir())
	defer st.Close()
	srv, _ := NewServer(t.TempDir(), st, nil)
	h := srv.Handler()
	for _, c := range []struct {
		headers map[string]string
		want    int
	}{
		{nil, http.StatusForbidden},
		{map[string]string{"X-Forwarded-Proto": "http"}, http.StatusForbidden},
		{map[string]string{"X-Forwarded-Proto": "ws"}, http.StatusForbidden},
		{map[string]string{"X-Forwarded-Proto": "wss"}, http.StatusOK}, // Traefik, for a WebSocket over TLS
		{map[string]string{"X-Forwarded-Proto": "https", "Origin": "https://evil.example"}, http.StatusForbidden},
		{map[string]string{"X-Forwarded-Proto": "https"}, http.StatusOK},
	} {
		r := httptest.NewRequest(http.MethodGet, "/remote/v1/health", nil)
		for k, v := range c.headers {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%v: %d, want %d", c.headers, w.Code, c.want)
		}
	}
}

func TestRevokedAndUnknownDevicesAreRefused(t *testing.T) {
	f := newFixture(t)
	d, key := f.pair(ScopeAdmin)
	c, stop := connected(t, d, key)
	if err := f.st.RevokeRemoteClient(context.Background(), d.ID); err != nil {
		t.Fatal(err)
	}
	// The open session ends at its next request.
	if code, _, err := get(t, c, "/api/status"); err == nil && code == 200 {
		t.Fatal("a revoked device was served")
	}
	stop()
	again, err := NewClient(d, key)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go again.Run(ctx)
	waitState(t, again, StateRefused)

	// A key the server never paired.
	stranger := make([]byte, 32)
	_, _ = rand.Read(stranger)
	other, _ := NewClient(d, stranger)
	go other.Run(ctx)
	waitState(t, other, StateRefused)
}

func TestToolsReachTheMCPOnly(t *testing.T) {
	f := newFixture(t)
	d, key := f.pair(ScopeTools)
	c, stop := connected(t, d, key)
	defer stop()
	if code, _, _ := get(t, c, "/api/status"); code != http.StatusForbidden {
		t.Errorf("a tools device reached the API: %d", code)
	}
	if code, out, _ := get(t, c, "/mcp"); code != 200 || out["path"] != "/mcp" {
		t.Errorf("a tools device and the MCP: %d %v", code, out)
	}
	// download_media's links are the MCP's too.
	if code, out, _ := get(t, c, "/media/abc"); code != 200 || out["path"] != "/media/abc" {
		t.Errorf("a tools device and a media link: %d %v", code, out)
	}
}

// A handler's panic answers 500 and leaves the session, and the server, up.
func TestAPanicEndsOnlyItsRequest(t *testing.T) {
	f := newFixture(t)
	admin := f.srv.Admin
	f.srv.Admin = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/boom" {
			panic("boom")
		}
		admin.ServeHTTP(w, r)
	})
	d, key := f.pair(ScopeAdmin)
	c, stop := connected(t, d, key)
	defer stop()
	if code, _, err := get(t, c, "/boom"); err != nil || code != http.StatusInternalServerError {
		t.Fatalf("a panic: %d %v", code, err)
	}
	if code, _, err := get(t, c, "/api/status"); err != nil || code != 200 {
		t.Fatalf("after a panic: %d %v", code, err)
	}
}

// The app keeps the last handshake stamp, so a clock that went back does
// not lock the device out: the server refuses a stamp that is not larger.
func TestAClockGoingBackKeepsTheDeviceIn(t *testing.T) {
	f := newFixture(t)
	d, key := f.pair(ScopeAdmin)
	var kept int64
	c, err := NewClient(d, key)
	if err != nil {
		t.Fatal(err)
	}
	c.OnStamp = func(s int64) { kept = s }
	ctx, cancel := context.WithCancel(context.Background())
	go c.Run(ctx)
	waitState(t, c, StateConnected)
	cancel()
	if kept == 0 {
		t.Fatal("the stamp was not handed over to be kept")
	}
	// The app restarts with the clock an hour behind.
	c2, err := NewClient(d, key)
	if err != nil {
		t.Fatal(err)
	}
	c2.Now = func() time.Time { return time.Now().Add(-time.Hour) }
	c2.LastStamp = kept
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go c2.Run(ctx2)
	waitState(t, c2, StateConnected)
}

func TestRotatingTheKey(t *testing.T) {
	f := newFixture(t)
	d, key := f.pair(ScopeAdmin)
	c, stop := connected(t, d, key)
	defer stop()
	next, err := c.Rotate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, c, StateConnected)
	if code, _, err := get(t, c, "/api/status"); err != nil || code != 200 {
		t.Fatalf("after rotating: %d %v", code, err)
	}
	stored, _ := f.st.RemoteClient(context.Background(), d.ID)
	nextPub, _ := publicKey(next)
	if string(stored.PublicKey) != string(nextPub) {
		t.Fatal("the server did not take the new key")
	}
	old, _ := NewClient(d, key)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go old.Run(ctx)
	waitState(t, old, StateRefused)
}

// session opens a session by hand, to send what a client never would.
func session(t *testing.T, f *fixture, d Device, key []byte, stamp int64) (*websocket.Conn, *channel, []byte) {
	t.Helper()
	u, _ := url.Parse(f.url())
	ws, err := dial(context.Background(), u, "/remote/v1/session")
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := publicKey(key)
	hs, _ := noise.NewHandshakeState(noise.Config{CipherSuite: suite, Pattern: noise.HandshakeIK, Initiator: true,
		Prologue: []byte(sessionPrologue), StaticKeypair: noise.DHKey{Private: key, Public: pub}, PeerStatic: d.ServerKey})
	first, _, _, err := hs.WriteMessage(nil, timestamp(stamp))
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Write(context.Background(), websocket.MessageBinary, first); err != nil {
		t.Fatal(err)
	}
	_, reply, err := ws.Read(context.Background())
	if err != nil {
		return ws, nil, first
	}
	_, cs1, cs2, err := hs.ReadMessage(nil, reply)
	if err != nil {
		t.Fatal(err)
	}
	return ws, &channel{ws: ws, send: cs1, recv: cs2}, first
}

func request(path string) []byte {
	head, _ := json.Marshal(reqHead{Method: "GET", Path: path})
	return frame(frameReqHead, 1, head)
}

// A message changed in transit, or sent twice, fails its check and ends the
// session; a handshake replayed is refused.
func TestTamperingAndReplaysAreRefused(t *testing.T) {
	f := newFixture(t)
	d, key := f.pair(ScopeAdmin)
	ctx := context.Background()

	// Tampered.
	ws, ch, _ := session(t, f, d, key, time.Now().UnixNano())
	sealed, _ := ch.send.Encrypt(nil, nil, request("/api/status"))
	sealed[len(sealed)/2] ^= 1
	_ = ws.Write(ctx, websocket.MessageBinary, sealed)
	tctx, tcancel := context.WithTimeout(ctx, 5*time.Second)
	defer tcancel()
	if _, _, err := ws.Read(tctx); err == nil || tctx.Err() != nil {
		t.Errorf("a tampered message must close the session at once: %v", err)
	}
	ws.CloseNow()

	// Replayed: the same ciphertext twice.
	ws, ch, first := session(t, f, d, key, time.Now().UnixNano())
	sealed, _ = ch.send.Encrypt(nil, nil, request("/api/status"))
	end, _ := ch.send.Encrypt(nil, nil, frame(frameReqEnd, 1, nil))
	_ = ws.Write(ctx, websocket.MessageBinary, sealed)
	_ = ws.Write(ctx, websocket.MessageBinary, end)
	for { // the genuine answer, to its end
		msg, err := ch.read(ctx)
		if err != nil {
			t.Fatalf("the genuine request: %v", err)
		}
		if msg[0] == frameRespEnd {
			break
		}
	}
	served := len(f.served)
	_ = ws.Write(ctx, websocket.MessageBinary, sealed)
	_ = ws.Write(ctx, websocket.MessageBinary, end)
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		if _, err := ch.read(rctx); err != nil {
			break
		}
	}
	if len(f.served) != served {
		t.Error("a replayed request was served again")
	}
	ws.CloseNow()

	// The first handshake message again, on a new connection.
	u, _ := url.Parse(f.url())
	ws, err := dial(ctx, u, "/remote/v1/session")
	if err != nil {
		t.Fatal(err)
	}
	_ = ws.Write(ctx, websocket.MessageBinary, first)
	if _, _, err := ws.Read(ctx); err == nil {
		t.Error("a replayed handshake was answered")
	}
	ws.CloseNow()
}

// Both ends rekey after the same number of messages and stay in step.
func TestRekeying(t *testing.T) {
	old := rekeyEvery.Load()
	rekeyEvery.Store(3)
	defer rekeyEvery.Store(old)
	f := newFixture(t)
	d, key := f.pair(ScopeAdmin)
	c, stop := connected(t, d, key)
	defer stop()
	for i := 0; i < 10; i++ {
		if code, _, err := get(t, c, "/api/status"); err != nil || code != 200 {
			t.Fatalf("request %d after rekeys: %d %v", i, code, err)
		}
	}
}

func hexDecode(s string) ([]byte, error) {
	b := make([]byte, len(s)/2)
	for i := range b {
		var v byte
		for _, ch := range s[2*i : 2*i+2] {
			v <<= 4
			switch {
			case ch >= '0' && ch <= '9':
				v |= byte(ch - '0')
			default:
				v |= byte(ch-'a') + 10
			}
		}
		b[i] = v
	}
	return b, nil
}

// Only the trusted proxy reaches the routes: a neighbour on its network,
// with any X-Forwarded-Proto, is refused.
func TestOnlyTheTrustedProxy(t *testing.T) {
	st, _ := state.Open(t.TempDir())
	defer st.Close()
	srv, _ := NewServer(t.TempDir(), st, nil)
	srv.TrustedProxy = TrustProxies("10.0.0.5, 172.18.0.0/30, localhost")
	for _, c := range []struct {
		from string
		want int
	}{
		{"10.0.0.5:4000", http.StatusOK},
		{"172.18.0.2:1", http.StatusOK},
		{"127.0.0.1:9", http.StatusOK}, // localhost, by name
		{"172.18.0.14:5555", http.StatusForbidden},
		{"10.0.0.6:4000", http.StatusForbidden},
	} {
		r := httptest.NewRequest(http.MethodGet, "/remote/v1/health", nil)
		r.RemoteAddr = c.from
		r.Header.Set("X-Forwarded-Proto", "https")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("from %s: %d, want %d", c.from, w.Code, c.want)
		}
	}
	if TrustProxies(" , ") != nil {
		t.Error("an empty list must trust whatever reaches the listener")
	}
	// A list with nothing valid in it is a mistake, and trusts nothing.
	if trust := TrustProxies("172.18.0.0/33"); trust == nil || trust("172.18.0.2:1234") {
		t.Error("an invalid list must trust nothing")
	}
}

// Knowing a code's id (from a log, say) is not enough to spend it: only a
// handshake that proves the secret does.
func TestTheCodeIDAloneDoesNotSpendTheCode(t *testing.T) {
	f := newFixture(t)
	code := f.code(ScopeAdmin)
	id, _, _, _ := decodeCode(code)
	u, _ := url.Parse(f.url())
	ws, err := dial(context.Background(), u, "/remote/v1/pair")
	if err != nil {
		t.Fatal(err)
	}
	_ = ws.Write(context.Background(), websocket.MessageBinary, []byte(id))
	_ = ws.Write(context.Background(), websocket.MessageBinary, []byte("junk"))
	_, _, _ = ws.Read(context.Background())
	ws.CloseNow()
	time.Sleep(100 * time.Millisecond)
	if _, _, err := Pair(context.Background(), f.url(), code, "a"); err != nil {
		t.Fatalf("the code was spent by someone who knew only its id: %v", err)
	}
}

func TestRevokingEndsTheOpenSessionAtOnce(t *testing.T) {
	f := newFixture(t)
	d, key := f.pair(ScopeAdmin)
	c, stop := connected(t, d, key)
	defer stop()
	if err := f.srv.Revoke(context.Background(), d.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for c.Status().State == StateConnected && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if c.Status().State == StateConnected {
		t.Fatal("the session stayed open after the device was revoked")
	}
}

// A new key the server lost (it restarted before the device proved it) is
// dropped, and the current one still works; nothing is kept until a key is
// proved.
func TestARotationTheServerLostFallsBack(t *testing.T) {
	f := newFixture(t)
	d, key := f.pair(ScopeAdmin)
	c, stop := connected(t, d, key)
	defer stop()
	kept := 0
	c.OnKey = func([]byte) { kept++ }
	stranger, _ := suite.GenerateKeypair(rand.Reader)
	c.mu.Lock()
	c.next = &stranger
	m := c.mux
	c.mu.Unlock()
	m.ch.close("reconnect")
	time.Sleep(200 * time.Millisecond)
	waitState(t, c, StateConnected)
	if code, _, err := get(t, c, "/api/status"); err != nil || code != 200 {
		t.Fatalf("after the fallback: %d %v", code, err)
	}
	if kept != 0 {
		t.Fatal("a key the server never took was kept")
	}
}

// A request whose handler does not read its body does not hold up the
// others in the same session.
func TestABodyNobodyReadsDoesNotBlockTheSession(t *testing.T) {
	f := newFixture(t)
	release := make(chan struct{})
	f.srv.Admin = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			<-release // never reads the body
		}
		_, _ = io.WriteString(w, "ok")
	})
	defer close(release)
	d, key := f.pair(ScopeAdmin)
	c, stop := connected(t, d, key)
	defer stop()
	go func() {
		req, _ := http.NewRequest(http.MethodPost, "http://local/slow", strings.NewReader(strings.Repeat("x", 8<<20)))
		if resp, err := c.RoundTrip(req); err == nil {
			resp.Body.Close()
		}
	}()
	time.Sleep(300 * time.Millisecond)
	done := make(chan error, 1)
	go func() {
		req, _ := http.NewRequest(http.MethodGet, "http://local/fast", nil)
		resp, err := c.RoundTrip(req)
		if err == nil {
			resp.Body.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a body nobody reads blocked the whole session")
	}
}

func TestADeviceNameIsCut(t *testing.T) {
	f := newFixture(t)
	if _, _, err := Pair(context.Background(), f.url(), f.code(ScopeAdmin), strings.Repeat("Mac ", 100)); err != nil {
		t.Fatal(err)
	}
	devices, _ := f.st.RemoteClients(context.Background())
	if len([]rune(devices[0].Name)) > maxNameLen {
		t.Fatalf("a %d-character name was kept", len([]rune(devices[0].Name)))
	}
}

// A request the app gives up on (a timeout, a page navigated away) ends on
// its own: the session, and the other requests in it, go on.
func TestACancelledRequestLeavesTheSessionUp(t *testing.T) {
	f := newFixture(t)
	release := make(chan struct{})
	f.srv.Admin = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			<-release
			_, _ = w.Write([]byte(strings.Repeat("late answer ", 10000)))
			return
		}
		_, _ = io.WriteString(w, "ok")
	})
	d, key := f.pair(ScopeAdmin)
	c, stop := connected(t, d, key)
	defer stop()
	since := c.Status().Since
	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://local/slow", strings.NewReader(strings.Repeat("y", 200<<10)))
		if _, err := c.RoundTrip(req); err == nil {
			t.Fatal("the slow request should have timed out")
		}
		cancel()
	}
	close(release) // the handlers answer now, for requests nobody waits for
	time.Sleep(300 * time.Millisecond)
	if code, _, err := get(t, c, "/fast"); err != nil || code != 200 {
		t.Fatalf("after cancelled requests: %d %v", code, err)
	}
	if st := c.Status(); st.State != StateConnected || st.Failures != 0 || !st.Since.Equal(since) {
		t.Fatalf("the session dropped: %+v", st)
	}
}

// Connections that never send anything do not hold the handshake slots for
// long.
func TestIdleConnectionsLetGoOfTheHandshakeSlots(t *testing.T) {
	f := newFixture(t)
	u, _ := url.Parse(f.url())
	var idle []*websocket.Conn
	for i := 0; i < maxPending; i++ {
		ws, err := dial(context.Background(), u, "/remote/v1/session")
		if err != nil {
			t.Fatal(err)
		}
		idle = append(idle, ws)
	}
	defer func() {
		for _, ws := range idle {
			ws.CloseNow()
		}
	}()
	time.Sleep(firstFrame + time.Second)
	if _, _, err := Pair(context.Background(), f.url(), f.code(ScopeAdmin), "a"); err != nil {
		t.Fatalf("idle connections still hold the slots: %v", err)
	}
}

// Closing a response before its end stops the server's handler: an
// interrupted download does not keep the tunnel busy.
func TestClosingABodyStopsTheServer(t *testing.T) {
	f := newFixture(t)
	stopped := make(chan struct{})
	f.srv.Admin = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(stopped)
		// A slow stream: the client's queue would take long to fill, so only
		// the cancel stops it soon.
		chunk := []byte(strings.Repeat("x", 2048))
		for r.Context().Err() == nil {
			if _, err := w.Write(chunk); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			time.Sleep(100 * time.Millisecond)
		}
	})
	d, key := f.pair(ScopeAdmin)
	c, stop := connected(t, d, key)
	defer stop()
	req, _ := http.NewRequest(http.MethodGet, "http://local/media/big", nil)
	resp, err := c.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(resp.Body, make([]byte, 1024)); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("the server kept sending after the body was closed")
	}
	if code, _, err := get(t, c, "/api/status"); err == nil && code == 0 {
		t.Fatal("the session should still answer")
	}
}

// Once the new key is taken, a session still open with the old one (a copy
// of the key elsewhere) ends: the old key keeps nothing alive.
func TestRotatingEndsTheOldKeysSessions(t *testing.T) {
	f := newFixture(t)
	d, key := f.pair(ScopeAdmin)
	c, stop := connected(t, d, key)
	defer stop()
	copied, stopCopy := connected(t, d, append([]byte{}, key...))
	defer stopCopy()
	since := copied.Status().Since
	if _, err := c.Rotate(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitState(t, c, StateConnected)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st := copied.Status(); st.State != StateConnected || !st.Since.Equal(since) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if st := copied.Status(); st.State == StateConnected && st.Since.Equal(since) {
		t.Fatal("the session opened with the old key is still up")
	}
	waitState(t, copied, StateRefused) // and it cannot come back
	if code, _, err := get(t, c, "/api/status"); err != nil || code != 200 {
		t.Fatalf("the device with the new key: %d %v", code, err)
	}
}

// A new key that cannot be kept is never sent; one that was kept brings
// the device back after a restart even if replacing the current key failed.
func TestARotatedKeyIsNeverOnlyInMemory(t *testing.T) {
	f := newFixture(t)
	d, key := f.pair(ScopeAdmin)
	c, stop := connected(t, d, key)
	defer stop()
	c.OnNextKey = func([]byte) error { return errors.New("keychain locked") }
	if _, err := c.Rotate(context.Background()); err == nil {
		t.Fatal("a rotation whose key could not be kept went ahead")
	}
	stored, _ := f.st.RemoteClient(context.Background(), d.ID)
	if pub, _ := publicKey(key); string(stored.PublicKey) != string(pub) {
		t.Fatal("the server took a key the app could not keep")
	}

	var kept []byte
	c.OnNextKey = func(k []byte) error { kept = append([]byte{}, k...); return nil }
	c.OnKey = func([]byte) {} // replacing the current key "fails": nothing saved
	if _, err := c.Rotate(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitState(t, c, StateConnected)
	stop()
	// The app starts again with the old key and the kept one.
	again, err := NewClient(d, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := again.SetNext(kept); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go again.Run(ctx)
	waitState(t, again, StateConnected)
}
