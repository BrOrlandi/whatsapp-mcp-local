// Package webhook delivers new WhatsApp messages to addresses the person
// configured, so scripts can act on them.
//
// wacli's sync posts each live message to a relay on loopback that only this
// process knows (a fresh secret every run), and the relay hands it to the
// manager, which delivers it to every webhook that wants it. wacli's own
// delivery is best effort and silent; doing it here gives each webhook a queue
// in order, retries, a record of the last delivery, and the rule that turns a
// webhook off when it stops answering: ten failed attempts, all within a
// minute, and the webhook is disabled and its queue discarded.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/account"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

// Events a webhook can receive.
var Events = []string{"message", "reaction", "receipt", "online"}

// Event is one thing that happened, as webhooks receive it.
type Event struct {
	Kind    string    `json:"event"`
	ID      string    `json:"delivery_id"`
	At      time.Time `json:"sent_at"`
	Message *Message  `json:"message,omitempty"`
	Receipt *Receipt  `json:"receipt,omitempty"`
	Online  *Online   `json:"online,omitempty"`
	// Account is the WhatsApp number the event happened on, so one script
	// can serve several numbers.
	Account *account.Info `json:"account,omitempty"`
	Test    bool          `json:"test,omitempty"`

	// chat and fromMe decide which webhooks want it.
	chat   string
	fromMe bool
}

// Message is a new message: received, or sent by the account from another
// device or by a tool.
type Message struct {
	ID         string     `json:"id"`
	ChatJID    string     `json:"chat_jid"`
	ChatName   string     `json:"chat_name,omitempty"`
	Group      bool       `json:"group"`
	Timestamp  time.Time  `json:"timestamp"`
	FromMe     bool       `json:"from_me"`
	SenderJID  string     `json:"sender_jid,omitempty"`
	SenderName string     `json:"sender_name,omitempty"`
	Text       string     `json:"text,omitempty"`
	Media      *Media     `json:"media,omitempty"`
	ReplyTo    *Reference `json:"reply_to,omitempty"`
	Reaction   *Reaction  `json:"reaction,omitempty"`
	Forwarded  bool       `json:"forwarded,omitempty"`
	Edited     bool       `json:"edited,omitempty"`
	Revoked    bool       `json:"revoked,omitempty"`
	Location   any        `json:"location,omitempty"`
	Poll       any        `json:"poll,omitempty"`
}

type Media struct {
	Type     string `json:"type"`
	MimeType string `json:"mime_type,omitempty"`
	Filename string `json:"filename,omitempty"`
	Bytes    uint64 `json:"bytes,omitempty"`
	Caption  string `json:"caption,omitempty"`
}

type Reference struct {
	ID        string `json:"id"`
	SenderJID string `json:"sender_jid,omitempty"`
	Text      string `json:"text,omitempty"`
}

type Reaction struct {
	To    string `json:"to"`
	Emoji string `json:"emoji"`
}

// Receipt says messages the account sent were delivered, read or played.
type Receipt struct {
	ChatJID    string    `json:"chat_jid"`
	SenderJID  string    `json:"sender_jid,omitempty"`
	MessageIDs []string  `json:"message_ids"`
	Type       string    `json:"type"`
	Timestamp  time.Time `json:"timestamp"`
}

// Online says WhatsApp is reachable again on this computer: the app started,
// the computer woke up, or the connection came back after a while away. It
// waits up to three minutes for the history the phone sends on connecting to
// settle, so a script that reads the chats then usually sees what arrived
// meanwhile; a history that takes longer is still arriving when it comes.
type Online struct {
	// Reason is started (the app started, with the computer or by hand),
	// woke (the computer woke from sleep) or reconnected (the internet or
	// WhatsApp came back after at least two minutes away).
	Reason string `json:"reason"`
	// OfflineSince is the last moment the app knew it was connected, when it
	// knows one; OfflineSeconds is how long it was away, until the connection
	// came back (the wait for the history is not counted).
	OfflineSince   *time.Time `json:"offline_since,omitempty"`
	OfflineSeconds int64      `json:"offline_seconds,omitempty"`
}

