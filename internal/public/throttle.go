package public

import (
	"crypto/sha256"
	"sync"
	"time"
)

// throttle slows down wrong passwords (and wrong keys), address by address,
// an IPv6 address by its /64: after a few, each try from that address waits
// longer, up to a ceiling. It never locks the account: another address
// signs in at once. The address every visitor shares, when the proxy does
// not say whom it forwards, waits only seconds, so nobody outside can keep
// the owner out. The table is bounded: the least recent address goes first.
type throttle struct {
	mu  sync.Mutex
	m   *lru[*tries]
	now func() time.Time
}

type tries struct {
	fails int
	until time.Time
	last  time.Time
}

const (
	freeTries     = 3
	maxDelay      = 15 * time.Minute
	maxSharedWait = 5 * time.Second
	forgetAfter   = time.Hour
	throttleSize  = 4096
)

func newThrottle() *throttle { return &throttle{m: newLRU[*tries](throttleSize), now: time.Now} }

// wait is how long addr must still wait before its next try.
func (t *throttle) wait(addr string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.m.get(addrKey(addr))
	if !ok {
		return 0
	}
	return max(0, e.until.Sub(t.now()))
}

// try is a sign-in from addr: how long it must still wait, or zero, and
// then the try is counted as a wrong password until ok says otherwise.
// tryUnless is try, skipped when skip is set.
func (t *throttle) tryUnless(skip bool, addr string, shared bool) time.Duration {
	if skip {
		return 0
	}
	return t.try(addr, shared)
}

func (t *throttle) try(addr string, shared bool) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := addrKey(addr)
	if e, ok := t.m.get(key); ok {
		if wait := e.until.Sub(t.now()); wait > 0 {
			return wait
		}
	}
	t.failLocked(key, shared)
	return 0
}

func (t *throttle) failLocked(key string, shared bool) {
	now := t.now()
	e, ok := t.m.get(key)
	if !ok || now.Sub(e.last) > forgetAfter {
		e = &tries{}
		t.m.put(key, e)
	}
	e.fails++
	e.last = now
	if e.fails >= freeTries {
		e.until = now.Add(delay(e.fails, shared))
	}
}

// undo gives back a try that was counted but never checked (the server
// was busy): being turned away is not a wrong password.
func (t *throttle) undo(addr string, shared bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.m.get(addrKey(addr))
	if !ok || e.fails == 0 {
		return
	}
	e.fails--
	if e.fails >= freeTries {
		e.until = e.last.Add(delay(e.fails, shared))
	} else {
		e.until = time.Time{}
	}
}

// ok forgets addr's wrong passwords, after a right one.
func (t *throttle) ok(addr string) {
	t.mu.Lock()
	t.m.remove(addrKey(addr))
	t.mu.Unlock()
}

func delay(fails int, shared bool) time.Duration {
	ceiling := maxDelay
	if shared {
		ceiling = maxSharedWait
	}
	n := fails - freeTries
	if n > 20 {
		return ceiling
	}
	return min(time.Second<<n, ceiling)
}

// refused remembers, by their hash, the wrong keys and tokens looked up in
// the last minute, so one sent again from an address that waits is refused
// without reading state.db. The table is bounded: the least recent goes
// first.
type refused struct {
	mu sync.Mutex
	m  *lru[time.Time]
}

const (
	refusedSize = 4096
	refusedFor  = time.Minute
)

func newRefused() *refused { return &refused{m: newLRU[time.Time](refusedSize)} }

func refusedKey(bearer string) string {
	sum := sha256.Sum256([]byte(bearer))
	return string(sum[:])
}

// has reports whether bearer was refused within the last minute.
func (r *refused) has(bearer string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	at, ok := r.m.get(refusedKey(bearer))
	return ok && now.Sub(at) < refusedFor
}

func (r *refused) add(bearer string, now time.Time) {
	r.mu.Lock()
	r.m.put(refusedKey(bearer), now)
	r.mu.Unlock()
}
