package webhook

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Names names chats and people for the events, the way the tools do.
type Names interface {
	Name(ctx context.Context, jid string) string
	Known(ctx context.Context, jid string) string
}

// Relay is where wacli's sync posts each live message: a listener on a random
// loopback port, checking a secret made for this run, so nothing else can
// feed the webhooks.
type Relay struct {
	m      *Manager
	names  Names
	secret string
	ln     net.Listener
	srv    *http.Server
}

// NewRelay opens the relay. Sync is pointed at URL with Secret.
func NewRelay(m *Manager, names Names) (*Relay, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	r := &Relay{m: m, names: names, secret: randomHex(24), ln: ln}
	r.srv = &http.Server{Handler: r, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second}
	go func() { _ = r.srv.Serve(ln) }()
	return r, nil
}

func (r *Relay) URL() string    { return "http://" + r.ln.Addr().String() + "/wacli" }
func (r *Relay) Secret() string { return r.secret }

func (r *Relay) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return r.srv.Shutdown(ctx)
}

func (r *Relay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost || req.URL.Path != "/wacli" {
		http.NotFound(w, req)
		return
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, 4<<20))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if !hmac.Equal([]byte(req.Header.Get("X-Wacli-Signature")), []byte(Sign(r.secret, body))) {
		http.Error(w, "bad signature", http.StatusForbidden)
		return
	}
	ev, err := r.event(req.Context(), body)
	if err == nil {
		r.m.Dispatch(context.WithoutCancel(req.Context()), ev)
	}
	w.WriteHeader(http.StatusOK)
}

// wacliPayload is what wacli's sync posts: a message with Go field names, or,
// with EventType set, a receipt.
type wacliPayload struct {
	EventType string

	Chat             string
	ChatName         string
	ID               string
	SenderJID        string
	PushName         string
	Timestamp        time.Time
	FromMe           bool
	Text             string
	ReplyToID        string
	ReplyToSenderJID string
	ReplyToDisplay   string
	ReactionToID     string
	ReactionEmoji    string
	IsForwarded      bool
	Edited           bool
	Revoked          bool
	Media            *struct {
		Type       string
		Caption    string
		Filename   string
		MimeType   string
		FileLength uint64
	}
	Location *struct {
		Latitude  float64
		Longitude float64
		Name      string
		Address   string
		IsLive    bool
	}
	Poll *struct {
		Question        string
		Options         []string
		SelectableCount uint32
	}

	Sender     string
	MessageIDs []string
	Type       string
}

var errSkip = errors.New("nothing a webhook wants")

func (r *Relay) event(ctx context.Context, body []byte) (Event, error) {
	var p wacliPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return Event{}, err
	}
	ev := Event{ID: randomHex(8), At: time.Now().UTC(), chat: p.Chat}
	switch p.EventType {
	case "receipt":
		if len(p.MessageIDs) == 0 {
			return ev, errSkip
		}
		ev.Kind = "receipt"
		ev.Receipt = &Receipt{ChatJID: p.Chat, SenderJID: p.Sender, MessageIDs: p.MessageIDs, Type: p.Type, Timestamp: p.Timestamp.UTC()}
		return ev, nil
	case "":
	default:
		return ev, errSkip
	}
	if p.ID == "" {
		return ev, errSkip
	}
	m := &Message{ID: p.ID, ChatJID: p.Chat, Group: strings.HasSuffix(p.Chat, "@g.us"), Timestamp: p.Timestamp.UTC(), FromMe: p.FromMe,
		SenderJID: p.SenderJID, Text: p.Text, Forwarded: p.IsForwarded, Edited: p.Edited, Revoked: p.Revoked}
	m.ChatName = p.ChatName
	if m.ChatName == "" || strings.Contains(m.ChatName, "@") {
		m.ChatName = r.names.Name(ctx, p.Chat)
	}
	if !p.FromMe {
		m.SenderName = r.names.Known(ctx, p.SenderJID)
		if m.SenderName == "" {
			m.SenderName = p.PushName
		}
		if m.SenderName == "" {
			m.SenderName = r.names.Name(ctx, p.SenderJID)
		}
	}
	if p.Media != nil {
		m.Media = &Media{Type: p.Media.Type, MimeType: p.Media.MimeType, Filename: p.Media.Filename, Bytes: p.Media.FileLength, Caption: p.Media.Caption}
		if p.Media.Caption != "" {
			m.Text = p.Media.Caption
		} else if strings.HasPrefix(m.Text, "[") {
			m.Text = "" // wacli's placeholder for media without a caption
		}
	}
	if p.ReplyToID != "" {
		m.ReplyTo = &Reference{ID: p.ReplyToID, SenderJID: p.ReplyToSenderJID, Text: p.ReplyToDisplay}
	}
	if p.Location != nil {
		m.Location = map[string]any{"latitude": p.Location.Latitude, "longitude": p.Location.Longitude, "name": p.Location.Name,
			"address": p.Location.Address, "live": p.Location.IsLive}
	}
	if p.Poll != nil {
		m.Poll = map[string]any{"question": p.Poll.Question, "options": p.Poll.Options, "max_answers": p.Poll.SelectableCount}
	}
	ev.Kind, ev.Message, ev.fromMe = "message", m, p.FromMe
	if p.ReactionToID != "" {
		ev.Kind = "reaction"
		m.Reaction = &Reaction{To: p.ReactionToID, Emoji: p.ReactionEmoji}
	} else if m.Text == "" && m.Media == nil && m.Location == nil && m.Poll == nil && !m.Revoked {
		return ev, errSkip // a protocol message, a poll vote, a call: nothing to read
	}
	return ev, nil
}
