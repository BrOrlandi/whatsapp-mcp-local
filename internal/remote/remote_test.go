package remote

import (
	"context"
	"crypto/rand"
	"encoding/json"
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
	if out["auth"] != "" {
		t.Fatal("a credential from the local side must not cross to the server")
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
		{nil, http.StatusUpgradeRequired},
		{map[string]string{"X-Forwarded-Proto": "http"}, http.StatusUpgradeRequired},
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
	old := rekeyEvery
	rekeyEvery = 3
	defer func() { rekeyEvery = old }()
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
