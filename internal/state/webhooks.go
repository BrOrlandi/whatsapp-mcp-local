package state

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Webhook is an address that receives a POST for every new message, so a
// script on this computer (or the network) can act on it.
type Webhook struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Secret string `json:"-"`
	// Events is what it receives: message, reaction, receipt.
	Events []string `json:"events"`
	// IncludeOwn also delivers the messages the account itself sends.
	IncludeOwn bool `json:"include_own"`
	// Chats limits it to these conversations; empty means all of them.
	Chats     []string  `json:"chats,omitempty"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`

	DisabledAt     *time.Time `json:"disabled_at,omitempty"`
	DisabledReason string     `json:"disabled_reason,omitempty"`
	LastAttemptAt  *time.Time `json:"last_attempt_at,omitempty"`
	LastSuccessAt  *time.Time `json:"last_success_at,omitempty"`
	LastResult     string     `json:"last_result,omitempty"`
	Delivered      int64      `json:"delivered"`
}

// ErrNoWebhook means no webhook has that id.
var ErrNoWebhook = errors.New("no webhook has that id")

func (s *State) migrateWebhooks() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS webhooks (
		id TEXT PRIMARY KEY,
		url TEXT NOT NULL,
		secret TEXT NOT NULL,
		events TEXT NOT NULL,
		include_own INTEGER NOT NULL DEFAULT 0,
		chats TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at INTEGER NOT NULL,
		disabled_at INTEGER NOT NULL DEFAULT 0,
		disabled_reason TEXT NOT NULL DEFAULT '',
		last_attempt_at INTEGER NOT NULL DEFAULT 0,
		last_success_at INTEGER NOT NULL DEFAULT 0,
		last_result TEXT NOT NULL DEFAULT '',
		delivered INTEGER NOT NULL DEFAULT 0)`)
	return err
}

const webhookColumns = `id, url, secret, events, include_own, chats, enabled, created_at, disabled_at, disabled_reason,
	last_attempt_at, last_success_at, last_result, delivered`

func splitList(v string) []string {
	if v == "" {
		return nil
	}
	return strings.Split(v, ",")
}

func scanWebhook(row interface{ Scan(...any) error }) (Webhook, error) {
	var w Webhook
	var events, chats string
	var created, disabled, attempt, success int64
	err := row.Scan(&w.ID, &w.URL, &w.Secret, &events, &w.IncludeOwn, &chats, &w.Enabled, &created, &disabled, &w.DisabledReason,
		&attempt, &success, &w.LastResult, &w.Delivered)
	w.Events, w.Chats = splitList(events), splitList(chats)
	w.CreatedAt = time.Unix(created, 0).UTC()
	w.DisabledAt, w.LastAttemptAt, w.LastSuccessAt = unixPtr(disabled), unixPtr(attempt), unixPtr(success)
	return w, err
}

// Webhooks lists every webhook, oldest first.
func (s *State) Webhooks(ctx context.Context) ([]Webhook, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+webhookColumns+` FROM webhooks ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Webhook
	for rows.Next() {
		w, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *State) Webhook(ctx context.Context, id string) (Webhook, error) {
	w, err := scanWebhook(s.db.QueryRowContext(ctx, `SELECT `+webhookColumns+` FROM webhooks WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return w, ErrNoWebhook
	}
	return w, err
}

// SaveWebhook creates a webhook or replaces its settings, keeping its
// delivery record.
func (s *State) SaveWebhook(ctx context.Context, w Webhook) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO webhooks (id, url, secret, events, include_own, chats, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET url = excluded.url, events = excluded.events, include_own = excluded.include_own,
			chats = excluded.chats, enabled = excluded.enabled`,
		w.ID, w.URL, w.Secret, strings.Join(w.Events, ","), w.IncludeOwn, strings.Join(w.Chats, ","), w.Enabled, w.CreatedAt.Unix())
	return err
}

func (s *State) DeleteWebhook(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM webhooks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoWebhook
	}
	return nil
}

// RecordDelivery notes one attempt and its result.
func (s *State) RecordDelivery(ctx context.Context, id string, at time.Time, result string, ok bool) error {
	query := `UPDATE webhooks SET last_attempt_at = ?1, last_result = ?2 WHERE id = ?3`
	if ok {
		query = `UPDATE webhooks SET last_attempt_at = ?1, last_result = ?2, last_success_at = ?1, delivered = delivered + 1 WHERE id = ?3`
	}
	_, err := s.db.ExecContext(ctx, query, at.Unix(), result, id)
	return err
}

// DisableWebhook turns a webhook off, saying why.
func (s *State) DisableWebhook(ctx context.Context, id string, at time.Time, reason string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE webhooks SET enabled = 0, disabled_at = ?, disabled_reason = ? WHERE id = ?`, at.Unix(), reason, id)
	return err
}

// EnableWebhook turns a webhook back on, forgetting why it was off.
func (s *State) EnableWebhook(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE webhooks SET enabled = 1, disabled_at = 0, disabled_reason = '' WHERE id = ?`, id)
	return err
}
