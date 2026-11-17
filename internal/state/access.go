package state

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// The server mode's ways in for agents on other computers: API keys, and
// the OAuth clients, connections and tokens of a login. Only the SHA-256 of
// a key or token is kept: the secret itself is shown once and never stored.

// ErrNoAccess means no key, client, connection or token has that id.
var ErrNoAccess = errors.New("not found")

// APIKey is a key an agent sends as Authorization: Bearer.
type APIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	ReadOnly   bool       `json:"read_only"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// OAuthClient is a client that registered itself (RFC 7591).
type OAuthClient struct {
	ID           string
	Name         string
	RedirectURIs []string
	CreatedAt    time.Time
	LastUsedAt   *time.Time
}

// Connection is an agent let in through the OAuth login: what the person
// approved, and with which permissions.
type Connection struct {
	ID         string     `json:"id"`
	ClientID   string     `json:"client_id"`
	ClientName string     `json:"client_name"`
	ReadOnly   bool       `json:"read_only"`
	Resource   string     `json:"-"`
	Password   string     `json:"-"` // the fingerprint of the password it was approved with
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// Token is an OAuth access or refresh token, by its hash.
type Token struct {
	Connection string
	Kind       string // access or refresh
	ExpiresAt  *time.Time
	UsedAt     *time.Time
}

func (s *State) migrateAccess() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS api_keys (
			id TEXT PRIMARY KEY,
			hash TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			read_only INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			last_used_at INTEGER NOT NULL DEFAULT 0,
			revoked_at INTEGER NOT NULL DEFAULT 0);
		CREATE TABLE IF NOT EXISTS oauth_clients (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			redirect_uris TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			last_used_at INTEGER NOT NULL DEFAULT 0);
		CREATE TABLE IF NOT EXISTS oauth_connections (
			id TEXT PRIMARY KEY,
			client_id TEXT NOT NULL,
			client_name TEXT NOT NULL,
			read_only INTEGER NOT NULL DEFAULT 0,
			resource TEXT NOT NULL,
			password TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			last_used_at INTEGER NOT NULL DEFAULT 0,
			revoked_at INTEGER NOT NULL DEFAULT 0);
		CREATE TABLE IF NOT EXISTS oauth_tokens (
			hash TEXT PRIMARY KEY,
			connection_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			expires_at INTEGER NOT NULL DEFAULT 0,
			used_at INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL);
		CREATE INDEX IF NOT EXISTS oauth_tokens_connection ON oauth_tokens (connection_id);`)
	return err
}

func unix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// ---- API keys ----

// AddAPIKey keeps a new key, by the hash of its secret.
func (s *State) AddAPIKey(ctx context.Context, k APIKey, hash string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO api_keys (id, hash, name, read_only, created_at) VALUES (?, ?, ?, ?, ?)`,
		k.ID, hash, k.Name, k.ReadOnly, unix(k.CreatedAt))
	return err
}

const apiKeyColumns = `id, name, read_only, created_at, last_used_at, revoked_at`

func scanAPIKey(row interface{ Scan(...any) error }) (APIKey, error) {
	var k APIKey
	var created, used, revoked int64
	if err := row.Scan(&k.ID, &k.Name, &k.ReadOnly, &created, &used, &revoked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return k, ErrNoAccess
		}
		return k, err
	}
	k.CreatedAt, k.LastUsedAt, k.RevokedAt = time.Unix(created, 0).UTC(), unixPtr(used), unixPtr(revoked)
	return k, nil
}

// APIKeyByHash is the key whose secret has this hash, revoked or not.
func (s *State) APIKeyByHash(ctx context.Context, hash string) (APIKey, error) {
	return scanAPIKey(s.db.QueryRowContext(ctx, `SELECT `+apiKeyColumns+` FROM api_keys WHERE hash = ?`, hash))
}

// APIKeys are every key, newest first, revoked ones included.
func (s *State) APIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+apiKeyColumns+` FROM api_keys ORDER BY created_at DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIKey{}
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// RevokeAPIKey stops a key at once.
func (s *State) RevokeAPIKey(ctx context.Context, id string, at time.Time) error {
	res, err := s.db.ExecContext(ctx, `UPDATE api_keys SET revoked_at = ? WHERE id = ? AND revoked_at = 0`, unix(at), id)
	return affected(res, err)
}

// TouchAPIKey records a key's use.
func (s *State) TouchAPIKey(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = ? WHERE id = ?`, unix(at), id)
	return err
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoAccess
	}
	return nil
}

// ---- OAuth clients ----

// AddOAuthClient keeps a registered client.
func (s *State) AddOAuthClient(ctx context.Context, c OAuthClient) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO oauth_clients (id, name, redirect_uris, created_at) VALUES (?, ?, ?, ?)`,
		c.ID, c.Name, strings.Join(c.RedirectURIs, "\n"), unix(c.CreatedAt))
	return err
}

