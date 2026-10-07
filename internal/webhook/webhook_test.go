package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

func init() {
	// The real schedule spans most of a minute; the tests keep its shape.
	retryWaits = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond, time.Millisecond, time.Millisecond,
		time.Millisecond, time.Millisecond, time.Millisecond, time.Millisecond}
	attemptTimeout = time.Second
}

type receiver struct {
	srv    *httptest.Server
	mu     sync.Mutex
	got    []Event
	sigOK  []bool
	calls  atomic.Int32
	status func(n int32) int
	secret string
}

func newReceiver(t *testing.T, status func(n int32) int) *receiver {
	r := &receiver{status: status}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		n := r.calls.Add(1)
		body, _ := io.ReadAll(req.Body)
		var ev Event
		_ = json.Unmarshal(body, &ev)
		r.mu.Lock()
		r.got = append(r.got, ev)
		r.sigOK = append(r.sigOK, req.Header.Get("X-WhatsApp-MCP-Signature") == Sign(r.secret, body) && req.Header.Get("X-WhatsApp-MCP-Event") == ev.Kind)
		r.mu.Unlock()
		w.WriteHeader(r.status(n))
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *receiver) events() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.got...)
}

func manager(t *testing.T) (*Manager, *state.State) {
	t.Helper()
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	m := New(st, nil, "test", nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	return m, st
}

func message(id, chat string, fromMe bool) Event {
	return Event{Kind: "message", ID: id, At: time.Now(), chat: chat, fromMe: fromMe,
		Message: &Message{ID: id, ChatJID: chat, FromMe: fromMe, Text: "oi " + id}}
}

func ptr[T any](v T) *T { return &v }

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestDeliversInOrderSigned(t *testing.T) {
	m, st := manager(t)
	r := newReceiver(t, func(int32) int { return http.StatusNoContent })
	h, secret, err := m.Create(context.Background(), Settings{URL: ptr(r.srv.URL)})
	if err != nil {
		t.Fatal(err)
	}
	r.secret = secret
	for _, id := range []string{"A", "B", "C"} {
		m.Dispatch(context.Background(), message(id, "1@s.whatsapp.net", false))
	}
	waitFor(t, "three deliveries", func() bool { return len(r.events()) == 3 })
	for i, ev := range r.events() {
		if ev.Message.ID != []string{"A", "B", "C"}[i] || !r.sigOK[i] {
			t.Errorf("delivery %d: %+v signed %v", i, ev.Message, r.sigOK[i])
		}
	}
	waitFor(t, "the record", func() bool { w, _ := st.Webhook(context.Background(), h.ID); return w.Delivered == 3 })
}

func TestTenFailuresDisableAndDropTheQueue(t *testing.T) {
	m, st := manager(t)
	r := newReceiver(t, func(int32) int { return http.StatusInternalServerError })
	h, _, err := m.Create(context.Background(), Settings{URL: ptr(r.srv.URL)})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"A", "B", "C"} {
		m.Dispatch(context.Background(), message(id, "1@s.whatsapp.net", false))
	}
	waitFor(t, "the webhook to be disabled", func() bool { w, _ := st.Webhook(context.Background(), h.ID); return !w.Enabled })
	time.Sleep(50 * time.Millisecond)
	if n := r.calls.Load(); n != 10 {
		t.Errorf("%d attempts, want 10", n)
	}
	for _, ev := range r.events() {
		if ev.Message.ID != "A" {
			t.Errorf("the queue should wait behind the first event, got %s", ev.Message.ID)
		}
	}
	got, _ := m.Get(context.Background(), h.ID)
	if got.Queued != 0 || got.DisabledReason == "" || got.DisabledAt == nil {
		t.Errorf("after disabling: %+v", got)
	}
	// Turned back on, it starts with an empty queue: B and C are gone.
	r.status = func(int32) int { return http.StatusOK }
	if _, err := m.Update(context.Background(), h.ID, Settings{Enabled: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	m.Dispatch(context.Background(), message("D", "1@s.whatsapp.net", false))
	waitFor(t, "the new event", func() bool { evs := r.events(); return evs[len(evs)-1].Message.ID == "D" })
	if w, _ := st.Webhook(context.Background(), h.ID); !w.Enabled || w.DisabledReason != "" {
		t.Errorf("re-enabled: %+v", w)
	}
}

func TestRetriesThroughABlip(t *testing.T) {
	m, st := manager(t)
	r := newReceiver(t, func(n int32) int {
		if n <= 3 {
			return http.StatusBadGateway
		}
		return http.StatusOK
	})
	h, _, _ := m.Create(context.Background(), Settings{URL: ptr(r.srv.URL)})
	m.Dispatch(context.Background(), message("A", "1@s.whatsapp.net", false))
	m.Dispatch(context.Background(), message("B", "1@s.whatsapp.net", false))
	waitFor(t, "both delivered", func() bool { w, _ := st.Webhook(context.Background(), h.ID); return w.Delivered == 2 })
	if w, _ := st.Webhook(context.Background(), h.ID); !w.Enabled {
		t.Error("a webhook that recovered within its retries must stay on")
	}
}

func TestFilters(t *testing.T) {
	m, _ := manager(t)
	r := newReceiver(t, func(int32) int { return http.StatusOK })
	_, _, err := m.Create(context.Background(), Settings{URL: ptr(r.srv.URL), Events: ptr([]string{"message"}), Chats: ptr([]string{"1@s.whatsapp.net"})})
	if err != nil {
		t.Fatal(err)
	}
	m.Dispatch(context.Background(), message("other-chat", "2@s.whatsapp.net", false))
	m.Dispatch(context.Background(), message("own", "1@s.whatsapp.net", true))
	reaction := message("reaction", "1@s.whatsapp.net", false)
	reaction.Kind = "reaction"
	m.Dispatch(context.Background(), reaction)
	m.Dispatch(context.Background(), message("wanted", "1@s.whatsapp.net", false))
	waitFor(t, "the wanted event", func() bool { return len(r.events()) >= 1 })
	time.Sleep(50 * time.Millisecond)
	if evs := r.events(); len(evs) != 1 || evs[0].Message.ID != "wanted" {
		t.Errorf("delivered %+v, want only the wanted one", evs)
	}
	if _, _, err := m.Create(context.Background(), Settings{URL: ptr("ftp://x")}); err == nil {
		t.Error("a non-http url must be refused")
	}
	if _, _, err := m.Create(context.Background(), Settings{URL: ptr(r.srv.URL), Events: ptr([]string{"presence"})}); err == nil {
		t.Error("an unknown event must be refused")
	}
}

type names struct{}

func (names) Name(_ context.Context, jid string) string  { return "Nome de " + jid }
func (names) Known(_ context.Context, jid string) string { return "" }

func TestRelay(t *testing.T) {
	m, _ := manager(t)
	r := newReceiver(t, func(int32) int { return http.StatusOK })
	_, secret, _ := m.Create(context.Background(), Settings{URL: ptr(r.srv.URL), Events: ptr(Events)})
	r.secret = secret
	relay, err := NewRelay(m, names{})
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()
	post := func(body string, signature string) int {
		req, _ := http.NewRequest(http.MethodPost, relay.URL(), bytes.NewReader([]byte(body)))
		req.Header.Set("X-Wacli-Signature", signature)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	msg := `{"Chat":"120363000000000001@g.us","ID":"X1","SenderJID":"5511900000001@s.whatsapp.net","PushName":"Mãe","Timestamp":"2026-10-07T10:00:00Z","Text":"[image]","Media":{"Type":"image","MimeType":"image/jpeg","FileLength":1234}}`
	if code := post(msg, "sha256=wrong"); code != http.StatusForbidden {
		t.Fatalf("a wrong signature answered %d", code)
	}
	if code := post(msg, Sign(relay.Secret(), []byte(msg))); code != http.StatusOK {
		t.Fatalf("a signed post answered %d", code)
	}
	reaction := `{"Chat":"5511900000001@s.whatsapp.net","ID":"X2","SenderJID":"5511900000001@s.whatsapp.net","Timestamp":"2026-10-07T10:01:00Z","ReactionToID":"X1","ReactionEmoji":"❤️"}`
	post(reaction, Sign(relay.Secret(), []byte(reaction)))
	receipt := `{"EventType":"receipt","Chat":"5511900000001@s.whatsapp.net","Sender":"5511900000001@s.whatsapp.net","MessageIDs":["Y"],"Type":"read","Timestamp":"2026-10-07T10:02:00Z"}`
	post(receipt, Sign(relay.Secret(), []byte(receipt)))
	empty := `{"Chat":"5511900000001@s.whatsapp.net","ID":"X3","Timestamp":"2026-10-07T10:03:00Z"}`
	post(empty, Sign(relay.Secret(), []byte(empty)))
	waitFor(t, "three deliveries", func() bool { return len(r.events()) >= 3 })
	time.Sleep(50 * time.Millisecond)
	evs := r.events()
	if len(evs) != 3 {
		t.Fatalf("delivered %d events, want 3 (the empty message skipped)", len(evs))
	}
	first := evs[0].Message
	if evs[0].Kind != "message" || !first.Group || first.Text != "" || first.Media == nil || first.Media.Bytes != 1234 ||
		first.SenderName != "Mãe" || first.ChatName != "Nome de 120363000000000001@g.us" {
		t.Errorf("message: %+v", first)
	}
	if evs[1].Kind != "reaction" || evs[1].Message.Reaction == nil || evs[1].Message.Reaction.Emoji != "❤️" {
		t.Errorf("reaction: %+v", evs[1])
	}
	if evs[2].Kind != "receipt" || evs[2].Receipt.Type != "read" || evs[2].Receipt.MessageIDs[0] != "Y" {
		t.Errorf("receipt: %+v", evs[2])
	}
}
