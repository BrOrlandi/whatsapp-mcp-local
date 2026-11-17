package state

import (
	"context"
	"time"
)

// A known browser is one the panel's password was typed in, remembered by
// the hash of a random id kept in a cookie: during a flood of password
// tries, it gets a reserve of turns of its own, whatever its address. The
// password fingerprint it was remembered with ends it when the password
// changes. Only the hash is kept.

// KnownBrowser is a remembered browser, by the hash of its cookie.
type KnownBrowser struct {
	Hash     string
	Password string // the fingerprint of the password it signed in with
	LastUsed time.Time
}

func (s *State) migrateBrowsers() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS known_browsers (
		hash TEXT PRIMARY KEY,
		password TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		last_used_at INTEGER NOT NULL)`)
	return err
}

// KnownBrowsers are the remembered browsers, most recently used first.
func (s *State) KnownBrowsers(ctx context.Context) ([]KnownBrowser, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT hash, password, last_used_at FROM known_browsers ORDER BY last_used_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KnownBrowser
	for rows.Next() {
		var b KnownBrowser
		var used int64
		if err := rows.Scan(&b.Hash, &b.Password, &used); err != nil {
			return nil, err
		}
		b.LastUsed = time.Unix(used, 0).UTC()
		out = append(out, b)
	}
	return out, rows.Err()
}

// RememberBrowser keeps a browser, or records its new use, and keeps only
// the max most recently used.
func (s *State) RememberBrowser(ctx context.Context, b KnownBrowser, max int) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO known_browsers (hash, password, created_at, last_used_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(hash) DO UPDATE SET password = excluded.password, last_used_at = excluded.last_used_at`,
		b.Hash, b.Password, unix(b.LastUsed), unix(b.LastUsed)); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM known_browsers WHERE hash NOT IN
		(SELECT hash FROM known_browsers ORDER BY last_used_at DESC, created_at DESC LIMIT ?)`, max)
	return err
}

// ForgetBrowsers drops every browser not remembered with this password
// fingerprint.
func (s *State) ForgetBrowsers(ctx context.Context, password string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM known_browsers WHERE password != ?`, password)
	return err
}
