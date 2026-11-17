package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// mediaLinks hands out short-lived URLs for downloaded files. The token (192
// random bits) is the only thing that grants access, and it expires in
// minutes. On the server mode's public address it is the only credential the
// link needs.
type mediaLinks struct {
	mu    sync.Mutex
	links map[string]mediaLink
}

type mediaLink struct {
	path    string
	mime    string
	expires time.Time
}

func newMediaLinks() *mediaLinks { return &mediaLinks{links: map[string]mediaLink{}} }

func (l *mediaLinks) add(path, mime string, ttl time.Duration) string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for k, v := range l.links {
		if now.After(v.expires) {
			delete(l.links, k)
		}
	}
	l.links[token] = mediaLink{path: path, mime: mime, expires: now.Add(ttl)}
	return token
}

// Add hands out a link to a file for ttl, and returns its token: the path
// is /media/<token>.
func (l *mediaLinks) Add(path, mime string, ttl time.Duration) string { return l.add(path, mime, ttl) }

// Live reports whether a /media/ path is a link that still works.
func (l *mediaLinks) Live(path string) bool {
	token, ok := strings.CutPrefix(path, "/media/")
	if !ok || token == "" {
		return false
	}
	l.mu.Lock()
	link, ok := l.links[token]
	l.mu.Unlock()
	return ok && time.Now().Before(link.expires)
}

// ServeHTTP answers GET /media/{token}.
func (l *mediaLinks) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/media/")
	l.mu.Lock()
	link, ok := l.links[token]
	l.mu.Unlock()
	if !ok || time.Now().After(link.expires) {
		http.Error(w, "this link has expired; call download_media again", http.StatusNotFound)
		return
	}
	if link.mime != "" {
		w.Header().Set("Content-Type", link.mime)
	}
	// The type is the one WhatsApp gave; a browser must not guess another.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(link.path)+`"`)
	http.ServeFile(w, r, link.path)
}
