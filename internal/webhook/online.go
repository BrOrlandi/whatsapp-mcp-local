package webhook

import (
	"context"
	"time"
)

// The online event: WhatsApp is reachable again on this computer, so a script
// can run what needs the chats (a summary of what came in overnight, the
// replies left waiting). It fires when the app starts, when the computer wakes
// from sleep, and when the connection comes back after minOffline away; not on
// the pauses sync takes for the tools, nor on a blip of a few seconds.

// lastOnlineSetting keeps, across restarts, the last moment the number was
// connected: on the next start it is when the app went away.
const lastOnlineSetting = "webhook.last_online"

var (
	// heartbeat is how often the last connected moment is written down, and
	// the clock that notices a sleep: a beat arriving much later than due
	// means the computer was asleep in between.
	heartbeat = 30 * time.Second
	// settle is how long the connection must hold, with no history arriving,
	// before the event goes; settleMax caps the wait for the history.
	settle    = 20 * time.Second
	settleMax = 3 * time.Minute
	// minOffline is the shortest time away a reconnection is announced for.
	minOffline = 2 * time.Minute
)

// wallNow is the time on the wall clock. The watcher compares only wall
// times: on macOS the monotonic clock stops while the computer sleeps, so a
// night asleep would look like no time at all.
var wallNow = func() time.Time { return time.Now().Round(0) }

// wakeWait is how long, after a sleep, the event waits for sync to connect
// anew: the connection sync reports may be a socket the sleep killed. If sync
// never reconnects (the socket survived), the event goes after it.
var wakeWait = 90 * time.Second

// SyncState is what the online watcher reads about sync: its state
// (connected, reconnecting, paused…), since when, and when history last
// arrived.
type SyncState func() (state string, since, historyAt time.Time)

// WatchOnline follows sync and dispatches an online event every time the
// number is reachable again. changes signals each change of sync's state. It
// returns when ctx ends.
func (m *Manager) WatchOnline(ctx context.Context, changes <-chan struct{}, read SyncState) {
	w := &onlineWatch{m: m, read: read, reason: "started"}
	if v, err := m.st.Setting(ctx, lastOnlineSetting); err == nil && v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			w.lastSeen = t
		}
	}
	w.offlineSince = w.lastSeen
	beat := time.NewTicker(heartbeat)
	defer beat.Stop()
	w.check(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-changes:
			w.check(ctx)
		case <-w.timer():
			w.fire(ctx)
		case <-beat.C:
			if w.slept() {
				w.check(ctx)
			} else if w.online {
				w.seen(ctx, wallNow())
			}
		}
	}
}

type onlineWatch struct {
	m    *Manager
	read SyncState

	online       bool
	lastSeen     time.Time // the last moment known connected, on the wall clock
	offlineSince time.Time // when it went away, zero when unknown
	reason       string    // why the next event goes: started, woke, reconnected
	wokeAt       time.Time // when a sleep was noticed, for the woke event

	waitFrom time.Time // when the connection was first seen holding
	pending  *time.Timer
}

func (w *onlineWatch) timer() <-chan time.Time {
	if w.pending == nil {
		return nil
	}
	return w.pending.C
}

func (w *onlineWatch) schedule(d time.Duration) {
	if w.pending != nil {
		w.pending.Stop()
	}
	w.pending = time.NewTimer(d)
}

func (w *onlineWatch) cancel() {
	if w.pending != nil {
		w.pending.Stop()
		w.pending = nil
	}
	w.waitFrom = time.Time{}
}

// goneSince marks the number away from at (the last moment it was connected).
func (w *onlineWatch) goneSince(at time.Time, reason string) {
	w.online = false
	w.offlineSince = at
	w.reason = reason
	w.cancel()
}

func (w *onlineWatch) seen(ctx context.Context, at time.Time) {
	w.lastSeen = at
	_ = w.m.st.SetSetting(context.WithoutCancel(ctx), lastOnlineSetting, at.UTC().Format(time.RFC3339))
}

// slept notices a sleep: while online the heartbeat writes lastSeen down
// every beat, so a gap of several beats on the wall clock means the computer
// was asleep. It runs before anything else looks at the state, so the time
// away is measured from before the sleep, not from when sync noticed it.
func (w *onlineWatch) slept() bool {
	now := wallNow()
	if !w.online || now.Sub(w.lastSeen) <= 3*heartbeat {
		return false
	}
	w.goneSince(w.lastSeen, "woke")
	w.wokeAt = now
	return true
}

// check reacts to sync's state now.
func (w *onlineWatch) check(ctx context.Context) {
	w.slept()
	state, _, _ := w.read()
	switch state {
	case "paused", "starting":
		// The tools pause sync for seconds at a time, and it starts again
		// after; nothing changes until it connects or fails.
	case "connected":
		if !w.online && w.pending == nil {
			w.waitFrom = wallNow()
			w.schedule(settle)
		}
	default:
		if w.online {
			w.seen(ctx, wallNow())
			w.goneSince(w.lastSeen, "reconnected")
		} else if w.pending != nil {
			// It dropped again while the history settled. After a short
			// outage the connection held until now, so that time is not away:
			// the count starts again from here.
			if w.reason == "reconnected" && w.waitFrom.Sub(w.offlineSince) < minOffline {
				w.offlineSince = wallNow()
			}
			w.cancel()
		}
	}
}

// fire sends the event if the connection held and the history settled.
func (w *onlineWatch) fire(ctx context.Context) {
	w.pending = nil
	state, since, historyAt := w.read()
	if state != "connected" {
		w.waitFrom = time.Time{}
		return
	}
	now := wallNow()
	waited := now.Sub(w.waitFrom)
	// Not after, rather than before: on a coarse clock (Windows) a connection
	// noticed in the same tick as the wake reads as the same instant.
	if w.reason == "woke" && !since.After(w.wokeAt) && now.Sub(w.wokeAt) < wakeWait {
		w.schedule(settle) // sync has not connected since the wake yet
		return
	}
	if quiet := now.Sub(historyAt); quiet < settle && waited < settleMax {
		w.schedule(settle - quiet)
		return
	}
	// Away is up to when the connection came back, not to now: the wait for
	// the history is not time away.
	away := time.Duration(0)
	if !w.offlineSince.IsZero() {
		away = w.waitFrom.Sub(w.offlineSince)
	}
	w.waitFrom = time.Time{}
	w.online = true
	announce := w.reason != "reconnected" || away >= minOffline
	ev := Event{Kind: "online", ID: randomHex(8), At: now.UTC(), Online: &Online{Reason: w.reason}}
	if !w.offlineSince.IsZero() {
		since := w.offlineSince.UTC().Truncate(time.Second)
		ev.Online.OfflineSince, ev.Online.OfflineSeconds = &since, int64(away.Seconds())
	}
	w.seen(ctx, now)
	if announce {
		w.m.logger.Info("whatsapp online", "reason", w.reason, "offline_seconds", ev.Online.OfflineSeconds)
		w.m.Dispatch(ctx, ev)
	}
}
