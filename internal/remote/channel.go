// Package remote lets the app on one computer use and manage the daemon on
// another (a server at home, say) over the network, safely.
//
// Two layers protect it, and neither replaces the other:
//
//   - HTTPS, always: the client dials https:// only, checks the server's
//     certificate and hostname the normal way, follows no redirect, and the
//     server refuses a request its proxy did not receive over HTTPS.
//   - The Noise Protocol Framework (github.com/flynn/noise), end to end
//     between the app and the daemon, inside that HTTPS: a proxy that ends
//     TLS (Traefik, Cloudflare) relays ciphertext it cannot read or change.
//
// A device is paired once with a one-time code from the server
// (Noise_XXpsk3: the code's secret authenticates both ends while they swap
// their static keys, and the server's key is also pinned by a fingerprint
// in the code). Every later session is Noise_IK with those keys: each end
// proves it holds its private key, the device's key lives in its keychain,
// and the server keeps only the device's public key. There is no bearer
// token on the wire to copy.
//
// Inside a session, HTTP requests travel as frames (mux.go), so the app can
// serve the remote panel and MCP on its own loopback port as if they were
// local.
package remote

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"
	"github.com/flynn/noise"
)

// suite is the cipher suite of every handshake: X25519, ChaCha20-Poly1305,
// SHA-256.
var suite = noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashSHA256)

// Prologues bind each handshake to its purpose and version: a message of
// one cannot be used in another.
const (
	pairPrologue    = "whatsapp-mcp remote pair v1"
	sessionPrologue = "whatsapp-mcp remote session v1"
)

// maxMessage is the largest Noise message (the protocol's limit), and
// maxPlain what it carries once the 16-byte tag is taken out.
const (
	maxMessage = 65535
	maxPlain   = maxMessage - 16
)

// rekeyEvery is how many messages each direction sends before both ends
// rekey it (Noise's Rekey, the same on both sides), so no key encrypts too
// much.
var rekeyEvery atomic.Uint64

func init() { rekeyEvery.Store(1 << 16) }

// channel is an established Noise session over a WebSocket. Messages arrive
// in order, and each is decrypted with the next nonce: a message replayed,
// dropped, reordered or changed in transit fails to decrypt, and the
// channel closes.
type channel struct {
	ws    *websocket.Conn
	rekey uint64 // rekeyEvery when the session began

	sendMu sync.Mutex
	send   *noise.CipherState
	sent   uint64

	recv     *noise.CipherState
	received uint64
}

var errClosed = errors.New("the remote session closed")

func (c *channel) write(ctx context.Context, plain []byte) error {
	if len(plain) > maxPlain {
		return fmt.Errorf("remote: a %d-byte message is larger than a Noise message", len(plain))
	}
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	out, err := c.send.Encrypt(nil, nil, plain)
	if err != nil {
		return err
	}
	if c.sent++; c.sent%c.every() == 0 {
		c.send.Rekey()
	}
	return c.ws.Write(ctx, websocket.MessageBinary, out)
}

// read is called from one goroutine only.
func (c *channel) read(ctx context.Context) ([]byte, error) {
	kind, msg, err := c.ws.Read(ctx)
	if err != nil {
		return nil, err
	}
	if kind != websocket.MessageBinary {
		return nil, errors.New("remote: a text frame inside a session")
	}
	plain, err := c.recv.Decrypt(nil, nil, msg)
	if err != nil {
		return nil, fmt.Errorf("remote: a message failed its integrity check: %w", err)
	}
	if c.received++; c.received%c.every() == 0 {
		c.recv.Rekey()
	}
	return plain, nil
}

func (c *channel) every() uint64 {
	if c.rekey == 0 {
		return rekeyEvery.Load()
	}
	return c.rekey
}

func (c *channel) close(reason string) {
	_ = c.ws.Close(websocket.StatusNormalClosure, reason)
}

// handshake runs a Noise handshake over a WebSocket and returns the
// session. payloads are what this side sends in its handshake messages, in
// order; the last payload the other side sent is returned with the session.
func handshake(ctx context.Context, ws *websocket.Conn, hs *noise.HandshakeState, initiator bool, payloads [][]byte) (*channel, []byte, error) {
	sending := initiator
	var got []byte
	mine := 0
	for {
		var cs1, cs2 *noise.CipherState
		var err error
		if sending {
			var p []byte
			if mine < len(payloads) {
				p = payloads[mine]
			}
			mine++
			var msg []byte
			msg, cs1, cs2, err = hs.WriteMessage(nil, p)
			if err != nil {
				return nil, nil, err
			}
			if err := ws.Write(ctx, websocket.MessageBinary, msg); err != nil {
				return nil, nil, err
			}
		} else {
			kind, msg, rerr := ws.Read(ctx)
			if rerr != nil {
				return nil, nil, rerr
			}
			if kind != websocket.MessageBinary {
				return nil, nil, errors.New("remote: a text frame in the handshake")
			}
			got, cs1, cs2, err = hs.ReadMessage(nil, msg)
			if err != nil {
				return nil, nil, fmt.Errorf("remote: the handshake failed: %w", err)
			}
		}
		if cs1 != nil {
			// cs1 encrypts what the initiator sends, cs2 what it receives.
			c := &channel{ws: ws, rekey: rekeyEvery.Load()}
			if initiator {
				c.send, c.recv = cs1, cs2
			} else {
				c.send, c.recv = cs2, cs1
			}
			return c, got, nil
		}
		sending = !sending
	}
}

// timestamp is the handshake's freshness mark: nanoseconds, big-endian.
func timestamp(ns int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(ns))
	return b
}