// Retry schedule: ten attempts, each with a short timeout, the waits between
// them growing. Ten failures in a row fit in under a minute; then the webhook
// is turned off.
var (
	retryWaits     = []time.Duration{500 * time.Millisecond, time.Second, 1500 * time.Millisecond, 2 * time.Second, 2500 * time.Millisecond, 3 * time.Second, 4 * time.Second, 5 * time.Second, 6 * time.Second}
	attemptTimeout = 3 * time.Second
	maxQueue       = 1000
)

// Same reports whether two chat JIDs are the same conversation (a person's
// phone number and LID). The daemon wires it to the index's names.
type Same func(ctx context.Context, a, b string) bool

// Manager holds the webhooks and their queues.
type Manager struct {
	st      *state.State
	logger  *slog.Logger
	client  *http.Client
	version string
	same    Same
	account func(context.Context) account.Info

	mu      sync.Mutex
	workers map[string]*worker
	ctx     context.Context
	// seen keeps the account's own messages dispatched lately, so one
	// reported by the tools and also seen by sync goes out once.
	seen map[string]time.Time
}

type worker struct {
	hook    state.Webhook
	queue   []Event
	dropped int
	wake    chan struct{}
	stop    context.CancelFunc
}

func New(st *state.State, logger *slog.Logger, version string, same Same) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{st: st, logger: logger, version: version, same: same, workers: map[string]*worker{},
		client: &http.Client{Timeout: attemptTimeout, Transport: transport(),
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// transport is the default one, except that it never connects to a
// link-local address or a cloud's metadata service. A webhook may post to
// this computer and its network (n8n beside the app, a LAN automation),
// which is what they are for; the metadata service is never a receiver, and
// on a server it hands out the machine's credentials.
func transport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second, Control: refuseMetadata}).DialContext
	return t
}

// errMetadata is the refusal of a link-local or metadata address.
var errMetadata = errors.New("webhooks do not reach link-local or cloud metadata addresses")

// metadata are the metadata services outside the link-local ranges.
var metadata = []netip.Addr{
	netip.MustParseAddr("fd00:ec2::254"),   // AWS, over IPv6
	netip.MustParseAddr("100.100.100.200"), // Alibaba Cloud
}

// metadataNames are the metadata services' own names, refused when a proxy
// dials for us and the address is never seen here.
var metadataNames = []string{"metadata.google.internal", "metadata.goog", "metadata"}

// viaProxy refuses, before a request leaves through a proxy from the
// environment (which dials the address itself, out of refuseMetadata's
// sight), a URL whose host is a link-local or metadata address, or a
// metadata service's name.
func viaProxy(t *http.Transport, req *http.Request) error {
	if t.Proxy == nil {
		return nil
	}
	if proxy, err := t.Proxy(req); err != nil || proxy == nil {
		return nil
	}
	host := strings.TrimSuffix(strings.ToLower(req.URL.Hostname()), ".")
	if slices.Contains(metadataNames, host) {
		return fmt.Errorf("%s: %w", host, errMetadata)
	}
	if a, err := netip.ParseAddr(host); err == nil {
		return refuseMetadata("", netip.AddrPortFrom(a, 80).String(), nil)
	}
	return nil
}

// refuseMetadata runs on the address actually dialled, after DNS, so a name
// that resolves to one is refused too.
func refuseMetadata(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return nil
	}
	a := ap.Addr().Unmap()
	if a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() || slices.Contains(metadata, a) {
		return fmt.Errorf("%s: %w", a, errMetadata)
	}
	return nil
}

// SetAccount names the number every event comes from. Call it before Start.
func (m *Manager) SetAccount(f func(context.Context) account.Info) { m.account = f }

// stamp writes the number on an event.
func (m *Manager) stamp(ctx context.Context, ev *Event) {
	if m.account == nil {
		return
	}
	a := m.account(ctx)
	a.Name = ""
	ev.Account = &a
}

// Start begins delivering to the enabled webhooks, until ctx ends.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	hooks, err := m.st.Webhooks(ctx)
	if err != nil {
		return err
	}
	for _, h := range hooks {
		if h.Enabled {
			m.run(h)
		}
	}
	return nil
}

