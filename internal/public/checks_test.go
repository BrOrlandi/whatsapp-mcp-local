package public

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

// floodServer is a server with its state, whose password checks run on a
// stopped clock with no waiting, and whose bucket is already empty from a
// flood of wrong passwords from fresh addresses.
func floodServer(t *testing.T, st *state.State, passwordFile string) *Server {
	t.Helper()
	s, _ := testServer(t, origin, "")
	// The same password file as the server before it.
	s.cfg.Password = NewPassword("", passwordFile)
	s.UseState(st)
	return s
}

func drain(s *Server, now time.Time) {
	s.checks.now = func() time.Time { return now }
	s.checks.maxWait = 0
	s.checks.logger = slog.New(slog.DiscardHandler)
	for i := 1; i <= checksBurst+5; i++ {
		signIn(s, "127.0.0.1:40000", "wrong password!", from(fmt.Sprintf("198.51.100.%d", i)))
	}
}

// The owner's browser, known by its cookie, signs in during a flood from an
// address it never used, after a restart of the server (the reserve is in
// state.db, not in memory); a browser without it, or with a made-up one,
// waits like everyone else.
func TestAKnownBrowserSignsInDuringAFloodFromAnyAddress(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	first, passwordFile := testServer(t, origin, "")
	first.UseState(st)
	rec := signIn(first, "127.0.0.1:40000", testPassword, from("203.0.113.50"))
	known := browserCookie(t, rec)
	var c *http.Cookie
	for _, k := range rec.Result().Cookies() {
		if k.Name == BrowserCookie {
			c = k
		}
	}
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.MaxAge < 300*24*3600 || c.Path != "/" {
		t.Fatalf("the known-browser cookie: %+v", c)
	}
	// Only its hash is kept.
	if list, _ := st.KnownBrowsers(context.Background()); len(list) != 1 || list[0].Hash != hashSecret(c.Value) || strings.Contains(list[0].Hash, c.Value) {
		t.Fatalf("what state.db keeps: %+v", list)
	}

	// A restart: a new server on the same state, its memory empty.
	s := floodServer(t, st, passwordFile)
	drain(s, time.Now())
	for _, h := range []map[string]string{from("192.0.2.77"), {"X-Forwarded-For": "192.0.2.78", "Cookie": BrowserCookie + "=wamcp_br_made-up"}} {
		if rec := signIn(s, "127.0.0.1:40000", testPassword, h); rec.Code != http.StatusTooManyRequests {
			t.Fatalf("a browser that is not known, during the flood: %d", rec.Code)
		}
	}
	if rec := signIn(s, "127.0.0.1:40000", testPassword, map[string]string{"X-Forwarded-For": "192.0.2.79", "Cookie": known}); rec.Code != http.StatusSeeOther {
		t.Fatalf("the known browser, from a new address, during the flood: %d", rec.Code)
	}
}

// A new password ends every known browser; the count is bounded.
func TestKnownBrowsersEndWithThePasswordAndAreBounded(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s, path := testServer(t, origin, "")
	s.UseState(st)
	s.checks.capacity, s.checks.tokens = 100, 100 // no flood here
	known := browserCookie(t, signIn(s, "127.0.0.1:40000", testPassword, from("203.0.113.50")))
	for i := 0; i < maxKnownBrowsers+5; i++ {
		signIn(s, "127.0.0.1:40000", testPassword, from(fmt.Sprintf("203.0.113.%d", 100+i)))
	}
	if list, _ := st.KnownBrowsers(context.Background()); len(list) != maxKnownBrowsers {
		t.Fatalf("%d known browsers kept, want %d", len(list), maxKnownBrowsers)
	}
	if len(s.browsers.known) != maxKnownBrowsers {
		t.Fatalf("%d known browsers in memory", len(s.browsers.known))
	}
	// The newest, then a new password.
	known = browserCookie(t, signIn(s, "127.0.0.1:40000", testPassword, from("203.0.113.50")))
	const newPassword = "another long password for the panel"
	if err := os.WriteFile(path, []byte(hashOf(t, newPassword, MinCost)), 0o600); err != nil {
		t.Fatal(err)
	}
	hash, _ := s.cfg.Password.Current()
	r := request("POST", "/entrar", "127.0.0.1:40000", map[string]string{"Cookie": known}, "")
	if s.knownBrowser(r, hash) != "" {
		t.Fatal("a browser known with the old password is still known")
	}
	if list, _ := st.KnownBrowsers(context.Background()); len(list) != 0 {
		t.Fatalf("%d browsers of the old password left in state.db", len(list))
	}
}

