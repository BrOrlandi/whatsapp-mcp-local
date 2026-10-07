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
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type State struct {
	db *sql.DB
}

// Transcript is the text of one voice note, and where it came from.
type Transcript struct {
	ChatJID   string `json:"chat_jid"`
	MessageID string `json:"message_id"`
	Text      string `json:"text"`
	// Raw is what the speech engine heard, kept when Text was corrected from
	// the conversation's context, so a correction can always be checked.
	Raw       string    `json:"raw_text,omitempty"`
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
	st := &State{db: db}
	if err := st.migrateClients(); err != nil {
		db.Close()
		return nil, err
	}
	if err := st.migrateTriage(); err != nil {
		db.Close()
		return nil, err
	}
	return st, nil
}

func (s *State) Close() error { return s.db.Close() }

var ErrNoTranscript = errors.New("no transcript kept for that message")

func (s *State) Transcript(ctx context.Context, chatJID, messageID string) (Transcript, error) {
	t := Transcript{ChatJID: chatJID, MessageID: messageID}
	var created int64
	err := s.db.QueryRowContext(ctx, `SELECT text, raw_text, language, model, source, created_at FROM transcripts WHERE chat_jid = ? AND message_id = ?`,
		chatJID, messageID).Scan(&t.Text, &t.Raw, &t.Language, &t.Model, &t.Source, &created)
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
	query := `SELECT chat_jid, message_id, text, raw_text, language, model, source, created_at FROM transcripts WHERE text LIKE ?`
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
		if err := rows.Scan(&t.ChatJID, &t.MessageID, &t.Text, &t.Raw, &t.Language, &t.Model, &t.Source, &created); err != nil {
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
	_, err := s.db.ExecContext(ctx, `INSERT INTO transcripts (chat_jid, message_id, text, raw_text, language, model, source, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (chat_jid, message_id) DO UPDATE SET text = excluded.text, raw_text = excluded.raw_text, language = excluded.language,
			model = excluded.model, source = excluded.source, created_at = excluded.created_at`,
		t.ChatJID, t.MessageID, t.Text, t.Raw, t.Language, t.Model, t.Source, t.CreatedAt.Unix())
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

// Client is an MCP client that has connected to the daemon: what it calls
// itself in the initialize handshake, and when it was last heard from.
type Client struct {
	Name      string    `json:"name"`
	Version   string    `json:"version,omitempty"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

func (s *State) migrateClients() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS clients (
		name TEXT PRIMARY KEY, version TEXT NOT NULL DEFAULT '', first_seen INTEGER NOT NULL, last_seen INTEGER NOT NULL)`)
	if err != nil {
		return err
	}
	// raw_text arrived after the first release of the table.
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('transcripts') WHERE name = 'raw_text'`).Scan(&n)
	if n == 0 {
		_, err = s.db.Exec(`ALTER TABLE transcripts ADD COLUMN raw_text TEXT NOT NULL DEFAULT ''`)
	}
	return err
}

// Transcribed reports which of the given (chat, message) pairs already have
// a transcript, keyed "chat/id".
func (s *State) Transcribed(ctx context.Context, keys []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, k := range keys {
		chat, id, _ := strings.Cut(k, "/")
		var one int
		err := s.db.QueryRowContext(ctx, `SELECT 1 FROM transcripts WHERE chat_jid = ? AND message_id = ?`, chat, id).Scan(&one)
		if err == nil {
			out[k] = true
		} else if !errors.Is(err, sql.ErrNoRows) {
			return out, err
		}
	}
	return out, nil
}

// TranscriptCount is how many voice notes have a transcript, and how many of
// those were corrected from context.
func (s *State) TranscriptCount(ctx context.Context) (total, corrected int, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(raw_text != '' AND raw_text != text), 0) FROM transcripts`).Scan(&total, &corrected)
	return
}

// SeeClient records that a client connected or made a call.
func (s *State) SeeClient(ctx context.Context, name, version string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO clients (name, version, first_seen, last_seen) VALUES (?, ?, ?, ?)
		ON CONFLICT (name) DO UPDATE SET last_seen = excluded.last_seen,
			version = CASE WHEN excluded.version != '' THEN excluded.version ELSE clients.version END`,
		name, version, at.Unix(), at.Unix())
	return err
}

func (s *State) Clients(ctx context.Context) ([]Client, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, version, first_seen, last_seen FROM clients ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Client
	for rows.Next() {
		var c Client
		var first, last int64
		if err := rows.Scan(&c.Name, &c.Version, &first, &last); err != nil {
			return nil, err
		}
		c.FirstSeen, c.LastSeen = time.Unix(first, 0), time.Unix(last, 0)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *State) ForgetClient(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM clients WHERE name = ?`, name)
	return err
}
