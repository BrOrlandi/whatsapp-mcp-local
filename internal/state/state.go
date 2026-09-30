// Package state is the gateway's own small database: the transcripts of voice
// notes and the settings the tools can change. It lives beside wacli's store,
// never inside it, because wacli asks companions not to write to wacli.db.
package state

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type State struct {
	db *sql.DB
}

// Transcript is the text of one voice note, and where it came from.
type Transcript struct {
	ChatJID   string    `json:"chat_jid"`
	MessageID string    `json:"message_id"`
	Text      string    `json:"text"`
	Language  string    `json:"language,omitempty"`
	Model     string    `json:"model,omitempty"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

func Open(dir string) (*State, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "state.db")
	dsn := "file:" + (&url.URL{Path: path}).EscapedPath() + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS transcripts (
			chat_jid TEXT NOT NULL,
			message_id TEXT NOT NULL,
			text TEXT NOT NULL,
			language TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			PRIMARY KEY (chat_jid, message_id)
		);
		CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);`); err != nil {
		db.Close()
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	return &State{db: db}, nil
}

func (s *State) Close() error { return s.db.Close() }

var ErrNoTranscript = errors.New("no transcript kept for that message")

func (s *State) Transcript(ctx context.Context, chatJID, messageID string) (Transcript, error) {
	t := Transcript{ChatJID: chatJID, MessageID: messageID}
	var created int64
	err := s.db.QueryRowContext(ctx, `SELECT text, language, model, source, created_at FROM transcripts WHERE chat_jid = ? AND message_id = ?`,
		chatJID, messageID).Scan(&t.Text, &t.Language, &t.Model, &t.Source, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNoTranscript
	}
	t.CreatedAt = time.Unix(created, 0).UTC()
	return t, err
}

// Transcripts returns the kept transcripts of the given messages of one chat.
func (s *State) Transcripts(ctx context.Context, chatJID string, ids []string) (map[string]Transcript, error) {
	out := map[string]Transcript{}
	for _, id := range ids {
		t, err := s.Transcript(ctx, chatJID, id)
		if errors.Is(err, ErrNoTranscript) {
			continue
		}
		if err != nil {
			return out, err
		}
		out[id] = t
	}
	return out, nil
}

// SearchTranscripts finds transcripts containing text, so a search reaches
// what was said in voice notes too.
func (s *State) SearchTranscripts(ctx context.Context, text, chatJID string, limit int) ([]Transcript, error) {
	query := `SELECT chat_jid, message_id, text, language, model, source, created_at FROM transcripts WHERE text LIKE ?`
	args := []any{"%" + text + "%"}
	if chatJID != "" {
		query += ` AND chat_jid = ?`
		args = append(args, chatJID)
	}
	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Transcript
	for rows.Next() {
		var t Transcript
		var created int64
		if err := rows.Scan(&t.ChatJID, &t.MessageID, &t.Text, &t.Language, &t.Model, &t.Source, &created); err != nil {
			return nil, err
		}
		t.CreatedAt = time.Unix(created, 0).UTC()
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *State) SaveTranscript(ctx context.Context, t Transcript) error {
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO transcripts (chat_jid, message_id, text, language, model, source, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (chat_jid, message_id) DO UPDATE SET text = excluded.text, language = excluded.language,
			model = excluded.model, source = excluded.source, created_at = excluded.created_at`,
		t.ChatJID, t.MessageID, t.Text, t.Language, t.Model, t.Source, t.CreatedAt.Unix())
	return err
}

func (s *State) Setting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s *State) SetSetting(ctx context.Context, key, value string) error {
	if value == "" {
		_, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key)
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}
