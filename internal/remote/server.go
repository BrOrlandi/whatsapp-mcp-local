package remote

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/flynn/noise"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

// Scopes of a paired device.
const (
	// ScopeAdmin is the app: the panel, the API and the MCP.
	ScopeAdmin = "admin"
	// ScopeTools reaches the MCP only.
	ScopeTools = "tools"
)

// pairTTL is how long a pairing code works; it works once.
const pairTTL = 10 * time.Minute

// Server is the daemon's side: it pairs devices and serves their sessions.
type Server struct {
	state  *state.State
	key    noise.DHKey
	logger *slog.Logger
	// Admin answers a paired app's requests: the panel, the API and the
	// MCP, as the daemon serves them on its loopback port.
	Admin http.Handler
	// Tools answers a tools-only device's requests; only /mcp reaches it.
	Tools http.Handler
	// Now is the clock, replaceable in tests.
	Now func() time.Time
	// TrustedProxy says whether a request comes from the HTTPS proxy, whose
	// X-Forwarded-Proto is believed; nil believes any (a listener only the
	// proxy reaches). Behind a Docker network shared with other containers,
	// it names the proxy, so a neighbour cannot pass for it.
	TrustedProxy func(remoteAddr string) bool

	mu       sync.Mutex
	codes    map[string]*pairCode
	rotates  map[string]rotation // a device's next key, by the device
	sessions map[string]map[*sessionRef]struct{}
	pending  int // handshakes under way
}

type pairCode struct {
	secret  []byte
	name    string
	scope   string
	expires time.Time
	// inUse is set while a pairing with the code runs: it cannot run twice,
	// and the code is spent only when the device proves it has the secret.
	inUse bool
}

type rotation struct {
	key     []byte
	expires time.Time
}

type sessionRef struct{ cancel context.CancelFunc }

// Limits on what a device, or anything that reaches the routes, can hold.
const (
	maxPending  = 16               // handshakes at once
	maxStreams  = 64               // requests at once in a session
	idleTimeout = 90 * time.Second // a session with nothing, not even a ping
	streamQueue = 64               // body frames queued for a request not read yet
	maxNameLen  = 64
	// firstFrame is how long a connection may wait before its first
	// message, so idle connections cannot hold the handshake slots.
	firstFrame = 5 * time.Second
)

// KeyFile is where the server keeps its static key pair, readable only by
// its owner, in the data folder.
const KeyFile = "remote-server.key"

// NewServer loads the server's static key from the data folder, creating it
// on the first run.
func NewServer(dataDir string, st *state.State, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}
	key, err := loadOrCreateKey(filepath.Join(dataDir, KeyFile))
	if err != nil {
		return nil, err
	}
	return &Server{state: st, key: key, logger: logger, Now: time.Now, codes: map[string]*pairCode{}, rotates: map[string]rotation{},
		sessions: map[string]map[*sessionRef]struct{}{}}, nil
}

func loadOrCreateKey(path string) (noise.DHKey, error) {
	if raw, err := os.ReadFile(path); err == nil {
		var k struct{ Private, Public []byte }
		if err := json.Unmarshal(raw, &k); err != nil || len(k.Private) != 32 || len(k.Public) != 32 {
			return noise.DHKey{}, fmt.Errorf("remote: %s is not a key", path)
		}
		return noise.DHKey{Private: k.Private, Public: k.Public}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return noise.DHKey{}, err
	}
	key, err := suite.GenerateKeypair(rand.Reader)
	if err != nil {
		return noise.DHKey{}, err
	}
	raw, _ := json.Marshal(struct{ Private, Public []byte }{key.Private, key.Public})
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return noise.DHKey{}, err
	}
	tmp := path + ".new"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return noise.DHKey{}, err
	}
	return key, os.Rename(tmp, path)
}

// PublicKey is the server's static public key.
func (s *Server) PublicKey() []byte { return append([]byte(nil), s.key.Public...) }

// Fingerprint is the start of the server's public key, as the pairing code
// carries it and the app shows it.
func Fingerprint(pub []byte) string { return hex.EncodeToString(pub[:min(len(pub), 8)]) }

// Code is a pairing code as the server hands it out: shown once, never
// stored, good for one device within pairTTL.
type Code struct {
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Scope       string    `json:"scope"`
	ExpiresAt   time.Time `json:"expires_at"`
	Fingerprint string    `json:"server_fingerprint"`
}

