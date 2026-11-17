package public

import (
	"container/list"
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
