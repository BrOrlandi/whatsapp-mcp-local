package webhook

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeSync is sync's state as the watcher reads it, changed by the test.
type fakeSync struct {
	mu        sync.Mutex
	state     string
	since     time.Time
	historyAt time.Time
	changes   chan struct{}
}

func (f *fakeSync) set(state string) {
	f.mu.Lock()
	f.state, f.since = state, wallNow()
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

func (f *fakeSync) read() (string, time.Time, time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state, f.since, f.historyAt
}

// skew moves the watcher's wall clock forward, as a night asleep does while
// the monotonic clock stands still.
var skew atomic.Int64

func init() {
	// Short times for the watcher, set once: watchers of earlier tests may
	// still be winding down, so they are never put back.
	heartbeat, settle, settleMax, minOffline, wakeWait = time.Hour, 40*time.Millisecond, 400*time.Millisecond, 150*time.Millisecond, 300*time.Millisecond
	wallNow = func() time.Time { return time.Now().Round(0).Add(time.Duration(skew.Load())) }
}

func shortOnlineTimes(t *testing.T) {
	t.Helper()
	skew.Store(0)
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

// onlineSetup is a webhook that wants the online event, watching a fake sync
// that is already online once.
func onlineSetup(t *testing.T) (*receiver, *fakeSync) {
	t.Helper()
	shortOnlineTimes(t)
	m, _ := manager(t)
	r := newReceiver(t, func(int32) int { return http.StatusOK })
	online, url := []string{"online"}, r.srv.URL
	if _, _, err := m.Create(context.Background(), Settings{URL: &url, Events: &online}); err != nil {
		t.Fatal(err)
	}
	fs := &fakeSync{state: "starting", changes: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go m.WatchOnline(ctx, fs.changes, fs.read)
	fs.set("connected")
	waitOnline(t, r, 1)
	return r, fs
}

func TestOnlineAfterSleep(t *testing.T) {
	r, fs := onlineSetup(t)
	// Eight hours asleep: the monotonic clock did not move, the wall did.
	// Sync then notices the dead socket and reconnects in a moment.
	skew.Store(int64(8 * time.Hour))
	fs.set("reconnecting")
	fs.set("connected")
	got := waitOnline(t, r, 2)
	o := got[1].Online
	if o.Reason != "woke" || o.OfflineSeconds < int64((8*time.Hour).Seconds()) {
		t.Fatalf("after sleep = %+v", o)
	}
}

func TestOnlineAfterSleepWaitsForTheReconnection(t *testing.T) {
	r, fs := onlineSetup(t)
	skew.Store(int64(8 * time.Hour))
	fs.set("connected") // a change that is not a new connection yet: still the old socket
	time.Sleep(120 * time.Millisecond)
	if n := len(onlineEvents(r)); n != 1 {
		t.Fatalf("woke announced before sync reconnected: %d events", n)
	}
	fs.set("reconnecting")
	fs.set("connected")
	if got := waitOnline(t, r, 2); got[1].Online.Reason != "woke" {
		t.Fatalf("reason = %q", got[1].Online.Reason)
	}
}

func TestOnlineToolPauseDuringHistory(t *testing.T) {
	r, fs := onlineSetup(t)
	// History keeps arriving, and a tool pauses and restarts sync meanwhile:
	// the wait for the history is not time away.
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(5 * time.Millisecond):
				fs.history(wallNow())
			}
		}
	}()
	fs.set("paused")
	fs.set("starting")
	fs.set("connected")
	time.Sleep(600 * time.Millisecond) // past settleMax and minOffline
	close(stop)
	if n := len(onlineEvents(r)); n != 1 {
		t.Fatalf("a tool pause was announced: %d events", n)
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

// step changes sync's state and gives the watcher a moment to see it: the
// change channel keeps one signal, so two changes in a row would merge.
func step(fs *fakeSync, state string) {
	fs.set(state)
	time.Sleep(10 * time.Millisecond)
}

func TestOnlineOfflineSecondsStopAtTheReconnection(t *testing.T) {
	r, fs := onlineSetup(t)
	step(fs, "reconnecting")
	skew.Add(int64(5 * time.Minute)) // five minutes away
	fs.history(wallNow())
	step(fs, "connected")
	skew.Add(int64(time.Minute)) // then a minute of history settling
	got := waitOnline(t, r, 2)
	if o := got[1].Online; o.Reason != "reconnected" || o.OfflineSeconds != 300 {
		t.Fatalf("offline_seconds = %d (%+v), want 300: the wait for the history is not time away", o.OfflineSeconds, o)
	}
}

func TestOnlineDropWhileHistorySettles(t *testing.T) {
	r, fs := onlineSetup(t)
	// A short outage, then the connection holds while history arrives, then
	// another short outage: the time it held is not away.
	step(fs, "reconnecting")
	skew.Add(int64(50 * time.Millisecond))
	fs.history(wallNow())
	step(fs, "connected")
	skew.Add(int64(125 * time.Millisecond))
	fs.history(wallNow())
	step(fs, "reconnecting")
	skew.Add(int64(5 * time.Millisecond))
	step(fs, "connected")
	time.Sleep(200 * time.Millisecond)
	if got := onlineEvents(r); len(got) != 1 {
		t.Fatalf("two short outages were announced as one long one: %+v", got[len(got)-1].Online)
	}
}
