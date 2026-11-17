package public

import (
	"sync"
	"time"
)

// throttle slows down wrong passwords, address by address: after a few, each
// try from that address waits longer, up to a ceiling. It never locks the
// account: another address signs in at once. The address every visitor
// shares, when the proxy does not say whom it forwards, waits only seconds,
// so nobody outside can keep the owner out.
type throttle struct {
	mu  sync.Mutex
	m   map[string]*tries
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
)

func newThrottle() *throttle { return &throttle{m: map[string]*tries{}, now: time.Now} }

// wait is how long addr must still wait before its next try.
func (t *throttle) wait(addr string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.m[addr]
	if e == nil {
		return 0
	}
	return max(0, e.until.Sub(t.now()))
}

// fail counts a wrong password from addr.
func (t *throttle) fail(addr string, shared bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	e := t.m[addr]
	if e == nil || now.Sub(e.last) > forgetAfter {
		e = &tries{}
		t.m[addr] = e
	}
	e.fails++
	e.last = now
	if e.fails >= freeTries {
		e.until = now.Add(delay(e.fails, shared))
	}
	if len(t.m) > 10000 {
		for k, v := range t.m {
			if now.Sub(v.last) > forgetAfter {
				delete(t.m, k)
			}
		}
	}
}

// ok forgets addr's wrong passwords, after a right one.
func (t *throttle) ok(addr string) {
	t.mu.Lock()
	delete(t.m, addr)
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
