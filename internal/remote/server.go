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

	mu      sync.Mutex
	codes   map[string]*pairCode
	rotates map[string]rotation // a device's next key, by the key
}

type pairCode struct {
	secret  []byte
	name    string
	scope   string
	expires time.Time
}

type rotation struct {
	id      string
	expires time.Time
}

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
	return &Server{state: st, key: key, logger: logger, Now: time.Now, codes: map[string]*pairCode{}, rotates: map[string]rotation{}}, nil
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

// take removes and returns a code, which works once.
func (s *Server) take(id string) (*pairCode, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.codes[id]
	delete(s.codes, id)
	if !ok || s.Now().After(c.expires) {
		return nil, false
	}
	return c, true
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

// guard refuses what did not come over HTTPS (directly, or through a proxy
// that says so in X-Forwarded-Proto) and anything from a web page, which
// carries an Origin. The client's own rule, https:// only, is what keeps
// the connection on HTTPS; this check catches a proxy set up without it.
func (s *Server) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil && !overTLS(r.Header.Get("X-Forwarded-Proto")) {
			http.Error(w, "HTTPS only", http.StatusUpgradeRequired)
			return
		}
		if r.Header.Get("Origin") != "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

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
	id := r.URL.Query().Get("id")
	ws, err := accept(w, r)
	if err != nil {
		return
	}
	defer ws.CloseNow()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	code, ok := s.take(id)
	if !ok {
		_ = ws.Close(websocket.StatusPolicyViolation, "unknown or expired pairing code")
		return
	}
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
	var hello struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(payload, &hello)
	name := strings.TrimSpace(hello.Name)
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
	kind, first, err := ws.Read(hctx)
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
	ch := &channel{ws: ws, send: cs2, recv: cs1}
	s.logger.Info("remote session opened", "device", device.ID, "name", device.Name)
	s.serve(r.Context(), ch, device)
	s.logger.Info("remote session closed", "device", device.ID)
}

// identify finds the paired device a key belongs to, or the device whose
// next key it is, and refuses a revoked device or a replayed handshake.
func (s *Server) identify(ctx context.Context, key []byte, stamp int64) (state.RemoteClient, error) {
	device, err := s.state.RemoteClientByKey(ctx, key)
	if errors.Is(err, state.ErrNoRemoteClient) {
		s.mu.Lock()
		rot, ok := s.rotates[hex.EncodeToString(key)]
		if ok {
			delete(s.rotates, hex.EncodeToString(key))
		}
		s.mu.Unlock()
		if !ok || s.Now().After(rot.expires) {
			return device, errors.New("unknown device key")
		}
		if err := s.state.RotateRemoteClient(ctx, rot.id, key); err != nil {
			return device, err
		}
		device, err = s.state.RemoteClient(ctx, rot.id)
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
	body   *io.PipeWriter
	cancel context.CancelFunc
}

// serve reads a device's requests and answers each in its own goroutine.
func (s *Server) serve(ctx context.Context, ch *channel, device state.RemoteClient) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	streams := map[uint32]*stream{}
	var mu sync.Mutex
	defer func() {
		mu.Lock()
		for _, st := range streams {
			_ = st.body.CloseWithError(errClosed)
			st.cancel()
		}
		mu.Unlock()
	}()
	for {
		// A revoked device loses its session at its next request.
		msg, err := ch.read(ctx)
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
			r, w := io.Pipe()
			sctx, scancel := context.WithCancel(ctx)
			mu.Lock()
			streams[id] = &stream{body: w, cancel: scancel}
			mu.Unlock()
			go func() {
				defer scancel()
				s.answer(sctx, ch, device, id, head, r)
				mu.Lock()
				delete(streams, id)
				mu.Unlock()
			}()
		case frameReqBody:
			mu.Lock()
			st := streams[id]
			mu.Unlock()
			if st != nil {
				_, _ = st.body.Write(payload)
			}
		case frameReqEnd:
			mu.Lock()
			st := streams[id]
			mu.Unlock()
			if st != nil {
				_ = st.body.Close()
			}
		case frameCancel:
			mu.Lock()
			st := streams[id]
			mu.Unlock()
			if st != nil {
				st.cancel()
			}
		}
	}
}

// answer runs one request through the handler the device's scope reaches.
func (s *Server) answer(ctx context.Context, ch *channel, device state.RemoteClient, id uint32, head reqHead, body io.ReadCloser) {
	w := &responseWriter{ctx: ctx, ch: ch, stream: id, header: http.Header{}}
	defer w.finish()
	// A body the handler did not read must not hold the session's reader.
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
		s.mu.Lock()
		s.rotates[hex.EncodeToString(req.Key)] = rotation{id: device.ID, expires: s.Now().Add(pairTTL)}
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