// NewCode makes a pairing code for one device. Its secret stays in this
// process's memory only, and goes with the first pairing or the deadline.
func (s *Server) NewCode(name, scope string) (Code, error) {
	if scope != ScopeAdmin && scope != ScopeTools {
		return Code{}, fmt.Errorf("scope must be %s or %s", ScopeAdmin, ScopeTools)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "app"
	}
	id := make([]byte, 5)
	secret := make([]byte, 32)
	if _, err := rand.Read(id); err != nil {
		return Code{}, err
	}
	if _, err := rand.Read(secret); err != nil {
		return Code{}, err
	}
	now := s.Now()
	s.mu.Lock()
	for k, c := range s.codes {
		if now.After(c.expires) {
			delete(s.codes, k)
		}
	}
	s.codes[hex.EncodeToString(id)] = &pairCode{secret: secret, name: name, scope: scope, expires: now.Add(pairTTL)}
	s.mu.Unlock()
	return Code{Code: encodeCode(id, secret, s.key.Public), Name: name, Scope: scope, ExpiresAt: now.Add(pairTTL).UTC(),
		Fingerprint: Fingerprint(s.key.Public)}, nil
}

// take reserves a code for one pairing at a time. done(true) spends it,
// once the device has proved it knows the secret; done(false) gives it
// back, so whoever knows only the code's id cannot spend it.
func (s *Server) take(id string) (*pairCode, func(spent bool), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.codes[id]
	if !ok || c.inUse || s.Now().After(c.expires) {
		return nil, nil, false
	}
	c.inUse = true
	return c, func(spent bool) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if spent {
			delete(s.codes, id)
		} else {
			c.inUse = false
		}
	}, true
}

// Revoke cuts a device off: it is recorded as revoked, and its open
// sessions end now.
func (s *Server) Revoke(ctx context.Context, id string) error {
	if err := s.state.RevokeRemoteClient(ctx, id); err != nil {
		return err
	}
	s.mu.Lock()
	for ref := range s.sessions[id] {
		ref.cancel()
	}
	delete(s.rotates, id)
	s.mu.Unlock()
	s.logger.Info("a remote device was revoked", "device", id)
	return nil
}

func (s *Server) track(id string, cancel context.CancelFunc) func() {
	ref := &sessionRef{cancel: cancel}
	s.mu.Lock()
	if s.sessions[id] == nil {
		s.sessions[id] = map[*sessionRef]struct{}{}
	}
	s.sessions[id][ref] = struct{}{}
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.sessions[id], ref)
		if len(s.sessions[id]) == 0 {
			delete(s.sessions, id)
		}
		s.mu.Unlock()
	}
}

// admit counts a handshake in, refusing past maxPending.
func (s *Server) admit() (func(), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending >= maxPending {
		return nil, false
	}
	s.pending++
	return func() {
		s.mu.Lock()
		s.pending--
		s.mu.Unlock()
	}, true
}

// Handler serves the two remote routes. It sits behind an HTTPS proxy on a
// private network and is never published as plain HTTP.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /remote/v1/pair", s.guard(s.servePair))
	mux.HandleFunc("GET /remote/v1/session", s.guard(s.serveSession))
	mux.HandleFunc("GET /remote/v1/health", s.guard(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok\n")
	}))
	return mux
}

// guard refuses what did not reach the server over HTTPS (directly, or
// through the trusted proxy, which says so in X-Forwarded-Proto), what came
// from anything but that proxy, and anything from a web page, which carries
// an Origin. The client's own rule, https:// only, is what keeps the
// connection on HTTPS; this keeps the routes to the proxy.
func (s *Server) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil && s.TrustedProxy != nil && !s.TrustedProxy(r.RemoteAddr) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.TLS == nil && !overTLS(r.Header.Get("X-Forwarded-Proto")) {
			http.Error(w, HTTPSOnly, http.StatusForbidden)
			return
		}
		if r.Header.Get("Origin") != "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// HTTPSOnly is the answer to a request that did not come over HTTPS.
const HTTPSOnly = "remote access is HTTPS only"