// run starts (or restarts, with an empty queue) the worker of a webhook.
func (m *Manager) run(h state.Webhook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.workers[h.ID]; ok {
		old.stop()
		delete(m.workers, h.ID)
	}
	if m.ctx == nil || !h.Enabled {
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	w := &worker{hook: h, wake: make(chan struct{}, 1), stop: cancel}
	m.workers[h.ID] = w
	go m.loop(ctx, w)
}

func (m *Manager) halt(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w, ok := m.workers[id]; ok {
		w.stop()
		delete(m.workers, id)
	}
}

// Dispatch queues an event for every enabled webhook that wants it.
func (m *Manager) Dispatch(ctx context.Context, ev Event) {
	m.mu.Lock()
	if m.duplicate(ev) {
		m.mu.Unlock()
		return
	}
	workers := make([]*worker, 0, len(m.workers))
	for _, w := range m.workers {
		workers = append(workers, w)
	}
	m.mu.Unlock()
	if len(workers) > 0 {
		m.stamp(ctx, &ev)
	}
	for _, w := range workers {
		if !m.wants(ctx, w.hook, ev) {
			continue
		}
		m.mu.Lock()
		if len(w.queue) >= maxQueue {
			w.queue = w.queue[1:]
			w.dropped++
		}
		w.queue = append(w.queue, ev)
		m.mu.Unlock()
		select {
		case w.wake <- struct{}{}:
		default:
		}
	}
}

// duplicate says whether an own message went out already, and remembers it.
// Called with mu held.
func (m *Manager) duplicate(ev Event) bool {
	if !ev.fromMe || ev.Message == nil || ev.Message.ID == "" {
		return false
	}
	now := time.Now()
	key := ev.Kind + "/" + ev.Message.ID
	if ev.Message.Edited {
		key += "/edited"
	}
	if ev.Message.Revoked {
		key += "/revoked"
	}
	if at, ok := m.seen[key]; ok && now.Sub(at) < 10*time.Minute {
		return true
	}
	if m.seen == nil || len(m.seen) > 1000 {
		m.seen = map[string]time.Time{}
	}
	m.seen[key] = now
	return false
}

func (m *Manager) wants(ctx context.Context, h state.Webhook, ev Event) bool {
	if !slices.Contains(h.Events, ev.Kind) {
		return false
	}
	if ev.Kind == "online" {
		return true // about the number, not a chat: the chat filter does not apply
	}
	if ev.fromMe && !h.IncludeOwn {
		return false
	}
	if len(h.Chats) == 0 {
		return true
	}
	for _, c := range h.Chats {
		if c == ev.chat || (m.same != nil && m.same(ctx, c, ev.chat)) {
			return true
		}
	}
	return false
}

func (m *Manager) loop(ctx context.Context, w *worker) {
	for {
		m.mu.Lock()
		var ev Event
		has := len(w.queue) > 0
		if has {
			ev = w.queue[0]
		}
		m.mu.Unlock()
		if !has {
			select {
			case <-ctx.Done():
				return
			case <-w.wake:
				continue
			}
		}
		if last, err := m.deliverWithRetries(ctx, w.hook, ev); err != nil {
			if ctx.Err() != nil {
				return
			}
			reason := i18n.Tf("%d tentativas seguidas sem sucesso em menos de um minuto (a última: %s)", len(retryWaits)+1, last)
			m.logger.Warn("webhook disabled", "id", w.hook.ID, "error", err)
			_ = m.st.DisableWebhook(context.WithoutCancel(ctx), w.hook.ID, time.Now(), reason)
			m.halt(w.hook.ID) // the queue goes with the worker
			return
		}
		m.mu.Lock()
		if len(w.queue) > 0 {
			w.queue = w.queue[1:]
		}
		m.mu.Unlock()
	}
}

// deliverWithRetries tries an event until it is accepted, or until the retry
// schedule runs out, and says how the last attempt went.
func (m *Manager) deliverWithRetries(ctx context.Context, h state.Webhook, ev Event) (string, error) {
	for attempt := 0; ; attempt++ {
		result, err := m.post(ctx, h, ev)
		_ = m.st.RecordDelivery(context.WithoutCancel(ctx), h.ID, time.Now(), result, err == nil)
		if err == nil || attempt >= len(retryWaits) {
			return result, err
		}
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(retryWaits[attempt]):
		}
	}
}

