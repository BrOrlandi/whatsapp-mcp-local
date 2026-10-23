package state

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// RemoteClient is a device paired to reach this daemon over the network
// (internal/remote): what it is called, the public half of its key, and what
// it may do. No secret of it is kept here: the device proves itself with
// its private key, which never leaves it.
type RemoteClient struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	PublicKey []byte     `json:"-"`
	Scope     string     `json:"scope"` // admin or tools
	CreatedAt time.Time  `json:"created_at"`
	LastSeen  *time.Time `json:"last_seen_at,omitempty"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	// LastHandshake is the newest handshake timestamp the device sent, which
	// a replayed handshake cannot beat.
	LastHandshake int64 `json:"-"`
}

// ErrNoRemoteClient means no paired device has that id or key.
var ErrNoRemoteClient = errors.New("no paired device has that id")

func (s *State) migrateRemote() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS remote_clients (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		public_key BLOB NOT NULL UNIQUE,
		scope TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		last_seen_at INTEGER,
		revoked_at INTEGER,
		last_handshake INTEGER NOT NULL DEFAULT 0
	)`)
	return err
}

func scanRemote(row interface{ Scan(...any) error }) (RemoteClient, error) {
	var c RemoteClient
	var created int64
	var seen, revoked sql.NullInt64
	if err := row.Scan(&c.ID, &c.Name, &c.PublicKey, &c.Scope, &created, &seen, &revoked, &c.LastHandshake); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c, ErrNoRemoteClient
		}
		return c, err
	}
	c.CreatedAt = time.Unix(created, 0).UTC()
	if seen.Valid {
		t := time.Unix(seen.Int64, 0).UTC()
		c.LastSeen = &t
	}
	if revoked.Valid {
		t := time.Unix(revoked.Int64, 0).UTC()
		c.RevokedAt = &t
	}
	return c, nil
}

const remoteColumns = `id, name, public_key, scope, created_at, last_seen_at, revoked_at, last_handshake`

// AddRemoteClient records a newly paired device.
func (s *State) AddRemoteClient(ctx context.Context, c RemoteClient) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO remote_clients (id, name, public_key, scope, created_at) VALUES (?, ?, ?, ?, ?)`,
		c.ID, c.Name, c.PublicKey, c.Scope, c.CreatedAt.Unix())
	return err
}

// RemoteClientByKey is the device with this public key, revoked or not.
func (s *State) RemoteClientByKey(ctx context.Context, key []byte) (RemoteClient, error) {
	return scanRemote(s.db.QueryRowContext(ctx, `SELECT `+remoteColumns+` FROM remote_clients WHERE public_key = ?`, key))
}

// RemoteClient is the device with this id.
func (s *State) RemoteClient(ctx context.Context, id string) (RemoteClient, error) {
	return scanRemote(s.db.QueryRowContext(ctx, `SELECT `+remoteColumns+` FROM remote_clients WHERE id = ?`, id))
}

// RemoteClients lists the paired devices, newest first.
func (s *State) RemoteClients(ctx context.Context) ([]RemoteClient, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+remoteColumns+` FROM remote_clients ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RemoteClient
	for rows.Next() {
		c, err := scanRemote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// TouchRemoteClient records a handshake: when, and its timestamp, which
// must be newer than the last one. It reports false for a replay.
func (s *State) TouchRemoteClient(ctx context.Context, id string, handshake int64, at time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE remote_clients SET last_seen_at = ?, last_handshake = ?
		WHERE id = ? AND revoked_at IS NULL AND last_handshake < ?`, at.Unix(), handshake, id, handshake)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// RevokeRemoteClient stops a device for good; it has to be paired again.
func (s *State) RevokeRemoteClient(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE remote_clients SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoRemoteClient
	}
	return nil
}

// RotateRemoteClient replaces a device's key with a new one it proved it
// holds, over a session opened with the old one.
func (s *State) RotateRemoteClient(ctx context.Context, id string, key []byte) error {
	res, err := s.db.ExecContext(ctx, `UPDATE remote_clients SET public_key = ?, last_handshake = 0 WHERE id = ? AND revoked_at IS NULL`, key, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoRemoteClient
	}
	return nil
}
