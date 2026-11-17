package public

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

// The prefixes of the secrets the server hands out, so a key or a token is
// recognisable where it ends up (a config file, a log a person pastes).
const (
	KeyPrefix     = "wamcp_key_"
	accessPrefix  = "wamcp_at_"
	refreshPrefix = "wamcp_rt_"
	codePrefix    = "wamcp_code_"
	clientPrefix  = "wamcp_client_"
)

const (
	// keyUnusedFor is how long a new key waits for its first use: a key
	// nobody used by then stops working, so one made and forgotten does
	// not stay valid.
	keyUnusedFor = 24 * time.Hour
	// accessTTL is an OAuth access token's life; the client refreshes it.
	accessTTL = time.Hour
	// touchEvery bounds how often a last-used date is written.
	touchEvery = time.Minute
)

// secret is a new random secret: 256 bits, base64url, after its prefix.
func secret(prefix string) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return prefix + base64.RawURLEncoding.EncodeToString(b)
}

// shortID is an id that names a key or a connection in lists: random, and
// saying nothing about any secret.
func shortID(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

// hashSecret is what is stored of a secret.
func hashSecret(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// passwordFingerprint is the fingerprint of a password hash a login is
// approved with: a new password ends every login made before it.
func passwordFingerprint(hash string) string {
	fp := fingerprint(hash)
	return hex.EncodeToString(fp[:])
}

// KeyView is a key as the panel and the command line list it.
type KeyView struct {
	state.APIKey
	// Expired is set for a key nobody used within a day of its creation.
	Expired bool `json:"expired"`
	// ExpiresAt is when an unused key stops working.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// CreateKey makes a key, keeps only its hash, and returns the key, the only
// time it is ever seen.
func CreateKey(ctx context.Context, st *state.State, name string, readOnly bool, now time.Time) (string, state.APIKey, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 60 {
		return "", state.APIKey{}, errors.New(i18n.T("dê um nome à chave, de até 60 caracteres, como o da ferramenta que vai usá-la"))
	}
	key := secret(KeyPrefix)
	k := state.APIKey{ID: shortID("k_"), Name: name, ReadOnly: readOnly, CreatedAt: now.UTC().Truncate(time.Second)}
	if err := st.AddAPIKey(ctx, k, hashSecret(key)); err != nil {
		return "", state.APIKey{}, err
	}
	return key, k, nil
}

// Keys lists the keys not revoked, saying which expired unused.
func Keys(ctx context.Context, st *state.State, now time.Time) ([]KeyView, error) {
	keys, err := st.APIKeys(ctx)
	if err != nil {
		return nil, err
	}
	out := []KeyView{}
	for _, k := range keys {
		if k.RevokedAt != nil {
			continue
		}
		v := KeyView{APIKey: k}
		if k.LastUsedAt == nil {
			at := k.CreatedAt.Add(keyUnusedFor)
			v.ExpiresAt = &at
			v.Expired = !now.Before(at)
		}
		out = append(out, v)
	}
	return out, nil
}

// RevokeKey stops a key at once.
func RevokeKey(ctx context.Context, st *state.State, id string, now time.Time) error {
	return st.RevokeAPIKey(ctx, id, now)
}

// access checks the credentials agents send, and keeps their last use.
type access struct {
	st       *state.State
	cfg      *Config
	now      func() time.Time
	mu       sync.Mutex
	touched  map[string]time.Time
	resource string
}

func newAccess(cfg *Config, st *state.State) *access {
	return &access{st: st, cfg: cfg, now: time.Now, touched: map[string]time.Time{}, resource: cfg.Origin + "/mcp"}
}

// touch records a use at most once a minute per key or connection, but
// always the first one.
func (a *access) touch(id string, write func(context.Context, string, time.Time) error) {
	now := a.now()
	a.mu.Lock()
	last, seen := a.touched[id]
	if seen && now.Sub(last) < touchEvery {
		a.mu.Unlock()
		return
	}
	a.touched[id] = now
	a.mu.Unlock()
	_ = write(context.Background(), id, now)
}

// authenticate is who a Bearer credential is, or false when it is no valid
// key or access token.
func (a *access) authenticate(ctx context.Context, bearer string) (caller.Caller, bool) {
	switch {
	case strings.HasPrefix(bearer, KeyPrefix):
		return a.byKey(ctx, bearer)
	case strings.HasPrefix(bearer, accessPrefix):
		return a.byToken(ctx, bearer)
	}
	return caller.Caller{}, false
}

func (a *access) byKey(ctx context.Context, key string) (caller.Caller, bool) {
	// The key is found by its hash, so no comparison runs on the secret.
	k, err := a.st.APIKeyByHash(ctx, hashSecret(key))
	if err != nil || k.RevokedAt != nil {
		return caller.Caller{}, false
	}
	if k.LastUsedAt == nil && !a.now().Before(k.CreatedAt.Add(keyUnusedFor)) {
		return caller.Caller{}, false
	}
	a.touch(k.ID, a.st.TouchAPIKey)
	return a.callerFor(caller.APIKey, k.ID, k.Name, k.ReadOnly), true
}

func (a *access) byToken(ctx context.Context, token string) (caller.Caller, bool) {
	t, err := a.st.TokenByHash(ctx, hashSecret(token))
	if err != nil || t.Kind != "access" || t.ExpiresAt == nil || !a.now().Before(*t.ExpiresAt) {
		return caller.Caller{}, false
	}
	c, ok := a.liveConnection(ctx, t.Connection)
	if !ok {
		return caller.Caller{}, false
	}
	a.touch(c.ID, a.st.TouchConnection)
	return a.callerFor(caller.OAuth, c.ID, c.ClientName, c.ReadOnly), true
}

// liveConnection is a login still valid: not revoked, for this server, and
// approved with the password in force (a new password ends it).
func (a *access) liveConnection(ctx context.Context, id string) (state.Connection, bool) {
	c, err := a.st.Connection(ctx, id)
	if err != nil || c.RevokedAt != nil || c.Resource != a.resource {
		return c, false
	}
	hash, err := a.cfg.Password.Current()
	if err != nil {
		return c, false
	}
	if c.Password != passwordFingerprint(hash) {
		_ = a.st.RevokeConnection(ctx, c.ID, a.now())
		return c, false
	}
	return c, true
}

func (a *access) callerFor(kind caller.Kind, id, name string, readOnly bool) caller.Caller {
	c := caller.Caller{Kind: kind, ConnectionID: id, ConnectionName: name, Origin: a.cfg.Origin, Client: string(kind) + ":" + id}
	if readOnly {
		c.Categories = caller.ReadOnlyCategories()
	}
	return c
}

// issue makes a connection's access and refresh tokens.
func (a *access) issue(ctx context.Context, connection string) (accessToken, refreshToken string, err error) {
	now := a.now()
	accessToken, refreshToken = secret(accessPrefix), secret(refreshPrefix)
	if err := a.st.AddToken(ctx, hashSecret(accessToken), connection, "access", now.Add(accessTTL), now); err != nil {
		return "", "", err
	}
	if err := a.st.AddToken(ctx, hashSecret(refreshToken), connection, "refresh", time.Time{}, now); err != nil {
		return "", "", err
	}
	// Spent refresh tokens are kept a month, so a stolen one replayed late
	// still ends its connection.
	_ = a.st.PruneTokens(ctx, now.Add(-30*24*time.Hour))
	return accessToken, refreshToken, nil
}
