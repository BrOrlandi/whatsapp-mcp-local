package webhook

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"
)

// fakeSync is sync's state as the watcher reads it, changed by the test.
type fakeSync struct {
	mu        sync.Mutex
	state     string
	historyAt time.Time
	changes   chan struct{}
}

func (f *fakeSync) set(state string) {
	f.mu.Lock()
	f.state = state
	f.mu.Unlock()
	select {
	case f.changes <- struct{}{}:
	default:
	}
}

func (f *fakeSync) history(at time.Time) {
	f.mu.Lock()
	f.historyAt = at
	f.mu.Unlock()
}

func (f *fakeSync) read() (string, time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state, f.historyAt
}

func shortOnlineTimes(t *testing.T) {
	old := []time.Duration{heartbeat, settle, settleMax, minOffline}
	heartbeat, settle, settleMax, minOffline = time.Hour, 40*time.Millisecond, 400*time.Millisecond, 150*time.Millisecond
	t.Cleanup(func() { heartbeat, settle, settleMax, minOffline = old[0], old[1], old[2], old[3] })
}

func onlineEvents(r *receiver) []Event {
	var out []Event
	for _, ev := range r.events() {
		if ev.Kind == "online" {
			out = append(out, ev)
		}
	}
	return out
}

func waitOnline(t *testing.T, r *receiver, n int) []Event {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if got := onlineEvents(r); len(got) >= n {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("got %d online events, want %d", len(onlineEvents(r)), n)
	return nil
}

func TestOnline(t *testing.T) {
	shortOnlineTimes(t)
	m, st := manager(t)
	r := newReceiver(t, func(int32) int { return http.StatusOK })
	online := []string{"online"}
	// A chat filter does not hold back the online event: it is about the number.
	chats := []string{"5511900000000@s.whatsapp.net"}
	url := r.srv.URL
	_, secret, err := m.Create(context.Background(), Settings{URL: &url, Events: &online, Chats: &chats})
	if err != nil {
		t.Fatal(err)
	}
	r.secret = secret
	// The app was last connected an hour ago.
	before := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	if err := st.SetSetting(context.Background(), lastOnlineSetting, before.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}

	fs := &fakeSync{state: "starting", changes: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.WatchOnline(ctx, fs.changes, fs.read)

	// Started: connected, with history arriving, then quiet.
	fs.history(time.Now())
	fs.set("connected")
	time.Sleep(20 * time.Millisecond)
	if n := len(onlineEvents(r)); n != 0 {
		t.Fatalf("online before the connection settled: %d", n)
	}
	got := waitOnline(t, r, 1)
	o := got[0].Online
	if o == nil || o.Reason != "started" || o.OfflineSince == nil || !o.OfflineSince.Equal(before) || o.OfflineSeconds < 3500 {
		t.Fatalf("first event = %+v", o)
	}

	// A pause for a tool and a blip of a moment say nothing.
	fs.set("paused")
	fs.set("connected")
	fs.set("reconnecting")
	time.Sleep(20 * time.Millisecond)
	fs.set("connected")
	time.Sleep(150 * time.Millisecond)
	if n := len(onlineEvents(r)); n != 1 {
		t.Fatalf("a pause or a blip was announced: %d events", n)
	}

	// Away for longer than minOffline: announced as reconnected.
	fs.set("reconnecting")
	time.Sleep(200 * time.Millisecond)
	fs.set("connected")
	got = waitOnline(t, r, 2)
	if o := got[1].Online; o.Reason != "reconnected" || o.OfflineSince == nil {
		t.Fatalf("second event = %+v", o)
	}
	r.mu.Lock()
	for i, ok := range r.sigOK {
		if !ok {
			t.Errorf("delivery %d: bad signature or event header", i)
		}
	}
	r.mu.Unlock()
	if v, _ := st.Setting(context.Background(), lastOnlineSetting); v == "" || v == before.Format(time.RFC3339) {
		t.Errorf("last online moment not kept: %q", v)
	}
}

func TestOnlineOnlyForWebhooksThatWantIt(t *testing.T) {
	shortOnlineTimes(t)
	m, _ := manager(t)
	r := newReceiver(t, func(int32) int { return http.StatusOK })
	url := r.srv.URL
	if _, _, err := m.Create(context.Background(), Settings{URL: &url}); err != nil { // messages only
		t.Fatal(err)
	}
	fs := &fakeSync{state: "connected", changes: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.WatchOnline(ctx, fs.changes, fs.read)
	time.Sleep(200 * time.Millisecond)
	if n := len(r.events()); n != 0 {
		t.Fatalf("a webhook without the online event got %d", n)
	}
}