// OAuthClient is a registered client.
func (s *State) OAuthClient(ctx context.Context, id string) (OAuthClient, error) {
	var c OAuthClient
	var uris string
	var created, used int64
	err := s.db.QueryRowContext(ctx, `SELECT id, name, redirect_uris, created_at, last_used_at FROM oauth_clients WHERE id = ?`, id).
		Scan(&c.ID, &c.Name, &uris, &created, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNoAccess
	}
	c.RedirectURIs, c.CreatedAt, c.LastUsedAt = strings.Split(uris, "\n"), time.Unix(created, 0).UTC(), unixPtr(used)
	return c, err
}

// TouchOAuthClient records that a client was used to sign in.
func (s *State) TouchOAuthClient(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE oauth_clients SET last_used_at = ? WHERE id = ?`, unix(at), id)
	return err
}

// OAuthClientCount is how many clients are registered.
func (s *State) OAuthClientCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM oauth_clients`).Scan(&n)
	return n, err
}

// PruneOAuthClients drops the clients registered before the given time that
// never led to a sign-in, and reports how many it dropped.
func (s *State) PruneOAuthClients(ctx context.Context, before time.Time) (int, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM oauth_clients WHERE last_used_at = 0 AND created_at < ?`, unix(before))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ---- connections ----

// AddConnection keeps an approved login.
func (s *State) AddConnection(ctx context.Context, c Connection) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO oauth_connections (id, client_id, client_name, read_only, resource, password, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, c.ID, c.ClientID, c.ClientName, c.ReadOnly, c.Resource, c.Password, unix(c.CreatedAt))
	return err
}

const connectionColumns = `id, client_id, client_name, read_only, resource, password, created_at, last_used_at, revoked_at`

func scanConnection(row interface{ Scan(...any) error }) (Connection, error) {
	var c Connection
	var created, used, revoked int64
	if err := row.Scan(&c.ID, &c.ClientID, &c.ClientName, &c.ReadOnly, &c.Resource, &c.Password, &created, &used, &revoked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c, ErrNoAccess
		}
		return c, err
	}
	c.CreatedAt, c.LastUsedAt, c.RevokedAt = time.Unix(created, 0).UTC(), unixPtr(used), unixPtr(revoked)
	return c, nil
}

// Connection is a login by its id.
func (s *State) Connection(ctx context.Context, id string) (Connection, error) {
	return scanConnection(s.db.QueryRowContext(ctx, `SELECT `+connectionColumns+` FROM oauth_connections WHERE id = ?`, id))
}

// Connections are the logins not revoked, newest first.
func (s *State) Connections(ctx context.Context) ([]Connection, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+connectionColumns+` FROM oauth_connections WHERE revoked_at = 0 ORDER BY created_at DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Connection{}
	for rows.Next() {
		c, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// RevokeConnection ends a login at once: it and every token it had.
func (s *State) RevokeConnection(ctx context.Context, id string, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE oauth_connections SET revoked_at = ? WHERE id = ? AND revoked_at = 0`, unix(at), id)
	if err := affected(res, err); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM oauth_tokens WHERE connection_id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// TouchConnection records a login's use.
func (s *State) TouchConnection(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE oauth_connections SET last_used_at = ? WHERE id = ?`, unix(at), id)
	return err
}

// ---- tokens ----

// AddToken keeps a token of a connection, by its hash.
func (s *State) AddToken(ctx context.Context, hash, connection, kind string, expires, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO oauth_tokens (hash, connection_id, kind, expires_at, created_at) VALUES (?, ?, ?, ?, ?)`,
		hash, connection, kind, unix(expires), unix(at))
	return err
}

// TokenByHash is a token by the hash of its secret.
func (s *State) TokenByHash(ctx context.Context, hash string) (Token, error) {
	var t Token
	var expires, used int64
	err := s.db.QueryRowContext(ctx, `SELECT connection_id, kind, expires_at, used_at FROM oauth_tokens WHERE hash = ?`, hash).
		Scan(&t.Connection, &t.Kind, &expires, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNoAccess
	}
	t.ExpiresAt, t.UsedAt = unixPtr(expires), unixPtr(used)
	return t, err
}

// SpendRefreshToken marks a refresh token as used, and reports whether this
// call was the one that did: a second use of the same token finds it spent.
func (s *State) SpendRefreshToken(ctx context.Context, hash string, at time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE oauth_tokens SET used_at = ? WHERE hash = ? AND kind = 'refresh' AND used_at = 0`, unix(at), hash)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// PruneTokens drops the access tokens that expired before the given time,
// and the refresh tokens spent before it.
func (s *State) PruneTokens(ctx context.Context, before time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM oauth_tokens WHERE (kind = 'access' AND expires_at < ?) OR (kind = 'refresh' AND used_at != 0 AND used_at < ?)`,
		unix(before), unix(before))
	return err
}
