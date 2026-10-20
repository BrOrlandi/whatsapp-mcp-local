package remote

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
)

// Frames inside a session: one byte of type, four of stream, then the
// payload. A stream is one HTTP request and its response.
const (
	frameReqHead  = 1 // {"m":method,"p":path,"h":headers}
	frameReqBody  = 2
	frameReqEnd   = 3
	frameRespHead = 4 // {"s":status,"h":headers}
	frameRespBody = 5
	frameRespEnd  = 6
	frameCancel   = 7
	framePing     = 8
	framePong     = 9
	frameRotate   = 10 // {"key": the device's next public key}
	frameRotated  = 11
)

const chunk = 32 << 10

type reqHead struct {
	Method string      `json:"m"`
	Path   string      `json:"p"`
	Header http.Header `json:"h"`
}

type respHead struct {
	Status int         `json:"s"`
	Header http.Header `json:"h"`
}

func frame(kind byte, stream uint32, payload []byte) []byte {
	b := make([]byte, 5+len(payload))
	b[0] = kind
	binary.BigEndian.PutUint32(b[1:], stream)
	copy(b[5:], payload)
	return b
}

func parseFrame(b []byte) (byte, uint32, []byte, error) {
	if len(b) < 5 {
		return 0, 0, nil, errors.New("remote: a frame too short")
	}
	return b[0], binary.BigEndian.Uint32(b[1:5]), b[5:], nil
}

// hopHeaders never cross: they belong to one connection, or to the
// computer the request came from. Authorization does cross: a program on
// the app's computer presents the server's token as it would on the
// server, and the server checks it.
var hopHeaders = []string{"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer",
	"Transfer-Encoding", "Upgrade", "Cookie", "Host"}

func cleanHeader(h http.Header) http.Header {
	out := h.Clone()
	if out == nil {
		out = http.Header{}
	}
	for _, k := range hopHeaders {
		out.Del(k)
	}
	return out
}