// The flood the second review simulated, about 10 sign-in tries a second
// from fresh addresses, each waiting its few seconds: the owner with a known
// browser gets in at once, again and again; the owner without one gets
// turns in proportion to how often it tries, never none.
func TestFloodTurnsWithAndWithoutAKnownBrowser(t *testing.T) {
	type result struct {
		first time.Duration
		turns int
	}
	run := func(seed uint64, browser string, span time.Duration) result {
		c := newChecks(slog.New(slog.DiscardHandler))
		start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		now := start
		c.now = func() time.Time { return now }
		c.draw = rand.New(rand.NewPCG(seed, 1)).IntN
		type waiter struct {
			t     *turn
			until time.Time
		}
		var attackers []waiter
		var owner *waiter
		res := result{first: -1}
		// The flood runs a minute before the owner comes: the bucket is
		// empty by then.
		const warmUp = time.Minute
		for step := time.Duration(0); step < warmUp+span; step += 100 * time.Millisecond {
			now = start.Add(step)
			attackers = append(attackers, waiter{c.join(""), now.Add(c.maxWait)})
			if step < warmUp {
				goto poll
			}
			if owner == nil {
				owner = &waiter{c.join(browser), now.Add(c.maxWait)}
			}
		poll:
			kept := attackers[:0]
			for _, a := range attackers {
				if !c.poll(a.t) && now.Before(a.until) {
					kept = append(kept, a)
				} else if !a.t.granted {
					c.giveUp(a.t)
				}
			}
			attackers = kept
			if owner == nil {
				continue
			}
			if c.poll(owner.t) {
				if res.first < 0 {
					res.first = step - warmUp
				}
				res.turns++
				owner = nil // signed in; the next try starts the next step
			} else if !now.Before(owner.until) {
				c.giveUp(owner.t)
				owner = nil // told busy; tries again at once
			}
		}
		return res
	}
	with := run(1, hashSecret("a known browser"), 240*time.Second)
	if with.first < 0 || with.first > time.Second || with.turns < 20 {
		t.Fatalf("with a known browser: first turn after %v, %d turns in 240 s", with.first, with.turns)
	}
	total, starved := 0, 0
	var firsts []time.Duration
	for seed := uint64(1); seed <= 20; seed++ {
		r := run(seed, "", time.Hour)
		total += r.turns
		if r.turns == 0 {
			starved++
		}
		firsts = append(firsts, r.first)
	}
	t.Logf("with a known browser: first turn after %v, %d turns in 240 s", with.first, with.turns)
	t.Logf("without one, 20 runs of an hour: %d turns in all (%.1f an hour), firsts %v", total, float64(total)/20, firsts)
	if starved > 0 || total/20 < 8 {
		t.Fatalf("without a known browser: %d runs with no turn in an hour, %d turns in all", starved, total)
	}
}

// A turn whose check never ran (no bcrypt slot came in time) is given back,
// and a check that ends in a panic still frees its slot.
func TestATurnWithNoCheckIsGivenBack(t *testing.T) {
	c := newChecks(slog.New(slog.DiscardHandler))
	now := time.Now()
	c.now = func() time.Time { return now }
	c.maxWait = 20 * time.Millisecond
	for range concurrent {
		c.running <- struct{}{}
	}
	before := c.tokens
	if _, ok := c.acquire(context.Background(), ""); ok {
		t.Fatal("a check ran with every slot taken")
	}
	if c.tokens != before {
		t.Fatalf("the turn was spent with no check: %v tokens, %v before", c.tokens, before)
	}
	for range concurrent {
		<-c.running
	}

	s, _ := testServer(t, origin, "")
	old := verifyPassword
	verifyPassword = func(string, string) bool { panic("bcrypt failed") }
	t.Cleanup(func() { verifyPassword = old })
	for range concurrent + 1 {
		func() {
			defer func() { _ = recover() }()
			signIn(s, "127.0.0.1:40000", testPassword, from("203.0.113.9"))
		}()
	}
	if n := len(s.checks.running); n != 0 {
		t.Fatalf("%d slots still held after checks that panicked", n)
	}
}

// Behind a proxy that forwards no visitor address, everyone shares one: a
// flood of wrong passwords puts that address in a long wait and empties the
// bucket, and the owner's known browser still signs in.
func TestAKnownBrowserSignsInWhenTheProxyForwardsNoAddress(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s, _ := testServer(t, origin, "")
	s.UseState(st)
	known := browserCookie(t, signIn(s, "127.0.0.1:40000", testPassword, nil))
	now := time.Now()
	s.checks.now = func() time.Time { return now }
	s.throttle.now = func() time.Time { return now }
	s.checks.maxWait = 0
	s.checks.logger = slog.New(slog.DiscardHandler)
	for range checksBurst + 5 {
		signIn(s, "127.0.0.1:40000", "wrong password!", nil)
	}
	if rec := signIn(s, "127.0.0.1:40000", testPassword, nil); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("someone without the cookie, during the flood: %d", rec.Code)
	}
	if rec := signIn(s, "127.0.0.1:40000", testPassword, map[string]string{"Cookie": known}); rec.Code != http.StatusSeeOther {
		t.Fatalf("the known browser, with no forwarded address, during the flood: %d", rec.Code)
	}
}
