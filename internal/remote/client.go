package remote

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/flynn/noise"
)

// ---- pairing codes ----

const codePrefix = "wamcp1-"

var codeEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// encodeCode is what the person copies from the server to the app: the
// code's id, its 32-byte secret, and the start of the server's key.
func encodeCode(id, secret, serverPub []byte) string {
	raw := append(append(append([]byte{}, id...), secret...), serverPub[:8]...)
	return codePrefix + strings.ToLower(codeEncoding.EncodeToString(raw))
}

func decodeCode(code string) (id string, secret, fingerprint []byte, err error) {
	code = strings.Join(strings.Fields(code), "")
	body, ok := strings.CutPrefix(strings.ToLower(code), codePrefix)
	if !ok {
		return "", nil, nil, errors.New("this is not a pairing code: it starts with " + codePrefix)
	}
	raw, err := codeEncoding.DecodeString(strings.ToUpper(body))
	if err != nil || len(raw) != 5+32+8 {
		return "", nil, nil, errors.New("the pairing code is incomplete: copy it again, whole")
	}
	return hex.EncodeToString(raw[:5]), raw[5:37], raw[37:], nil
}

// ---- where the server is ----

// ServerURL checks the server's address: https:// only, and a host. The app
// never speaks to a server in plain HTTP, not even to pair.
func ServerURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return nil, errors.New("the server's address must be a full https:// address, such as https://whatsapp.example.com")
	}
	if u.Scheme != "https" {
		return nil, errors.New("the server's address must start with https://: the remote connection is HTTPS only")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("the server's address must be just https:// and the host (and a path, if any)")
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	return u, nil
}

// httpClient checks the server's certificate the normal way (chain,
// hostname, validity) and follows no redirect: a redirect could only take
// the connection somewhere it did not choose, plain HTTP included.
func httpClient() *http.Client {
	return &http.Client{
		Transport: Transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("the server answered with a redirect, which the remote connection does not follow")
		},
	}
}

func dial(ctx context.Context, server *url.URL, path string) (*websocket.Conn, error) {
	u := *server
	u.Scheme = "wss"
	path, query, _ := strings.Cut(path, "?")
	u.Path += path
	u.RawQuery = query
	ws, resp, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{HTTPClient: httpClient()})
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUpgradeRequired {
			return nil, errors.New("the server is not behind HTTPS")
		}
		return nil, err
	}
	ws.SetReadLimit(maxMessage + 1024)
	return ws, nil
}

// Transport is the HTTP transport, the system's by default (which checks
// certificates); tests set one that trusts their own certificate.
var Transport http.RoundTripper

// ---- pairing ----

// Device is what the app keeps to reach a server: everything but its
// private key, which goes to the keychain.
type Device struct {
	Server    string `json:"server"`
	ServerKey []byte `json:"server_key"`
	ID        string `json:"device_id"`
	Scope     string `json:"scope"`
}