// sendBody streams r as body frames of a stream, then the end frame. The
// frames are written with the session's own context: a request cancelled
// midway stops sending (ctx), it must not close the session, which a write
// with a cancelled context would.
func sendBody(ctx context.Context, ch *channel, kind, end byte, stream uint32, r io.Reader) error {
	if r != nil {
		buf := make([]byte, chunk)
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			n, err := r.Read(buf)
			if n > 0 {
				if werr := ch.write(context.Background(), frame(kind, stream, buf[:n])); werr != nil {
					return werr
				}
			}
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return ch.write(context.Background(), frame(end, stream, nil))
}

// ---- the server's side: requests in, responses out ----

// responseWriter streams a handler's response back over the session.
type responseWriter struct {
	// ctx is the session's: writes never close the session. stop is the
	// request's: once it is cancelled, nothing more is sent for it.
	ctx    context.Context
	stop   context.Context
	ch     *channel
	stream uint32
	header http.Header
	wrote  bool
	err    error
}

func (w *responseWriter) Header() http.Header { return w.header }

func (w *responseWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.wrote = true
	if w.stopped() {
		return
	}
	head, _ := json.Marshal(respHead{Status: status, Header: cleanHeader(w.header)})
	w.err = w.ch.write(w.ctx, frame(frameRespHead, w.stream, head))
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if w.err != nil {
		return 0, w.err
	}
	n := 0
	for len(b) > 0 {
		if w.stopped() {
			return n, w.err
		}
		part := b[:min(len(b), chunk)]
		if err := w.ch.write(w.ctx, frame(frameRespBody, w.stream, part)); err != nil {
			w.err = err
			return n, err
		}
		n += len(part)
		b = b[len(part):]
	}
	return n, nil
}

func (w *responseWriter) Flush() {}

// stopped reports whether the request was cancelled, which ends its
// response quietly.
func (w *responseWriter) stopped() bool {
	if w.stop != nil && w.stop.Err() != nil {
		if w.err == nil {
			w.err = w.stop.Err()
		}
		return true
	}
	return false
}

func (w *responseWriter) finish() {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if w.err == nil && !w.stopped() {
		_ = w.ch.write(w.ctx, frame(frameRespEnd, w.stream, nil))
	}
}

// ---- the client's side: a RoundTripper over the session ----

type pending struct {
	head chan respHead
	body chan []byte // nil ends the body
}

type clientMux struct {
	ch *channel

	mu      sync.Mutex
	next    uint32
	streams map[uint32]*pending
	closed  error
	pongs   chan struct{}
	rotated chan error
}

func newClientMux(ch *channel) *clientMux {
	return &clientMux{ch: ch, streams: map[uint32]*pending{}, pongs: make(chan struct{}, 1), rotated: make(chan error, 1)}
}

// deliver queues part of a response body; a reader that falls too far
// behind loses its response, not the session.
func (m *clientMux) deliver(stream uint32, p *pending, part []byte) {
	select {
	case p.body <- part:
	default:
		_ = m.ch.write(context.Background(), frame(frameCancel, stream, nil))
		m.mu.Lock()
		if m.streams[stream] == p {
			delete(m.streams, stream)
			close(p.body)
		}
		m.mu.Unlock()
	}
}

// run reads the session until it ends, handing each frame to its stream.
func (m *clientMux) run(ctx context.Context) error {
	for {
		msg, err := m.ch.read(ctx)
		if err != nil {
			m.fail(err)
			return err
		}
		kind, stream, payload, err := parseFrame(msg)
		if err != nil {
			m.fail(err)
			return err
		}
		switch kind {
		case framePong:
			select {
			case m.pongs <- struct{}{}:
			default:
			}
		case frameRotated:
			var r struct {
				Error string `json:"error"`
			}
			_ = json.Unmarshal(payload, &r)
			var rerr error
			if r.Error != "" {
				rerr = errors.New(r.Error)
			}
			select {
			case m.rotated <- rerr:
			default:
			}
		case frameRespHead:
			var h respHead
			if json.Unmarshal(payload, &h) != nil {
				continue
			}
			if p := m.get(stream); p != nil {
				select {
				case p.head <- h:
				default: // a second head for the same stream: ignored
				}
			}
		case frameRespBody:
			if p := m.get(stream); p != nil && len(payload) > 0 {
				m.deliver(stream, p, append([]byte{}, payload...))
			}
		case frameRespEnd:
			if p := m.get(stream); p != nil {
				m.deliver(stream, p, nil)
				m.drop(stream)
			}
		}
	}
}

func (m *clientMux) get(stream uint32) *pending {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.streams[stream]
}

func (m *clientMux) drop(stream uint32) {
	m.mu.Lock()
	delete(m.streams, stream)
	m.mu.Unlock()
}

func (m *clientMux) fail(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed == nil {
		m.closed = err
	}
	for id, p := range m.streams {
		close(p.body)
		close(p.head)
		delete(m.streams, id)
	}
}

// RoundTrip sends one request over the session and returns its response,
// whose body streams in as it arrives.
func (m *clientMux) RoundTrip(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	if m.closed != nil {
		m.mu.Unlock()
		return nil, errClosed
	}
	m.next++
	stream := m.next
	p := &pending{head: make(chan respHead, 1), body: make(chan []byte, streamQueue)}
	m.streams[stream] = p
	m.mu.Unlock()

	ctx := req.Context()
	path := req.URL.RequestURI()
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	head, _ := json.Marshal(reqHead{Method: req.Method, Path: path, Header: cleanHeader(req.Header)})
	if err := m.ch.write(context.Background(), frame(frameReqHead, stream, head)); err != nil {
		m.drop(stream)
		return nil, err
	}
	var body io.Reader
	if req.Body != nil {
		body = req.Body
		defer req.Body.Close()
	}
	if err := sendBody(ctx, m.ch, frameReqBody, frameReqEnd, stream, body); err != nil {
		if ctx.Err() != nil {
			_ = m.ch.write(context.Background(), frame(frameCancel, stream, nil))
		}
		m.drop(stream)
		return nil, err
	}
	select {
	case h, ok := <-p.head:
		if !ok {
			return nil, errClosed
		}
		resp := &http.Response{StatusCode: h.Status, Status: http.StatusText(h.Status), Header: h.Header,
			Body: &responseBody{ctx: ctx, frames: p.body, cancel: func() {
				_ = m.ch.write(context.Background(), frame(frameCancel, stream, nil))
				m.drop(stream)
			}}, Request: req, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1, ContentLength: -1}
		if resp.Header == nil {
			resp.Header = http.Header{}
		}
		return resp, nil
	case <-ctx.Done():
		_ = m.ch.write(context.Background(), frame(frameCancel, stream, nil))
		m.drop(stream)
		return nil, ctx.Err()
	}
}

// responseBody is a response's body, from the frames the mux queued; a nil
// frame ends it, and a closed queue means the session or the stream ended.
type responseBody struct {
	ctx    context.Context
	frames chan []byte
	cur    []byte
	done   bool
	// cancel tells the server to stop sending: closed before its end, the
	// body is not wanted any more.
	cancel func()
	closed sync.Once
}

func (b *responseBody) Read(p []byte) (int, error) {
	for len(b.cur) == 0 {
		if b.done {
			return 0, io.EOF
		}
		select {
		case part, ok := <-b.frames:
			if !ok {
				return 0, errClosed
			}
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

func (b *responseBody) Close() error {
	if !b.done && b.cancel != nil {
		b.closed.Do(b.cancel)
	}
	return nil
}
