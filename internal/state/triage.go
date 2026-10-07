package state

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Mark is what a reply pass decided about a chat, kept so the next pass sees
// only what is left. Nothing about it reaches WhatsApp.
type Mark struct {
	ChatJID     string     `json:"chat_jid"`
	HandledAt   *time.Time `json:"handled_at,omitempty"`
	SnoozedAt   *time.Time `json:"snoozed_at,omitempty"`
	SnoozeUntil *time.Time `json:"snooze_until,omitempty"`
	Note        string     `json:"note,omitempty"`
}

// Hides reports whether the mark keeps a chat out of the triage lists, given
// when its latest message from someone else arrived. A message newer than the
// mark always brings the chat back.
func (m Mark) Hides(lastIncoming, now time.Time) bool {
	if m.HandledAt != nil && !lastIncoming.After(*m.HandledAt) {
		return true
	}
	return m.SnoozeUntil != nil && now.Before(*m.SnoozeUntil) && m.SnoozedAt != nil && !lastIncoming.After(*m.SnoozedAt)
}

func (s *State) migrateTriage() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS triage (
		chat_jid TEXT PRIMARY KEY,
		handled_at INTEGER NOT NULL DEFAULT 0,
		snoozed_at INTEGER NOT NULL DEFAULT 0,
		snooze_until INTEGER NOT NULL DEFAULT 0,
		note TEXT NOT NULL DEFAULT '')`)
	return err
}

func unixPtr(v int64) *time.Time {
	if v == 0 {
		return nil
	}
	t := time.Unix(v, 0).UTC()
	return &t
}

// Marks returns every chat's mark, by chat.
func (s *State) Marks(ctx context.Context) (map[string]Mark, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT chat_jid, handled_at, snoozed_at, snooze_until, note FROM triage`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Mark{}
	for rows.Next() {
		var m Mark
		var handled, snoozed, until int64
		if err := rows.Scan(&m.ChatJID, &handled, &snoozed, &until, &m.Note); err != nil {
			return nil, err
		}
		m.HandledAt, m.SnoozedAt, m.SnoozeUntil = unixPtr(handled), unixPtr(snoozed), unixPtr(until)
		out[m.ChatJID] = m
	}
	return out, rows.Err()
}

// MarkOf returns one chat's mark, ok false when it has none.
func (s *State) MarkOf(ctx context.Context, chat string) (Mark, bool, error) {
	m := Mark{ChatJID: chat}
	var handled, snoozed, until int64
	err := s.db.QueryRowContext(ctx, `SELECT handled_at, snoozed_at, snooze_until, note FROM triage WHERE chat_jid = ?`, chat).
		Scan(&handled, &snoozed, &until, &m.Note)
	if errors.Is(err, sql.ErrNoRows) {
		return m, false, nil
	}
	if err != nil {
		return m, false, err
	}
	m.HandledAt, m.SnoozedAt, m.SnoozeUntil = unixPtr(handled), unixPtr(snoozed), unixPtr(until)
	return m, true, nil
}

// MarkHandled records that a chat was dealt with now, and lifts its snooze.
func (s *State) MarkHandled(ctx context.Context, chat, note string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO triage (chat_jid, handled_at, note) VALUES (?, ?, ?)
		ON CONFLICT (chat_jid) DO UPDATE SET handled_at = excluded.handled_at, snoozed_at = 0, snooze_until = 0,
			note = CASE WHEN excluded.note != '' THEN excluded.note ELSE triage.note END`, chat, at.Unix(), note)
	return err
}

// Snooze hides a chat until a moment, unless someone writes in it first. It
// replaces a handled mark: the chat is meant to come back then.
func (s *State) Snooze(ctx context.Context, chat, note string, at, until time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO triage (chat_jid, snoozed_at, snooze_until, note) VALUES (?, ?, ?, ?)
		ON CONFLICT (chat_jid) DO UPDATE SET handled_at = 0, snoozed_at = excluded.snoozed_at, snooze_until = excluded.snooze_until,
			note = CASE WHEN excluded.note != '' THEN excluded.note ELSE triage.note END`, chat, at.Unix(), until.Unix(), note)
	return err
}

// ClearMark forgets what was decided about a chat.
func (s *State) ClearMark(ctx context.Context, chat string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM triage WHERE chat_jid = ?`, chat)
	return err
}
