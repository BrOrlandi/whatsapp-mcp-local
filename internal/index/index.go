// Package index reads wacli's message store directly, read-only, for the
// lookups its CLI does not offer: finding a message by id alone, and measuring
// how much of the past the store actually covers.
//
// wacli documents a read-only connection as safe while `sync --follow` writes,
// and asks companions never to write to it. This package only ever opens it
// with mode=ro.
package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Index struct {
	db *sql.DB
}

// Open opens wacli.db read-only. The file may not exist yet on a store that
// was never paired; every query then reports that plainly.
func Open(path string) (*Index, error) {
	dsn := "file:" + (&url.URL{Path: path}).EscapedPath() + "?mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	return &Index{db: db}, nil
}

func (x *Index) Close() error { return x.db.Close() }

// ErrNotFound means no stored message has that id.
var ErrNotFound = errors.New("no indexed message has that id")

// Message is the slice of a stored message the tools need to act on it.
type Message struct {
	ChatJID   string    `json:"chat_jid"`
	ChatName  string    `json:"chat_name,omitempty"`
	ID        string    `json:"id"`
	SenderJID string    `json:"sender_jid,omitempty"`
	FromMe    bool      `json:"from_me"`
	Timestamp time.Time `json:"timestamp"`
	Text      string    `json:"text,omitempty"`
	MediaType string    `json:"media_type,omitempty"`
	MimeType  string    `json:"mime_type,omitempty"`
	Filename  string    `json:"filename,omitempty"`
	Revoked   bool      `json:"revoked,omitempty"`
}

// MessageByID finds a message by id. Ids are unique per chat rather than
// globally, so a chat narrows the search when the caller knows it; otherwise
// the newest match wins.
func (x *Index) MessageByID(ctx context.Context, id, chatJID string) (Message, error) {
	query := `SELECT m.chat_jid, COALESCE(m.chat_name, c.name, ''), m.msg_id, COALESCE(m.sender_jid,''), m.from_me, m.ts,
		COALESCE(m.display_text, m.text, ''), COALESCE(m.media_type,''), COALESCE(m.mime_type,''), COALESCE(m.filename,''), m.revoked
		FROM messages m LEFT JOIN chats c ON c.jid = m.chat_jid
		WHERE m.msg_id = ?`
	args := []any{strings.TrimSpace(id)}
	if chatJID != "" {
		query += ` AND m.chat_jid = ?`
		args = append(args, chatJID)
	}
	query += ` ORDER BY m.ts DESC LIMIT 1`
	var m Message
	var ts int64
	err := x.db.QueryRowContext(ctx, query, args...).Scan(&m.ChatJID, &m.ChatName, &m.ID, &m.SenderJID, &m.FromMe, &ts,
		&m.Text, &m.MediaType, &m.MimeType, &m.Filename, &m.Revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrNotFound
	}
	if err != nil {
		return m, describe(err)
	}
	m.Timestamp = time.Unix(ts, 0).UTC()
	return m, nil
}

// Coverage is how far the store reaches.
type Coverage struct {
	Messages int64      `json:"messages"`
	Chats    int64      `json:"chats"`
	Oldest   *time.Time `json:"history_since,omitempty"`
	Newest   *time.Time `json:"newest_message,omitempty"`
}