// overTLS reads X-Forwarded-Proto: https, or wss, which Traefik sends for a
// WebSocket that arrived over TLS. http and ws did not.
func overTLS(proto string) bool {
	proto = strings.ToLower(strings.TrimSpace(proto))
	return proto == "https" || proto == "wss"
}

func accept(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(maxMessage + 1024)
	return ws, nil
}

// servePair runs Noise_XXpsk3 with a code's secret: both ends prove they
// know it while they swap static keys, and the device is recorded.
func (s *Server) servePair(w http.ResponseWriter, r *http.Request) {
	done, ok := s.admit()
	if !ok {
		http.Error(w, "busy", http.StatusServiceUnavailable)
		return
	}
	defer done()
	ws, err := accept(w, r)
	if err != nil {
		return
	}
	defer ws.CloseNow()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	// The code's id comes in the first message, not in the address, which
	// a proxy may log.
	fctx, fcancel := context.WithTimeout(ctx, firstFrame)
	kind, idMsg, err := ws.Read(fctx)
	fcancel()
	if err != nil || kind != websocket.MessageBinary || len(idMsg) > 32 {
		return
	}
	id := string(idMsg)
	code, release, ok := s.take(id)
	if !ok {
		_ = ws.Close(websocket.StatusPolicyViolation, "unknown or expired pairing code")
		return
	}
	spent := false
	defer func() { release(spent) }()
	hs, err := noise.NewHandshakeState(noise.Config{CipherSuite: suite, Pattern: noise.HandshakeXX, Initiator: false,
		Prologue: []byte(pairPrologue + ":" + id), StaticKeypair: s.key, PresharedKey: code.secret, PresharedKeyPlacement: 3})
	if err != nil {
		_ = ws.Close(websocket.StatusInternalError, "")
		return
	}
	ch, payload, err := handshake(ctx, ws, hs, false, nil)
	if err != nil {
		s.logger.Warn("a remote pairing failed", "error", err)
		_ = ws.Close(websocket.StatusPolicyViolation, "pairing failed")
		return
	}
	// The device has proved it knows the secret: the code is spent, whatever
	// happens next.
	spent = true
	var hello struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(payload, &hello)
	name := strings.Join(strings.Fields(hello.Name), " ")
	if runes := []rune(name); len(runes) > maxNameLen {
		name = string(runes[:maxNameLen])
	}
	if name == "" {
		name = code.name
	}
	// The device checks this server's key against the code's fingerprint
	// and confirms; only then is it recorded.
	confirm, err := ch.read(ctx)
	if err != nil || string(confirm) != "confirm" {
		ch.close("not confirmed")
		return
	}
	device := state.RemoteClient{ID: randomID(), Name: name, PublicKey: hs.PeerStatic(), Scope: code.scope, CreatedAt: s.Now().UTC()}
	if err := s.state.AddRemoteClient(ctx, device); err != nil {
		ch.close("could not record the device")
		return
	}
	s.logger.Info("a device was paired for remote access", "device", device.ID, "name", device.Name, "scope", device.Scope)
	answer, _ := json.Marshal(map[string]string{"id": device.ID, "scope": device.Scope})
	_ = ch.write(ctx, answer)
	ch.close("paired")
}

