package public

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"slices"
	"sync"
	"time"
)

// checks guards the password checks themselves, whoever sends them: a token
// bucket for all of them together, and at most a couple of bcrypt checks at
// once.
//
// When the bucket is empty, the checks that arrive wait for a turn, and each
// turn goes to one of those waiting drawn at random, not to whichever asks
// first. A strict queue would starve its tail: with checks giving up after a
// few seconds and a flood arriving faster than turns come, the one at the
// back never reaches the front. A draw gives every waiting check the same
// chance, so an owner without a known browser gets turns in proportion to
// how often it tries, never none.
//
// A browser the right password was typed in before (a known browser, by its
// cookie) has a small reserve of its own besides, so a flood keeps its owner
// waiting seconds, whatever address it comes from.
type checks struct {
	mu       sync.Mutex
	tokens   float64
	last     time.Time
	capacity float64
	perSec   float64
	// waiting are the checks waiting for a turn of everyone's bucket.
	waiting []*turn
	// reserves are the known browsers' own turns, by the hash of the cookie.
	reserves *lru[*reserve]
	running  chan struct{}
	// maxWait is how long a check waits for its turn before the answer is
	// "try again in a few seconds".
	maxWait time.Duration
	now     func() time.Time
	sleep   func(time.Duration)
	// draw picks which of n waiting checks gets the next turn.
	draw   func(n int) int
	logger *slog.Logger
	warned time.Time
}

// turn is one check's place: granted once it has a token, from everyone's
// bucket or from its browser's reserve.
type turn struct {
	browser string
	granted bool
	reserve *reserve
}

type reserve struct {
	tokens float64
	last   time.Time
}

const (
	// checksBurst and checksEvery bound the password checks of everyone
	// together: 20 at once, then one every 4 seconds, 21,600 a day.
	checksBurst = 20
	checksEvery = 4 * time.Second
	// The reserve of a known browser: 3 at once, then one every 10 seconds.
	reserveBurst = 3
	reserveEvery = 10 * time.Second
	concurrent   = 2
)

func newChecks(logger *slog.Logger) *checks {
	return &checks{tokens: checksBurst, capacity: checksBurst, perSec: 1 / checksEvery.Seconds(),
		reserves: newLRU[*reserve](maxKnownBrowsers), running: make(chan struct{}, concurrent), maxWait: 5 * time.Second,
		now: time.Now, sleep: time.Sleep, draw: rand.IntN, logger: logger}
}

// hand refills everyone's bucket and hands its tokens to waiting checks,
// drawn at random. The caller holds the lock.
func (c *checks) hand(now time.Time) {
	if !c.last.IsZero() {
		c.tokens = min(c.capacity, c.tokens+now.Sub(c.last).Seconds()*c.perSec)
	}
	c.last = now
	for c.tokens >= 1 && len(c.waiting) > 0 {
		i := c.draw(len(c.waiting))
		c.waiting[i].granted = true
		c.waiting = slices.Delete(c.waiting, i, i+1)
		c.tokens--
	}
}

// fromReserve takes a token of a known browser's reserve. The caller holds
// the lock.
func (c *checks) fromReserve(t *turn, now time.Time) bool {
	if t.browser == "" {
		return false
	}
	r, ok := c.reserves.get(t.browser)
	if !ok {
		r = &reserve{tokens: reserveBurst, last: now}
		c.reserves.put(t.browser, r)
	}
	r.tokens = min(reserveBurst, r.tokens+now.Sub(r.last).Seconds()/reserveEvery.Seconds())
	r.last = now
	if r.tokens < 1 {
		return false
	}
	r.tokens--
	t.granted, t.reserve = true, r
	return true
}

// join is a check asking for its turn; browser is the hash of a known
// browser's cookie, or "".
func (c *checks) join(browser string) *turn {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	c.hand(now)
	t := &turn{browser: browser}
	if len(c.waiting) == 0 && c.tokens >= 1 {
		c.tokens--
		t.granted = true
		return t
	}
	if now.Sub(c.warned) > time.Minute {
		c.warned = now
		c.logger.Warn("the panel password is being checked more often than allowed: someone may be guessing it; checks wait their turn", "limit_per_day", int(24*time.Hour/checksEvery))
	}
	if c.fromReserve(t, now) {
		return t
	}
	c.waiting = append(c.waiting, t)
	return t
}

// poll reports whether a waiting check has its turn now.
func (c *checks) poll(t *turn) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	c.hand(now)
	if t.granted {
		return true
	}
	if c.fromReserve(t, now) {
		c.waiting = slices.DeleteFunc(c.waiting, func(w *turn) bool { return w == t })
		return true
	}
	return false
}

// giveUp ends a wait; true when the turn came just before.
func (c *checks) giveUp(t *turn) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t.granted {
		return true
	}
	c.waiting = slices.DeleteFunc(c.waiting, func(w *turn) bool { return w == t })
	return false
}

// refund gives back a turn that led to no check.
func (c *checks) refund(t *turn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t.reserve != nil {
		t.reserve.tokens = min(reserveBurst, t.reserve.tokens+1)
		return
	}
	c.tokens = min(c.capacity, c.tokens+1)
	c.hand(c.now())
}

// acquire waits up to maxWait for a check's turn and then for a free bcrypt
// slot, and returns the function that ends the check; false means busy, and
// a turn that led to no check is given back.
func (c *checks) acquire(ctx context.Context, browser string) (func(), bool) {
	t := c.join(browser)
	if !t.granted {
		deadline := c.now().Add(c.maxWait)
		for !c.poll(t) {
			if !c.now().Before(deadline) || ctx.Err() != nil {
				if !c.giveUp(t) {
					return nil, false
				}
				break
			}
			c.sleep(100 * time.Millisecond)
		}
	}
	var once sync.Once
	release := func() { once.Do(func() { <-c.running }) }
	select {
	case c.running <- struct{}{}:
		return release, true
	default:
	}
	timer := time.NewTimer(c.maxWait)
	defer timer.Stop()
	select {
	case c.running <- struct{}{}:
		return release, true
	case <-ctx.Done():
	case <-timer.C:
	}
	c.refund(t)
	return nil, false
}
