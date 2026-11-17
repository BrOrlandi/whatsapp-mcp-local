package mcp

import (
	"context"
	"sync"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
)

// Three reading tools ask WhatsApp's servers, as the owner, about numbers
// and groups the caller names: check_numbers, get_profile_picture, and
// get_group with live. For an agent on another computer they share a
// bucket per connection, so a key or a login cannot make the account look
// up numbers in bulk. Programs on this computer are not limited.
const (
	// liveBurst lookups at once (each number of check_numbers is one),
	// then one more every liveEvery: 60 an hour.
	liveBurst = 20
	liveEvery = time.Minute
)

type liveLimits struct {
	mu      sync.Mutex
	buckets map[string]*liveBucket
	now     func() time.Time
}

type liveBucket struct {
	tokens float64
	last   time.Time
}

func newLiveLimits() *liveLimits {
	return &liveLimits{buckets: map[string]*liveBucket{}, now: time.Now}
}

// allow spends cost lookups of the caller's connection, and says how long
// to wait when there are not enough.
func (l *liveLimits) allow(ctx context.Context, cost int) (bool, time.Duration) {
	c := caller.From(ctx)
	if c.Kind == caller.Local {
		return true, 0
	}
	key := string(c.Kind) + ":" + c.ConnectionID
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b := l.buckets[key]
	if b == nil {
		if len(l.buckets) >= 1000 {
			// Connections are only made with a credential; forget the
			// idlest when there are many.
			var idlest string
			for k, v := range l.buckets {
				if idlest == "" || v.last.Before(l.buckets[idlest].last) {
					idlest = k
				}
			}
			delete(l.buckets, idlest)
		}
		b = &liveBucket{tokens: liveBurst, last: now}
		l.buckets[key] = b
	}
	b.tokens = min(liveBurst, b.tokens+now.Sub(b.last).Seconds()/liveEvery.Seconds())
	b.last = now
	if float64(cost) > b.tokens {
		need := (float64(cost) - b.tokens) * liveEvery.Seconds()
		return false, time.Duration(need) * time.Second
	}
	b.tokens -= float64(cost)
	return true, 0
}

// liveAllowed is nil when the caller may make cost live lookups now, and
// the refusal otherwise.
func (s *Server) liveAllowed(ctx context.Context, cost int) map[string]any {
	ok, wait := s.live.allow(ctx, cost)
	if ok {
		return nil
	}
	return toolError("this connection asked WhatsApp about too many numbers or groups lately (%d at once, then one a minute); try again in %d seconds",
		liveBurst, int(wait.Round(time.Second)/time.Second)+1)
}