// post sends one event once. Any 2xx answer is a delivery.
func (m *Manager) post(ctx context.Context, h state.Webhook, ev Event) (string, error) {
	body, err := json.Marshal(ev)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		return err.Error(), err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "whatsapp-mcp-local/"+m.version)
	req.Header.Set("X-WhatsApp-MCP-Event", ev.Kind)
	req.Header.Set("X-WhatsApp-MCP-Delivery", ev.ID)
	req.Header.Set("X-WhatsApp-MCP-Signature", Sign(h.Secret, body))
	if t, ok := m.client.Transport.(*http.Transport); ok {
		if err := viaProxy(t, req); err != nil {
			return describe(err), err
		}
	}
	resp, err := m.client.Do(req)
	if err != nil {
		said := describe(err)
		if said == unknownFailure() {
			// What went wrong in the program's own words, for the log only.
			m.logger.Warn("webhook delivery failed", "id", h.ID, "error", err)
		}
		return said, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return i18n.Tf("respondeu %s", resp.Status), fmt.Errorf("answered %s", resp.Status)
	}
	return resp.Status, nil
}

// describe says why an attempt failed in words the settings page can show.
// Anything else is said in general words: the error's own text, which can
// tell what is on the network inside, goes only to the log.
func describe(err error) string {
	var dns *net.DNSError
	var ue *url.Error
	var cert *tls.CertificateVerificationError
	switch {
	case errors.Is(err, errMetadata):
		return i18n.T("esse endereço não pode receber webhooks")
	case errors.Is(err, errRefused):
		return i18n.T("nenhum programa respondeu nesse endereço")
	case errors.As(err, &dns):
		return i18n.T("o endereço não foi encontrado")
	case errors.As(err, &ue) && ue.Timeout():
		return i18n.Tf("não respondeu em %d segundos", int(attemptTimeout.Seconds()))
	case errors.As(err, &cert):
		return i18n.T("o certificado do endereço não foi aceito")
	}
	return unknownFailure()
}

func unknownFailure() string { return i18n.T("não foi possível entregar nesse endereço") }

// Sign is the signature a webhook receives in X-WhatsApp-MCP-Signature: the
// HMAC-SHA256 of the body with the webhook's secret, in hex, after "sha256=".
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---- configuration ----

// Settings is what a webhook is created or changed with.
type Settings struct {
	URL        *string   `json:"url"`
	Events     *[]string `json:"events"`
	IncludeOwn *bool     `json:"include_own"`
	Chats      *[]string `json:"chats"`
	Enabled    *bool     `json:"enabled"`
}

// UserError is a request to fix rather than a failure. Err, when set, says
// which one (ErrBadURL), for a caller that words it its own way.
type UserError struct {
	Msg string
	Err error
}

func (e UserError) Error() string { return e.Msg }
func (e UserError) Unwrap() error { return e.Err }

// ErrBadURL is a webhook address that is not a whole http:// or https:// one.
var ErrBadURL = errors.New("webhook: not a whole http:// or https:// address")

func validURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return UserError{Msg: i18n.T("url precisa ser um endereço http:// ou https:// completo"), Err: ErrBadURL}
	}
	return nil
}

func validEvents(events []string) error {
	if len(events) == 0 {
		return UserError{Msg: i18n.Tf("events precisa ter ao menos um de: %s", strings.Join(Events, ", "))}
	}
	for _, e := range events {
		if !slices.Contains(Events, e) {
			return UserError{Msg: i18n.Tf("evento %q desconhecido; os eventos são %s", e, strings.Join(Events, ", "))}
		}
	}
	return nil
}

// Create adds a webhook and starts delivering to it. The secret is returned
// here only: it signs every delivery.
func (m *Manager) Create(ctx context.Context, s Settings) (state.Webhook, string, error) {
	if s.URL == nil {
		return state.Webhook{}, "", UserError{Msg: i18n.T("url é obrigatório")}
	}
	h := state.Webhook{ID: randomHex(6), URL: strings.TrimSpace(*s.URL), Secret: randomHex(24), Events: []string{"message"},
		Enabled: true, CreatedAt: time.Now().UTC()}
	if err := validURL(h.URL); err != nil {
		return h, "", err
	}
	if s.Events != nil {
		h.Events = *s.Events
	}
	if err := validEvents(h.Events); err != nil {
		return h, "", err
	}
	if s.IncludeOwn != nil {
		h.IncludeOwn = *s.IncludeOwn
	}
	if s.Chats != nil {
		h.Chats = cleanList(*s.Chats)
	}
	if s.Enabled != nil {
		h.Enabled = *s.Enabled
	}
	if err := m.st.SaveWebhook(ctx, h); err != nil {
		return h, "", err
	}
	m.run(h)
	return h, h.Secret, nil
}

