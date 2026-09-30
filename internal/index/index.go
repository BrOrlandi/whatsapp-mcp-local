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

// Activity is how alive the store looks: when a message last arrived from
// someone else, and how many arrived recently.
type Activity struct {
	NewestIncoming *time.Time `json:"newest_incoming,omitempty"`
	NewestAny      *time.Time `json:"newest_any,omitempty"`
	Oldest         *time.Time `json:"history_since,omitempty"`
	LastHour       int64      `json:"messages_last_hour"`
	LastDay        int64      `json:"messages_last_24h"`
}

// Activity measures recent traffic. Incoming messages are what prove the
// device is receiving: the account's own sends reach the store through sync
// too, but a send can be recorded locally before WhatsApp delivers anything.
func (x *Index) Activity(ctx context.Context, now time.Time) (Activity, error) {
	var a Activity
	var incoming, any, oldest sql.NullInt64
	err := x.db.QueryRowContext(ctx, `SELECT
		(SELECT MAX(ts) FROM messages WHERE from_me = 0),
		(SELECT MAX(ts) FROM messages),
		(SELECT MIN(ts) FROM messages),
		(SELECT COUNT(*) FROM messages WHERE ts >= ?),
		(SELECT COUNT(*) FROM messages WHERE ts >= ?)`,
		now.Add(-time.Hour).Unix(), now.Add(-24*time.Hour).Unix()).Scan(&incoming, &any, &oldest, &a.LastHour, &a.LastDay)
	if err != nil {
		return a, describe(err)
	}
	if incoming.Valid {
		t := time.Unix(incoming.Int64, 0).UTC()
		a.NewestIncoming = &t
	}
	if any.Valid {
		t := time.Unix(any.Int64, 0).UTC()
		a.NewestAny = &t
	}
	if oldest.Valid {
		t := time.Unix(oldest.Int64, 0).UTC()
		a.Oldest = &t
	}
	return a, nil
}

// ChatPreview is one row of the chat list the panel draws like WhatsApp's.
type ChatPreview struct {
	JID         string    `json:"jid"`
	Name        string    `json:"name"`
	Kind        string    `json:"kind"`
	Unread      int       `json:"unread"`
	Pinned      bool      `json:"pinned,omitempty"`
	At          time.Time `json:"at"`
	LastFromMe  bool      `json:"last_from_me"`
	LastSender  string    `json:"last_sender,omitempty"`
	LastText    string    `json:"last_text"`
	LastMedia   string    `json:"last_media,omitempty"`
	OldestKnown time.Time `json:"oldest_known"`
	Messages    int64     `json:"messages"`
}

// displayText is what a message shows in a preview: its text, its caption, or
// the kind of media it carries.
const displayText = `COALESCE(NULLIF(m.media_caption,''), NULLIF(m.text,''), NULLIF(m.display_text,''), '')`

// chatName resolves a conversation's name the way the WhatsApp app would:
// the stored chat name, then the contact's, then the number.
const chatName = `COALESCE(NULLIF(c.name,''), NULLIF(a.alias,''), NULLIF(ct.system_name,''), NULLIF(ct.full_name,''), NULLIF(ct.push_name,''), NULLIF(ct.business_name,''), '')`

