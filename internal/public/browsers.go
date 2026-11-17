package public

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

// A known browser is one the right password was typed in: it keeps a
// random id in a long-lived cookie, and the server keeps only its hash (in
// state.db, so it survives restarts and image updates), with the
// fingerprint of the password it was typed with. During a flood of password
// tries a known browser draws on a reserve of its own, from any address; a
// password change ends every one. A missing or unknown cookie costs a hash
// and a map lookup, nothing more.

// BrowserCookie is the known browser's cookie.
const BrowserCookie = "whatsapp_mcp_browser"

const (
	maxKnownBrowsers = 20
	browserMaxAge    = 365 * 24 * time.Hour
)

type knownBrowser struct {
	password string
	lastUsed time.Time
}

type browsers struct {
	mu     sync.Mutex
	loaded bool
	known  map[string]knownBrowser
}

// loadBrowsers reads the remembered browsers once. The caller holds the lock.
func (s *Server) loadBrowsers() {
	if s.browsers.loaded {
		return
	}
	s.browsers.loaded = true
	s.browsers.known = map[string]knownBrowser{}
	if s.state == nil {
		return
	}
	list, err := s.state.KnownBrowsers(context.Background())
	if err != nil {
		s.logger.Warn("public listener: could not read the known browsers", "error", err)
		return
	}
	for _, b := range list {
		s.browsers.known[b.Hash] = knownBrowser{password: b.Password, lastUsed: b.LastUsed}
	}
}

// knownBrowser is the hash of the request's known-browser cookie when it is
// one remembered with the password in force, or "".
func (s *Server) knownBrowser(r *http.Request, passwordHash string) string {
	c, err := r.Cookie(BrowserCookie)
	if err != nil || len(c.Value) == 0 || len(c.Value) > 100 {
		return ""
	}
	h := hashSecret(c.Value)
	s.browsers.mu.Lock()
	defer s.browsers.mu.Unlock()
	s.loadBrowsers()
	b, ok := s.browsers.known[h]
	if !ok {
		return ""
	}
	if fp := passwordFingerprint(passwordHash); b.password != fp {
		// The password changed: every browser remembered before is
		// forgotten.
		for k, v := range s.browsers.known {
			if v.password != fp {
				delete(s.browsers.known, k)
			}
		}
		if s.state != nil {
			_ = s.state.ForgetBrowsers(context.Background(), fp)
		}
		return ""
	}
	return h
}

// rememberBrowser marks the request's browser as known, after the right
// password: the same id when it had one, a new one otherwise.
func (s *Server) rememberBrowser(w http.ResponseWriter, r *http.Request, passwordHash string) {
	id := ""
	if c, err := r.Cookie(BrowserCookie); err == nil && s.knownBrowser(r, passwordHash) != "" {
		id = c.Value
	} else {
		id = secret("wamcp_br_")
	}
	h, fp, now := hashSecret(id), passwordFingerprint(passwordHash), s.now()
	s.browsers.mu.Lock()
	s.loadBrowsers()
	s.browsers.known[h] = knownBrowser{password: fp, lastUsed: now}
	for len(s.browsers.known) > maxKnownBrowsers {
		oldest := ""
		for k, v := range s.browsers.known {
			if oldest == "" || v.lastUsed.Before(s.browsers.known[oldest].lastUsed) {
				oldest = k
			}
		}
		delete(s.browsers.known, oldest)
	}
	s.browsers.mu.Unlock()
	if s.state != nil {
		if err := s.state.RememberBrowser(r.Context(), state.KnownBrowser{Hash: h, Password: fp, LastUsed: now}, maxKnownBrowsers); err != nil {
			s.logger.Warn("public listener: could not remember the browser", "error", err)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: BrowserCookie, Value: id, Path: "/", HttpOnly: true, Secure: s.cfg.HTTPS(),
		SameSite: http.SameSiteLaxMode, MaxAge: int(browserMaxAge / time.Second)})
}