// serveSession runs Noise_IK: the device proves it holds the key it paired
// with (or the next one it asked to rotate to), and its handshake must be
// newer than its last one. Then it serves the device's requests.
func (s *Server) serveSession(w http.ResponseWriter, r *http.Request) {
	admitted, ok := s.admit()
	if !ok {
		http.Error(w, "busy", http.StatusServiceUnavailable)
		return
	}
	var once sync.Once
	done := func() { once.Do(admitted) }
	defer done()
	ws, err := accept(w, r)
	if err != nil {
		return
	}
	defer ws.CloseNow()
	hctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	hs, err := noise.NewHandshakeState(noise.Config{CipherSuite: suite, Pattern: noise.HandshakeIK, Initiator: false,
		Prologue: []byte(sessionPrologue), StaticKeypair: s.key})
	if err != nil {
		cancel()
		_ = ws.Close(websocket.StatusInternalError, "")
		return
	}
	// IK: the device's first message carries its static key and a
	// timestamp; check both before answering.
	fctx, fcancel := context.WithTimeout(hctx, firstFrame)
	kind, first, err := ws.Read(fctx)
	fcancel()
	if err != nil || kind != websocket.MessageBinary {
		cancel()
		return
	}
	stamp, _, _, err := hs.ReadMessage(nil, first)
	if err != nil || len(stamp) != 8 {
		cancel()
		_ = ws.Close(websocket.StatusPolicyViolation, "handshake failed")
		return
	}
	device, err := s.identify(hctx, hs.PeerStatic(), int64(binary.BigEndian.Uint64(stamp)))
	if err != nil {
		cancel()
		s.logger.Warn("a remote session was refused", "error", err)
		_ = ws.Close(websocket.StatusPolicyViolation, "unauthorized")
		return
	}
	reply, cs1, cs2, err := hs.WriteMessage(nil, nil)
	if err == nil {
		err = ws.Write(hctx, websocket.MessageBinary, reply)
	}
	cancel()
	if err != nil || cs1 == nil {
		return
	}
	ch := &channel{ws: ws, send: cs2, recv: cs1, rekey: rekeyEvery.Load()}
	done()
	ctx, stop := context.WithCancel(r.Context())
	defer stop()
	untrack := s.track(device.ID, stop)
	defer untrack()
	s.logger.Info("remote session opened", "device", device.ID, "name", device.Name)
	s.serve(ctx, ch, device)
	s.logger.Info("remote session closed", "device", device.ID)
}

// identify finds the paired device a key belongs to, or the device whose
// next key it is, and refuses a revoked device or a replayed handshake.
func (s *Server) identify(ctx context.Context, key []byte, stamp int64) (state.RemoteClient, error) {
	device, err := s.state.RemoteClientByKey(ctx, key)
	if errors.Is(err, state.ErrNoRemoteClient) {
		s.mu.Lock()
		id := ""
		for dev, rot := range s.rotates {
			if s.Now().After(rot.expires) {
				delete(s.rotates, dev)
			} else if subtle.ConstantTimeCompare(rot.key, key) == 1 {
				id = dev
				delete(s.rotates, dev)
			}
		}
		s.mu.Unlock()
		if id == "" {
			return device, errors.New("unknown device key")
		}
		if err := s.state.RotateRemoteClient(ctx, id, key); err != nil {
			return device, err
		}
		device, err = s.state.RemoteClient(ctx, id)
	}
	if err != nil {
		return device, err
	}
	if device.RevokedAt != nil {
		return device, fmt.Errorf("device %s was revoked", device.ID)
	}
	fresh, err := s.state.TouchRemoteClient(ctx, device.ID, stamp, s.Now())
	if err != nil {
		return device, err
	}
	if !fresh {
		return device, fmt.Errorf("device %s sent an old handshake (a replay?)", device.ID)
	}
	return device, nil
}

type stream struct {
	body   chan []byte // nil closes the body
	cancel context.CancelFunc
}

// serve reads a device's requests and answers each in its own goroutine.
// The reader never waits on a handler: a request's body is queued, and a
// request that falls too far behind is cancelled.
func (s *Server) serve(ctx context.Context, ch *channel, device state.RemoteClient) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	streams := map[uint32]*stream{}
	var mu sync.Mutex
	defer func() {
		mu.Lock()
		for _, st := range streams {
			st.cancel()
		}
		mu.Unlock()
	}()
	drop := func(id uint32) {
		mu.Lock()
		if st := streams[id]; st != nil {
			st.cancel()
			delete(streams, id)
		}
		mu.Unlock()
	}
	for {
		// The device pings every 25 seconds: a session silent for longer
		// than idleTimeout is gone.
		rctx, rcancel := context.WithTimeout(ctx, idleTimeout)
		msg, err := ch.read(rctx)
		rcancel()
		if err != nil {
			return
		}
		kind, id, payload, err := parseFrame(msg)
		if err != nil {
			ch.close("bad frame")
			return
		}
		switch kind {
		case framePing:
			_ = ch.write(ctx, frame(framePong, 0, nil))
		case frameRotate:
			s.rotate(ctx, ch, device, payload)
		case frameReqHead:
			var head reqHead
			if json.Unmarshal(payload, &head) != nil || !strings.HasPrefix(head.Path, "/") {
				ch.close("bad request")
				return
			}
			current, err := s.state.RemoteClient(ctx, device.ID)
			if err != nil || current.RevokedAt != nil {
				ch.close("revoked")
				return
			}
			mu.Lock()
			if len(streams) >= maxStreams || streams[id] != nil {
				mu.Unlock()
				ch.close("too many requests at once")
				return
			}
			sctx, scancel := context.WithCancel(ctx)
			st := &stream{body: make(chan []byte, streamQueue), cancel: scancel}
			streams[id] = st
			mu.Unlock()
			go func() {
				defer scancel()
				s.answer(ctx, sctx, ch, device, id, head, &bodyReader{ctx: sctx, frames: st.body})
				mu.Lock()
				if streams[id] == st {
					delete(streams, id)
				}
				mu.Unlock()
			}()
		case frameReqBody, frameReqEnd:
			mu.Lock()
			st := streams[id]
			mu.Unlock()
			if st == nil {
				continue
			}
			part := []byte(nil)
			if kind == frameReqBody {
				part = append([]byte{}, payload...)
				if len(part) == 0 {
					continue
				}
			}
			select {
			case st.body <- part:
			default:
				drop(id) // the handler is not reading: give up on it, not on the session
			}
		case frameCancel:
			drop(id)
		}
	}
}