// RecentChats lists the conversations with the most recent real messages,
// each with its last one. Reactions and content-free system rows are skipped,
// so the preview matches what the phone's chat list shows.
func (x *Index) RecentChats(ctx context.Context, limit int) ([]ChatPreview, error) {
	rows, err := x.db.QueryContext(ctx, `
		WITH last AS (
			SELECT m.chat_jid, MAX(m.ts) AS ts FROM messages m
			WHERE m.deleted_at IS NULL AND COALESCE(m.reaction_to_id,'') = '' AND (`+displayText+` != '' OR COALESCE(m.media_type,'') != '')
			  AND m.chat_jid NOT LIKE '%@broadcast' AND m.chat_jid NOT LIKE '%@newsletter'
			GROUP BY m.chat_jid ORDER BY ts DESC LIMIT ?)
		SELECT l.chat_jid, `+chatName+`, COALESCE(c.kind,''), COALESCE(c.unread_count,0), COALESCE(c.pinned,0), l.ts,
			m.from_me, COALESCE(NULLIF(m.sender_name,''), ''), `+displayText+`, COALESCE(m.media_type,''),
			(SELECT MIN(ts) FROM messages WHERE chat_jid = l.chat_jid), (SELECT COUNT(*) FROM messages WHERE chat_jid = l.chat_jid)
		FROM last l
		JOIN messages m ON m.rowid = (SELECT rowid FROM messages WHERE chat_jid = l.chat_jid AND ts = l.ts AND deleted_at IS NULL ORDER BY rowid DESC LIMIT 1)
		LEFT JOIN chats c ON c.jid = l.chat_jid
		LEFT JOIN contacts ct ON ct.jid = l.chat_jid
		LEFT JOIN contact_aliases a ON a.jid = l.chat_jid
		ORDER BY l.ts DESC`, limit)
	if err != nil {
		return nil, describe(err)
	}
	defer rows.Close()
	var out []ChatPreview
	for rows.Next() {
		var p ChatPreview
		var ts, oldest int64
		if err := rows.Scan(&p.JID, &p.Name, &p.Kind, &p.Unread, &p.Pinned, &ts, &p.LastFromMe, &p.LastSender, &p.LastText, &p.LastMedia, &oldest, &p.Messages); err != nil {
			return nil, err
		}
		p.At, p.OldestKnown = time.Unix(ts, 0).UTC(), time.Unix(oldest, 0).UTC()
		if p.Name == "" {
			p.Name = phoneOf(p.JID)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Bubble is one message as the panel draws it.
type Bubble struct {
	ID       string    `json:"id"`
	ChatJID  string    `json:"chat_jid"`
	ChatName string    `json:"chat_name,omitempty"`
	FromMe   bool      `json:"from_me"`
	Sender   string    `json:"sender,omitempty"`
	At       time.Time `json:"at"`
	Text     string    `json:"text"`
	Media    string    `json:"media,omitempty"`
	Group    bool      `json:"group,omitempty"`
}

func (x *Index) bubbles(ctx context.Context, where string, args ...any) ([]Bubble, error) {
	rows, err := x.db.QueryContext(ctx, `SELECT m.msg_id, m.chat_jid, `+chatName+`, m.from_me,
			COALESCE(NULLIF(m.sender_name,''), NULLIF(sa.alias,''), NULLIF(sc.system_name,''), NULLIF(sc.full_name,''), NULLIF(sc.push_name,''), ''),
			m.ts, `+displayText+`, COALESCE(m.media_type,'')
		FROM messages m
		LEFT JOIN chats c ON c.jid = m.chat_jid
		LEFT JOIN contacts ct ON ct.jid = m.chat_jid
		LEFT JOIN contact_aliases a ON a.jid = m.chat_jid
		LEFT JOIN contacts sc ON sc.jid = m.sender_jid
		LEFT JOIN contact_aliases sa ON sa.jid = m.sender_jid
		WHERE m.deleted_at IS NULL AND COALESCE(m.reaction_to_id,'') = '' AND (`+displayText+` != '' OR COALESCE(m.media_type,'') != '') AND `+where, args...)
	if err != nil {
		return nil, describe(err)
	}
	defer rows.Close()
	var out []Bubble
	for rows.Next() {
		var b Bubble
		var ts int64
		if err := rows.Scan(&b.ID, &b.ChatJID, &b.ChatName, &b.FromMe, &b.Sender, &ts, &b.Text, &b.Media); err != nil {
			return nil, err
		}
		b.At = time.Unix(ts, 0).UTC()
		b.Group = strings.HasSuffix(b.ChatJID, "@g.us")
		if b.ChatName == "" {
			b.ChatName = phoneOf(b.ChatJID)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// RecentIncoming is the latest messages others sent, across every chat: what
// the phone's notifications would have shown.
func (x *Index) RecentIncoming(ctx context.Context, limit int) ([]Bubble, error) {
	return x.bubbles(ctx, `m.from_me = 0 AND m.chat_jid NOT LIKE '%@broadcast' AND m.chat_jid NOT LIKE '%@newsletter' ORDER BY m.ts DESC LIMIT ?`, limit)
}

// Conversation is the latest messages of one chat, oldest first.
func (x *Index) Conversation(ctx context.Context, jid string, limit int) ([]Bubble, error) {
	b, err := x.bubbles(ctx, `m.chat_jid = ? ORDER BY m.ts DESC LIMIT ?`, jid, limit)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return b, err
}

// OwnName is the push name of the paired account, when WhatsApp has told it.
func (x *Index) OwnName(ctx context.Context, jid string) string {
	user := strings.SplitN(jid, "@", 2)[0]
	user = strings.SplitN(user, ":", 2)[0]
	var name string
	_ = x.db.QueryRowContext(ctx, `SELECT COALESCE(NULLIF(full_name,''), NULLIF(push_name,''), '') FROM contacts WHERE jid = ?`,
		user+"@s.whatsapp.net").Scan(&name)
	return name
}

func phoneOf(jid string) string {
	user := strings.SplitN(jid, "@", 2)[0]
	if strings.HasSuffix(jid, "@s.whatsapp.net") {
		return "+" + user
	}
	return user
}