func (x *Index) Coverage(ctx context.Context) (Coverage, error) {
	var c Coverage
	var oldest, newest sql.NullInt64
	err := x.db.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(DISTINCT chat_jid), MIN(ts), MAX(ts) FROM messages WHERE deleted_at IS NULL`).
		Scan(&c.Messages, &c.Chats, &oldest, &newest)
	if err != nil {
		return c, describe(err)
	}
	if oldest.Valid {
		t := time.Unix(oldest.Int64, 0).UTC()
		c.Oldest = &t
	}
	if newest.Valid {
		t := time.Unix(newest.Int64, 0).UTC()
		c.Newest = &t
	}
	return c, nil
}

// ChatOldest is when the store's knowledge of one chat begins.
func (x *Index) ChatOldest(ctx context.Context, chatJID string) (*time.Time, error) {
	var oldest sql.NullInt64
	if err := x.db.QueryRowContext(ctx, `SELECT MIN(ts) FROM messages WHERE chat_jid = ?`, chatJID).Scan(&oldest); err != nil {
		return nil, describe(err)
	}
	if !oldest.Valid {
		return nil, nil
	}
	t := time.Unix(oldest.Int64, 0).UTC()
	return &t, nil
}

// Gap is a window in which no conversation at all produced a message.
type Gap struct {
	From  time.Time `json:"from"`
	Until time.Time `json:"until"`
	Hours float64   `json:"hours"`
}

// Gaps finds windows longer than threshold, within the last `within`, in which
// the whole store is silent. One quiet chat is ordinary; every chat quiet at
// once is the shape a stopped sync leaves, so "nobody wrote" and "we were not
// listening" are told apart instead of reading the same.
func (x *Index) Gaps(ctx context.Context, threshold, within time.Duration, limit int) ([]Gap, error) {
	since := time.Now().Add(-within).Unix()
	rows, err := x.db.QueryContext(ctx, `SELECT prev, ts FROM (
		SELECT ts, LAG(ts) OVER (ORDER BY ts) AS prev FROM messages WHERE ts >= ?
	) WHERE prev IS NOT NULL AND ts - prev >= ? ORDER BY ts DESC LIMIT ?`, since, int64(threshold.Seconds()), limit)
	if err != nil {
		return nil, describe(err)
	}
	defer rows.Close()
	var gaps []Gap
	for rows.Next() {
		var from, until int64
		if err := rows.Scan(&from, &until); err != nil {
			return nil, err
		}
		gaps = append(gaps, Gap{From: time.Unix(from, 0).UTC(), Until: time.Unix(until, 0).UTC(), Hours: float64(until-from) / 3600})
	}
	return gaps, rows.Err()
}

// ActiveChats lists conversations by how recently they were active, for
// spreading history requests across the index.
func (x *Index) ActiveChats(ctx context.Context, limit int) ([]string, error) {
	rows, err := x.db.QueryContext(ctx, `SELECT chat_jid FROM messages GROUP BY chat_jid ORDER BY MAX(ts) DESC LIMIT ?`, limit)
	if err != nil {
		return nil, describe(err)
	}
	defer rows.Close()
	var chats []string
	for rows.Next() {
		var jid string
		if err := rows.Scan(&jid); err != nil {
			return nil, err
		}
		chats = append(chats, jid)
	}
	return chats, rows.Err()
}

// Contact is one address-book entry as wacli stored it.
type Contact struct {
	JID   string `json:"jid"`
	Phone string `json:"phone,omitempty"`
	Name  string `json:"name,omitempty"`
}

// Contacts lists stored contacts that have a name, optionally filtered by a
// name or number fragment.
func (x *Index) Contacts(ctx context.Context, search string, limit int) ([]Contact, error) {
	name := `COALESCE(NULLIF(a.alias,''), NULLIF(c.system_name,''), NULLIF(c.full_name,''), NULLIF(c.push_name,''), NULLIF(c.business_name,''), NULLIF(c.first_name,''), '')`
	query := `SELECT c.jid, COALESCE(c.phone,''), ` + name + ` AS display FROM contacts c LEFT JOIN contact_aliases a ON a.jid = c.jid
		WHERE ` + name + ` != ''`
	var args []any
	if s := strings.TrimSpace(search); s != "" {
		like := "%" + strings.ToLower(s) + "%"
		query += ` AND (LOWER(` + name + `) LIKE ? OR c.phone LIKE ? OR c.jid LIKE ?)`
		args = append(args, like, like, like)
	}
	query += ` ORDER BY display COLLATE NOCASE LIMIT ?`
	args = append(args, limit)
	rows, err := x.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, describe(err)
	}
	defer rows.Close()
	var out []Contact
	for rows.Next() {
		var c Contact
		if err := rows.Scan(&c.JID, &c.Phone, &c.Name); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Group is a group the account belongs to, from wacli's last snapshot.
type Group struct {
	JID          string        `json:"jid"`
	Name         string        `json:"name"`
	Owner        string        `json:"owner,omitempty"`
	Community    bool          `json:"community,omitempty"`
	Parent       string        `json:"community_jid,omitempty"`
	CreatedAt    *time.Time    `json:"created_at,omitempty"`
	Participants []Participant `json:"participants,omitempty"`
	Size         int           `json:"size"`
	SnapshotAt   time.Time     `json:"snapshot_at"`
}

type Participant struct {
	JID   string `json:"jid"`
	Name  string `json:"name,omitempty"`
	Phone string `json:"phone,omitempty"`
	Role  string `json:"role,omitempty"`
}

func (x *Index) Groups(ctx context.Context, search string, limit int) ([]Group, error) {
	query := `SELECT g.jid, COALESCE(g.name,''), COALESCE(g.owner_jid,''), g.is_parent, COALESCE(g.linked_parent_jid,''), COALESCE(g.created_ts,0), g.updated_at,
		(SELECT COUNT(*) FROM group_participants p WHERE p.group_jid = g.jid)
		FROM groups g WHERE g.left_at IS NULL`
	var args []any
	if s := strings.TrimSpace(search); s != "" {
		query += ` AND LOWER(g.name) LIKE ?`
		args = append(args, "%"+strings.ToLower(s)+"%")
	}
	query += ` ORDER BY g.name COLLATE NOCASE LIMIT ?`
	args = append(args, limit)
	rows, err := x.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, describe(err)
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func scanGroup(row interface{ Scan(...any) error }) (Group, error) {
	var g Group
	var created, updated int64
	if err := row.Scan(&g.JID, &g.Name, &g.Owner, &g.Community, &g.Parent, &created, &updated, &g.Size); err != nil {
		return g, err
	}
	if created > 0 {
		t := time.Unix(created, 0).UTC()
		g.CreatedAt = &t
	}
	g.SnapshotAt = time.Unix(updated, 0).UTC()
	return g, nil
}

// Group reads one group and its participants, optionally filtered.
func (x *Index) Group(ctx context.Context, jid, search string) (Group, error) {
	row := x.db.QueryRowContext(ctx, `SELECT g.jid, COALESCE(g.name,''), COALESCE(g.owner_jid,''), g.is_parent, COALESCE(g.linked_parent_jid,''), COALESCE(g.created_ts,0), g.updated_at,
		(SELECT COUNT(*) FROM group_participants p WHERE p.group_jid = g.jid)
		FROM groups g WHERE g.jid = ?`, jid)
	g, err := scanGroup(row)
	if errors.Is(err, sql.ErrNoRows) {
		return g, fmt.Errorf("no group %s in the store; it syncs when the account receives a message from it", jid)
	}
	if err != nil {
		return g, describe(err)
	}
	name := `COALESCE(NULLIF(a.alias,''), NULLIF(c.system_name,''), NULLIF(c.full_name,''), NULLIF(c.push_name,''), NULLIF(c.business_name,''), '')`
	query := `SELECT p.user_jid, ` + name + `, COALESCE(c.phone,''), COALESCE(p.role,'') FROM group_participants p
		LEFT JOIN contacts c ON c.jid = p.user_jid LEFT JOIN contact_aliases a ON a.jid = p.user_jid WHERE p.group_jid = ?`
	args := []any{jid}
	if s := strings.TrimSpace(search); s != "" {
		like := "%" + strings.ToLower(s) + "%"
		query += ` AND (LOWER(` + name + `) LIKE ? OR p.user_jid LIKE ? OR c.phone LIKE ?)`
		args = append(args, like, like, like)
	}
	rows, err := x.db.QueryContext(ctx, query, args...)
	if err != nil {
		return g, describe(err)
	}
	defer rows.Close()
	for rows.Next() {
		var p Participant
		if err := rows.Scan(&p.JID, &p.Name, &p.Phone, &p.Role); err != nil {
			return g, err
		}
		g.Participants = append(g.Participants, p)
	}
	return g, rows.Err()
}

// describe turns the error of a store that was never created into advice.
func describe(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "unable to open") || strings.Contains(msg, "no such table") || strings.Contains(msg, "out of memory (14)") {
		return errors.New("the wacli store is empty: pair WhatsApp first with `wacli auth`")
	}
	return err
}
