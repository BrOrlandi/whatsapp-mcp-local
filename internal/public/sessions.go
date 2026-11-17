package public

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"time"
)

const (
	sessionMaxAge = 30 * 24 * time.Hour
	sessionIdle   = 7 * 24 * time.Hour
)

// sessions are the panel's sign-ins, kept in memory: a restart signs
// everyone out. Only the SHA-256 of each session id is kept, and each one
// records the password it was made with, so a new password ends them all.
type sessions struct {
	mu  sync.Mutex
	m   map[[32]byte]*session
	now func() time.Time
}

type session struct {
	created  time.Time
	seen     time.Time
	password [32]byte
}

func newSessions() *sessions { return &sessions{m: map[[32]byte]*session{}, now: time.Now} }

// create starts a session for the password whose fingerprint is given, and
// returns its id, the cookie's value.
func (s *sessions) create(password [32]byte) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	id := base64.RawURLEncoding.EncodeToString(b)
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune(now)
	s.m[sha256.Sum256([]byte(id))] = &session{created: now, seen: now, password: password}
	return id
}

// valid reports whether id is a live session made with the password whose
// fingerprint is given, and keeps it alive.
func (s *sessions) valid(id string, password [32]byte) bool {
	if id == "" {
		return false
	}
	key := sha256.Sum256([]byte(id))
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.m[key]
	if e == nil {
		return false
	}
	if e.password != password || now.Sub(e.created) > sessionMaxAge || now.Sub(e.seen) > sessionIdle {
		delete(s.m, key)
		return false
	}
	e.seen = now
	return true
}

// end signs a session out.
func (s *sessions) end(id string) {
	s.mu.Lock()
	delete(s.m, sha256.Sum256([]byte(id)))
	s.mu.Unlock()
}

func (s *sessions) prune(now time.Time) {
	for k, e := range s.m {
		if now.Sub(e.created) > sessionMaxAge || now.Sub(e.seen) > sessionIdle {
			delete(s.m, k)
		}
	}
}
