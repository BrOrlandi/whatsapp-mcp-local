package public

import (
	"container/list"
	"context"
	"log/slog"
	"sync"
	"time"
)

// Everything a request without a credential can make the server remember
// is bounded: least-recently-used tables that drop their oldest entry when
// full, never a map that grows without end or is wiped (resetting
// everyone's limits at once).

// addrKey is the key an address is limited by: an IPv6 address by its /64,
// which one machine usually holds whole, an IPv4 address as it is.
func addrKey(addr string) string {
	a, ok := parseAddr(addr)
	if !ok {
		return addr
	}
	if a.Is6() && !a.Is4In6() {
		p, _ := a.Prefix(64)
		return p.String()
	}
	return a.String()
}

// lru is a bounded table: Put on a full table drops the least recently used
// entry. Callers hold their own lock.
type lru[V any] struct {
	max   int
	order *list.List // front: most recent
	items map[string]*list.Element
}

type lruEntry[V any] struct {
	key   string
	value V
}

func newLRU[V any](max int) *lru[V] {
	return &lru[V]{max: max, order: list.New(), items: map[string]*list.Element{}}
}

func (l *lru[V]) get(key string) (V, bool) {
	if e, ok := l.items[key]; ok {
		l.order.MoveToFront(e)
		return e.Value.(*lruEntry[V]).value, true
	}
	var zero V
	return zero, false
}

func (l *lru[V]) put(key string, v V) {
	if e, ok := l.items[key]; ok {
		e.Value.(*lruEntry[V]).value = v
		l.order.MoveToFront(e)
		return
	}
	if l.order.Len() >= l.max {
		oldest := l.order.Back()
		l.order.Remove(oldest)
		delete(l.items, oldest.Value.(*lruEntry[V]).key)
	}
	l.items[key] = l.order.PushFront(&lruEntry[V]{key: key, value: v})
}

func (l *lru[V]) remove(key string) {
	if e, ok := l.items[key]; ok {
		l.order.Remove(e)
		delete(l.items, key)
	}
}

func (l *lru[V]) len() int { return l.order.Len() }

// window counts events per address in the last hour, to cap them.
type window struct {
	mu   sync.Mutex
	hits *lru[[]time.Time]
	now  func() time.Time
}

func newWindow(size int) *window { return &window{hits: newLRU[[]time.Time](size), now: time.Now} }

// allow records an event for addr when fewer than limit happened in the
// last hour, and reports whether it did.
func (w *window) allow(addr string, limit int) bool {
	key := addrKey(addr)
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	old, _ := w.hits.get(key)
	recent := make([]time.Time, 0, len(old)+1)
	for _, t := range old {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	if len(recent) >= limit {
		w.hits.put(key, recent)
		return false
	}
	w.hits.put(key, append(recent, now))
	return true
}

// checks guards the password checks themselves, whoever sends them: a
// token bucket for all of them together, and at most a couple of bcrypt
// checks at once. An address the right password came from before has a
// small bucket of its own, so a flood from elsewhere keeps the owner
// waiting seconds, not minutes.
type checks struct {
	mu       sync.Mutex
	tokens   float64
	last     time.Time
	capacity float64
	perSec   float64
	// owners are the addresses that signed in with the right password in
	// the last month, each with its own reserve.
	owners  *lru[*reserve]
	running chan struct{}
	// maxWait is how long a check waits for its turn before the answer is
	// "try again in a few seconds".
	maxWait time.Duration
	now     func() time.Time
	sleep   func(time.Duration)
	logger  *slog.Logger
	warned  time.Time
}

type reserve struct {
	tokens float64
	last   time.Time
	since  time.Time
}

const (
	// checksBurst and checksEvery bound the password checks of everyone
	// together: 20 at once, then one every 4 seconds, 21,600 a day.
	checksBurst = 20
	checksEvery = 4 * time.Second
	// The reserve of an address the right password came from: 3 at once,
	// then one every 10 seconds.
	reserveBurst = 3
	reserveEvery = 10 * time.Second
	ownerFor     = 30 * 24 * time.Hour
	concurrent   = 2
)

func newChecks(logger *slog.Logger) *checks {
	return &checks{tokens: checksBurst, capacity: checksBurst, perSec: 1 / checksEvery.Seconds(), owners: newLRU[*reserve](64),
		running: make(chan struct{}, concurrent), maxWait: 5 * time.Second, now: time.Now, sleep: time.Sleep, logger: logger}
}

// take is one token for addr, from the bucket of everyone or from the
// address's own reserve.
func (c *checks) take(addr string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if !c.last.IsZero() {
		c.tokens = min(c.capacity, c.tokens+now.Sub(c.last).Seconds()*c.perSec)
	}
	c.last = now
	if c.tokens >= 1 {
		c.tokens--
		return true
	}
	if now.Sub(c.warned) > time.Minute {
		c.warned = now
		c.logger.Warn("the panel password is being checked more often than allowed: someone may be guessing it; checks wait their turn", "limit_per_day", int(24*time.Hour/checksEvery))
	}
	if r, ok := c.owners.get(addrKey(addr)); ok && now.Sub(r.since) < ownerFor {
		r.tokens = min(reserveBurst, r.tokens+now.Sub(r.last).Seconds()/reserveEvery.Seconds())
		r.last = now
		if r.tokens >= 1 {
			r.tokens--
			return true
		}
	}
	return false
}

// acquire waits up to maxWait for a check's turn, and returns the function
// that ends it; false means busy.
func (c *checks) acquire(ctx context.Context, addr string) (func(), bool) {
	deadline := c.now().Add(c.maxWait)
	for !c.take(addr) {
		if !c.now().Before(deadline) || ctx.Err() != nil {
			return nil, false
		}
		c.sleep(100 * time.Millisecond)
	}
	release := func() { <-c.running }
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
		return nil, false
	case <-timer.C:
		return nil, false
	}
}

// trust records that the right password came from addr.
func (c *checks) trust(addr string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	key := addrKey(addr)
	if r, ok := c.owners.get(key); ok {
		r.since = now
		return
	}
	c.owners.put(key, &reserve{tokens: reserveBurst, last: now, since: now})
}

// logLimiter keeps a repeated warning to one line a minute.
type logLimiter struct {
	mu       sync.Mutex
	last     time.Time
	skipped  int
	interval time.Duration
}

func (l *logLimiter) allow(now time.Time) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.last) < l.interval {
		l.skipped++
		return false, 0
	}
	n := l.skipped
	l.last, l.skipped = now, 0
	return true, n
}