// bodyReader is a request's body, from the frames serve queued for it; a nil
// frame ends it.
type bodyReader struct {
	ctx    context.Context
	frames chan []byte
	cur    []byte
	done   bool
}

func (b *bodyReader) Read(p []byte) (int, error) {
	for len(b.cur) == 0 {
		if b.done {
			return 0, io.EOF
		}
		select {
		case part := <-b.frames:
			if part == nil {
				b.done = true
				continue
			}
			b.cur = part
		case <-b.ctx.Done():
			return 0, b.ctx.Err()
		}
	}
	n := copy(p, b.cur)
	b.cur = b.cur[n:]
	return n, nil
}

func (b *bodyReader) Close() error { return nil }

// answer runs one request through the handler the device's scope reaches.
func (s *Server) answer(session, ctx context.Context, ch *channel, device state.RemoteClient, id uint32, head reqHead, body io.ReadCloser) {
	w := &responseWriter{ctx: session, stop: ctx, ch: ch, stream: id, header: http.Header{}}
	defer w.finish()
	defer body.Close()
	handler := s.Admin
	if device.Scope != ScopeAdmin {
		handler = s.Tools
		if head.Path != "/mcp" && !strings.HasPrefix(head.Path, "/mcp?") {
			http.Error(w, "this device reaches the MCP only", http.StatusForbidden)
			return
		}
	}
	if handler == nil {
		http.Error(w, "not available", http.StatusServiceUnavailable)
		return
	}
	req, err := http.NewRequestWithContext(WithDevice(ctx, device), head.Method, "http://remote"+head.Path, body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.Header = cleanHeader(head.Header)
	req.RemoteAddr = "remote:" + device.ID
	handler.ServeHTTP(w, req)
}

// rotate takes a device's next key: the device then opens a session with
// it, which proves it holds it, and only then the old key stops working.
func (s *Server) rotate(ctx context.Context, ch *channel, device state.RemoteClient, payload []byte) {
	var req struct {
		Key []byte `json:"key"`
	}
	answer := map[string]string{}
	if json.Unmarshal(payload, &req) != nil || len(req.Key) != 32 || subtle.ConstantTimeCompare(req.Key, device.PublicKey) == 1 {
		answer["error"] = "a new 32-byte key is needed"
	} else {
		// One pending key per device: a newer request replaces it. Until the
		// device opens a session with it, its current key still works.
		s.mu.Lock()
		s.rotates[device.ID] = rotation{key: append([]byte{}, req.Key...), expires: s.Now().Add(pairTTL)}
		s.mu.Unlock()
	}
	raw, _ := json.Marshal(answer)
	_ = ch.write(ctx, frame(frameRotated, 0, raw))
}

type deviceKey struct{}

// WithDevice marks a request as coming from a paired device.
func WithDevice(ctx context.Context, d state.RemoteClient) context.Context {
	return context.WithValue(ctx, deviceKey{}, d)
}

// DeviceFrom is the paired device a request came from, if any.
func DeviceFrom(ctx context.Context) (state.RemoteClient, bool) {
	d, ok := ctx.Value(deviceKey{}).(state.RemoteClient)
	return d, ok
}

func randomID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