// Pair registers this device with the server named in serverURL, using a
// pairing code from it. It returns what to keep and the device's new
// private key, for the keychain.
func Pair(ctx context.Context, serverURL, code, name string) (Device, []byte, error) {
	server, err := ServerURL(serverURL)
	if err != nil {
		return Device{}, nil, err
	}
	id, secret, fingerprint, err := decodeCode(code)
	if err != nil {
		return Device{}, nil, err
	}
	key, err := suite.GenerateKeypair(rand.Reader)
	if err != nil {
		return Device{}, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ws, err := dial(ctx, server, "/remote/v1/pair?id="+url.QueryEscape(id))
	if err != nil {
		return Device{}, nil, fmt.Errorf("could not reach the server: %w", err)
	}
	defer ws.CloseNow()
	hs, err := noise.NewHandshakeState(noise.Config{CipherSuite: suite, Pattern: noise.HandshakeXX, Initiator: true,
		Prologue: []byte(pairPrologue + ":" + id), StaticKeypair: key, PresharedKey: secret, PresharedKeyPlacement: 3})
	if err != nil {
		return Device{}, nil, err
	}
	hello, _ := json.Marshal(map[string]string{"name": name})
	ch, _, err := handshake(ctx, ws, hs, true, [][]byte{nil, hello})
	if err != nil {
		return Device{}, nil, errors.New("the server refused the pairing code: it may have expired or been used; make a new one")
	}
	serverKey := hs.PeerStatic()
	if subtle.ConstantTimeCompare(serverKey[:8], fingerprint) != 1 {
		return Device{}, nil, errors.New("the server's key does not match the pairing code: this is not the server that made the code")
	}
	if err := ch.write(ctx, []byte("confirm")); err != nil {
		return Device{}, nil, err
	}
	answer, err := ch.read(ctx)
	if err != nil {
		return Device{}, nil, fmt.Errorf("the server did not confirm the pairing: %w", err)
	}
	var got struct {
		ID    string `json:"id"`
		Scope string `json:"scope"`
	}
	if json.Unmarshal(answer, &got) != nil || got.ID == "" {
		return Device{}, nil, errors.New("the server's answer to the pairing could not be read")
	}
	return Device{Server: server.String(), ServerKey: serverKey, ID: got.ID, Scope: got.Scope}, key.Private, nil
}

// ---- sessions ----

// Connection states, as the app shows them.
const (
	StateConnecting   = "connecting"
	StateConnected    = "connected"
	StateReconnecting = "reconnecting"
	// StateUnavailable is a server that has not answered for a while.
	StateUnavailable = "unavailable"
	// StateRefused is a server that answered and refused this device:
	// revoked, or paired with another key.
	StateRefused = "refused"
)

// Status is the connection between this app and the server; whether the
// server reaches WhatsApp is another matter, which /api/status answers.
type Status struct {
	State     string    `json:"state"`
	Server    string    `json:"server"`
	Since     time.Time `json:"since"`
	LastError string    `json:"last_error,omitempty"`
	Failures  int       `json:"failures,omitempty"`
}

// Client keeps a session with the server open, reconnecting as needed, and
// sends HTTP requests over it.
type Client struct {
	device Device
	server *url.URL
	key    noise.DHKey
	// Now is the clock, replaceable in tests.
	Now func() time.Time

	mu     sync.Mutex
	mux    *clientMux
	status Status
	wake   chan struct{}
	last   int64
	ready  chan struct{}
}

// NewClient prepares a client for a paired device with its private key.
func NewClient(device Device, private []byte) (*Client, error) {
	server, err := ServerURL(device.Server)
	if err != nil {
		return nil, err
	}
	if len(private) != 32 || len(device.ServerKey) != 32 {
		return nil, errors.New("the device's keys are missing: pair it again")
	}
	pub, err := publicKey(private)
	if err != nil {
		return nil, err
	}
	return &Client{device: device, server: server, key: noise.DHKey{Private: private, Public: pub}, Now: time.Now,
		status: Status{State: StateConnecting, Server: server.Host, Since: time.Now()}, wake: make(chan struct{}, 1), ready: make(chan struct{})}, nil
}

// Device is the paired device the client speaks for.
func (c *Client) Device() Device { return c.device }

// Status is how the connection with the server stands now.
func (c *Client) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

func (c *Client) setStatus(f func(*Status)) {
	c.mu.Lock()
	before := c.status.State
	f(&c.status)
	if c.status.State != before {
		c.status.Since = c.Now()
	}
	c.mu.Unlock()
}

// Run keeps the session up until ctx ends: it reconnects with backoff, and
// after a minute of failures shows the server as unavailable.
func (c *Client) Run(ctx context.Context) {
	backoff := time.Second
	var down time.Time
	for ctx.Err() == nil {
		mux, err := c.connect(ctx)
		if err == nil {
			backoff, down = time.Second, time.Time{}
			c.mu.Lock()
			c.mux = mux
			c.mu.Unlock()
			c.setStatus(func(s *Status) { s.State, s.LastError, s.Failures = StateConnected, "", 0 })
			go c.keepAlive(ctx, mux)
			err = mux.run(ctx)
			c.mu.Lock()
			c.mux = nil
			c.mu.Unlock()
		}
		if ctx.Err() != nil {
			return
		}
		if down.IsZero() {
			down = c.Now()
		}
		refused := errors.Is(err, errRefused)
		c.setStatus(func(s *Status) {
			s.Failures++
			s.LastError = err.Error()
			switch {
			case refused:
				s.State = StateRefused
			case c.Now().Sub(down) > time.Minute:
				s.State = StateUnavailable
			default:
				s.State = StateReconnecting
			}
		})
		select {
		case <-ctx.Done():
			return
		case <-c.wake:
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

// Retry asks a waiting client to try again now.
func (c *Client) Retry() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

var errRefused = errors.New("the server refused this device: it may have been revoked; pair it again")

func (c *Client) connect(ctx context.Context) (*clientMux, error) {
	hctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ws, err := dial(hctx, c.server, "/remote/v1/session")
	if err != nil {
		return nil, err
	}
	hs, err := noise.NewHandshakeState(noise.Config{CipherSuite: suite, Pattern: noise.HandshakeIK, Initiator: true,
		Prologue: []byte(sessionPrologue), StaticKeypair: c.key, PeerStatic: c.device.ServerKey})
	if err != nil {
		ws.CloseNow()
		return nil, err
	}
	c.mu.Lock()
	stamp := max(c.Now().UnixNano(), c.last+1)
	c.last = stamp
	c.mu.Unlock()
	ch, _, err := handshake(hctx, ws, hs, true, [][]byte{timestamp(stamp)})
	if err != nil {
		ws.CloseNow()
		if websocket.CloseStatus(err) == websocket.StatusPolicyViolation {
			return nil, errRefused
		}
		return nil, err
	}
	return newClientMux(ch), nil
}

func (c *Client) keepAlive(ctx context.Context, m *clientMux) {
	t := time.NewTicker(25 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if m.ch.write(ctx, frame(framePing, 0, nil)) != nil {
				return
			}
		}
	}
}

// ErrNotConnected is a request made while the session is down.
var ErrNotConnected = errors.New("the app is not connected to the server right now")

// RoundTrip sends a request to the server over the session.
func (c *Client) RoundTrip(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	m := c.mux
	c.mu.Unlock()
	if m == nil {
		return nil, ErrNotConnected
	}
	return m.RoundTrip(req)
}

// Rotate gives the device a new key: the server is told over the current
// session, the client reconnects proving it holds the new one, and only
// then is the old one dropped. It returns the new private key, for the
// keychain.
func (c *Client) Rotate(ctx context.Context) ([]byte, error) {
	c.mu.Lock()
	m := c.mux
	c.mu.Unlock()
	if m == nil {
		return nil, ErrNotConnected
	}
	next, err := suite.GenerateKeypair(rand.Reader)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(map[string][]byte{"key": next.Public})
	if err := m.ch.write(ctx, frame(frameRotate, 0, raw)); err != nil {
		return nil, err
	}
	select {
	case err := <-m.rotated:
		if err != nil {
			return nil, err
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	c.mu.Lock()
	c.key = next
	c.mu.Unlock()
	m.ch.close("rotating the key")
	c.Retry()
	return next.Private, nil
}

func publicKey(private []byte) ([]byte, error) {
	k, err := suite.GenerateKeypair(fixedReader(private))
	if err != nil {
		return nil, err
	}
	return k.Public, nil
}

// fixedReader yields a given private key to GenerateKeypair, which derives
// the public half from it.
type fixedReader []byte

func (f fixedReader) Read(p []byte) (int, error) { return copy(p, f), nil }