// Update changes a webhook. Turning a disabled one back on starts it with an
// empty queue.
func (m *Manager) Update(ctx context.Context, id string, s Settings) (state.Webhook, error) {
	h, err := m.st.Webhook(ctx, id)
	if err != nil {
		return h, err
	}
	if s.URL != nil {
		if err := validURL(*s.URL); err != nil {
			return h, err
		}
		h.URL = strings.TrimSpace(*s.URL)
	}
	if s.Events != nil {
		if err := validEvents(*s.Events); err != nil {
			return h, err
		}
		h.Events = *s.Events
	}
	if s.IncludeOwn != nil {
		h.IncludeOwn = *s.IncludeOwn
	}
	if s.Chats != nil {
		h.Chats = cleanList(*s.Chats)
	}
	if s.Enabled != nil {
		h.Enabled = *s.Enabled
	}
	if err := m.st.SaveWebhook(ctx, h); err != nil {
		return h, err
	}
	if h.Enabled {
		if err := m.st.EnableWebhook(ctx, id); err != nil {
			return h, err
		}
	}
	if h, err = m.st.Webhook(ctx, id); err != nil {
		return h, err
	}
	if h.Enabled {
		m.run(h)
	} else {
		m.halt(id)
	}
	return h, nil
}

// Delete removes a webhook and drops its queue.
func (m *Manager) Delete(ctx context.Context, id string) error {
	m.halt(id)
	return m.st.DeleteWebhook(ctx, id)
}

// List is every webhook with how much waits in its queue.
func (m *Manager) List(ctx context.Context) ([]Status, error) {
	hooks, err := m.st.Webhooks(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Status, 0, len(hooks))
	for _, h := range hooks {
		out = append(out, m.status(h))
	}
	return out, nil
}

// Get is one webhook with its queue.
func (m *Manager) Get(ctx context.Context, id string) (Status, error) {
	h, err := m.st.Webhook(ctx, id)
	if err != nil {
		return Status{}, err
	}
	return m.status(h), nil
}

// Status is a webhook as the API shows it.
type Status struct {
	state.Webhook
	Queued  int `json:"queued"`
	Dropped int `json:"dropped,omitempty"`
}

func (m *Manager) status(h state.Webhook) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := Status{Webhook: h}
	if w, ok := m.workers[h.ID]; ok {
		st.Queued, st.Dropped = len(w.queue), w.dropped
	}
	return st
}

// TestMessage is the message of a test event. Like every event, it is the
// same whatever language the panel shows: the language is the interface's,
// and the programs that receive the webhooks see one contract.
func TestMessage(at time.Time) *Message {
	return &Message{ID: "TEST", ChatJID: "5511900000000@s.whatsapp.net", ChatName: "Teste", Timestamp: at,
		SenderJID: "5511900000000@s.whatsapp.net", SenderName: "Teste", Text: "Mensagem de teste do WhatsApp MCP"}
}

// Test sends one test event to a webhook now, once, and says what came back.
// It neither queues nor counts toward turning the webhook off.
func (m *Manager) Test(ctx context.Context, id string) (map[string]any, error) {
	h, err := m.st.Webhook(ctx, id)
	if err != nil {
		return nil, err
	}
	ev := Event{Kind: "message", ID: randomHex(8), At: time.Now().UTC(), Test: true, Message: TestMessage(time.Now().UTC())}
	m.stamp(ctx, &ev)
	started := time.Now()
	result, err := m.post(ctx, h, ev)
	out := map[string]any{"ok": err == nil, "result": result, "ms": time.Since(started).Milliseconds()}
	if err != nil {
		// The error's own words only for a caller on this computer; from
		// outside, they could tell what is on the network inside.
		out["error"] = result
		if caller.From(ctx).Kind == caller.Local {
			out["error"] = err.Error()
		} else {
			m.logger.Warn("webhook test failed", "id", h.ID, "error", err)
		}
	}
	return out, nil
}

func cleanList(v []string) []string {
	var out []string
	for _, s := range v {
		if s = strings.TrimSpace(s); s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}
