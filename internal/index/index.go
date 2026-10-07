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
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Index struct {
	db    *sql.DB
	Names *Names
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
	return &Index{db: db, Names: openNames(db, filepath.Join(filepath.Dir(path), "session.db"))}, nil
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
	Group       bool      `json:"group"`
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

// visible keeps out what the phone's chat list does not show: reactions,
// content-free system rows, status broadcasts and channels.
const visible = `m.deleted_at IS NULL AND COALESCE(m.reaction_to_id,'') = '' AND (` + displayText + ` != '' OR COALESCE(m.media_type,'') != '')
	AND m.chat_jid NOT LIKE '%@broadcast' AND m.chat_jid NOT LIKE '%@newsletter'`

// RecentChats lists the conversations with the most recent real messages,
// each with its last one, named the way the phone would name them.
func (x *Index) RecentChats(ctx context.Context, limit int) ([]ChatPreview, error) {
	rows, err := x.db.QueryContext(ctx, `
		WITH last AS (SELECT m.chat_jid, MAX(m.ts) AS ts FROM messages m WHERE `+visible+` GROUP BY m.chat_jid ORDER BY ts DESC LIMIT ?)
		SELECT l.chat_jid, COALESCE(c.unread_count,0), COALESCE(c.pinned,0), l.ts,
			m.from_me, COALESCE(m.sender_jid,''), COALESCE(m.sender_name,''), `+displayText+`, COALESCE(m.media_type,''),
			(SELECT MIN(ts) FROM messages WHERE chat_jid = l.chat_jid), (SELECT COUNT(*) FROM messages WHERE chat_jid = l.chat_jid)
		FROM last l
		JOIN messages m ON m.rowid = (SELECT rowid FROM messages m WHERE m.chat_jid = l.chat_jid AND m.ts = l.ts AND `+visible+` ORDER BY rowid DESC LIMIT 1)
		LEFT JOIN chats c ON c.jid = l.chat_jid
		ORDER BY l.ts DESC`, limit)
	if err != nil {
		return nil, describe(err)
	}
	defer rows.Close()
	var out []ChatPreview
	for rows.Next() {
		var p ChatPreview
		var ts, oldest int64
		var senderJID, senderName string
		if err := rows.Scan(&p.JID, &p.Unread, &p.Pinned, &ts, &p.LastFromMe, &senderJID, &senderName, &p.LastText, &p.LastMedia, &oldest, &p.Messages); err != nil {
			return nil, err
		}
		p.At, p.OldestKnown = time.Unix(ts, 0).UTC(), time.Unix(oldest, 0).UTC()
		p.Group = strings.HasSuffix(p.JID, "@g.us")
		p.Name = x.Names.Name(ctx, p.JID)
		if p.Group && !p.LastFromMe {
			p.LastSender = x.sender(ctx, senderJID, senderName)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// sender names whoever wrote a message: the address book first, then what
// they called themselves.
func (x *Index) sender(ctx context.Context, jid, stored string) string {
	if name := x.Names.Known(ctx, jid); name != "" {
		return name
	}
	if !isJIDLike(stored) {
		return stored
	}
	if jid == "" {
		return ""
	}
	return x.Names.Name(ctx, jid)
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

// Conversation is the latest messages of one chat, oldest first.
func (x *Index) Conversation(ctx context.Context, jid string, limit int) ([]Bubble, error) {
	rows, err := x.db.QueryContext(ctx, `SELECT m.msg_id, m.from_me, COALESCE(m.sender_jid,''), COALESCE(m.sender_name,''), m.ts, `+displayText+`, COALESCE(m.media_type,'')
		FROM messages m WHERE m.chat_jid = ? AND `+visible+` ORDER BY m.ts DESC, m.rowid DESC LIMIT ?`, jid, limit)
	if err != nil {
		return nil, describe(err)
	}
	defer rows.Close()
	group := strings.HasSuffix(jid, "@g.us")
	chatName := x.Names.Name(ctx, jid)
	var out []Bubble
	for rows.Next() {
		b := Bubble{ChatJID: jid, ChatName: chatName, Group: group}
		var ts int64
		var senderJID, senderName string
		if err := rows.Scan(&b.ID, &b.FromMe, &senderJID, &senderName, &ts, &b.Text, &b.Media); err != nil {
			return nil, err
		}
		b.At = time.Unix(ts, 0).UTC()
		if !b.FromMe {
			b.Sender = x.sender(ctx, senderJID, senderName)
		}
		out = append(out, b)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}

// OwnName is the push name of the paired account, when WhatsApp has told it.
func (x *Index) OwnName(ctx context.Context, jid string) string {
	if name := x.Names.Self(ctx); name != "" {
		return name
	}
	return x.Names.Known(ctx, jid)
}

// ContextLine is one text message near a voice note, said by whom.
type ContextLine struct {
	At     time.Time `json:"at"`
	Sender string    `json:"sender"`
	Text   string    `json:"text"`
}

// AudioContext is the conversation around a voice note: its name, who spoke
// in it, and the text messages just before and just after. It is what lets a
// transcript be checked against what the chat was talking about.
type AudioContext struct {
	ChatName string        `json:"chat_name"`
	Speaker  string        `json:"speaker,omitempty"`
	People   []string      `json:"people,omitempty"`
	Before   []ContextLine `json:"before,omitempty"`
	After    []ContextLine `json:"after,omitempty"`
}

func (x *Index) AudioContext(ctx context.Context, chatJID, msgID string, before, after int) (AudioContext, error) {
	out := AudioContext{ChatName: x.Names.Name(ctx, chatJID)}
	var ts int64
	var fromMe bool
	var senderJID, senderName string
	err := x.db.QueryRowContext(ctx, `SELECT ts, from_me, COALESCE(sender_jid,''), COALESCE(sender_name,'') FROM messages WHERE chat_jid = ? AND msg_id = ?`,
		chatJID, msgID).Scan(&ts, &fromMe, &senderJID, &senderName)
	if err != nil {
		return out, describe(err)
	}
	if !fromMe {
		out.Speaker = x.sender(ctx, senderJID, senderName)
	}
	read := func(query string, args ...any) ([]ContextLine, error) {
		rows, err := x.db.QueryContext(ctx, `SELECT m.ts, m.from_me, COALESCE(m.sender_jid,''), COALESCE(m.sender_name,''), `+displayText+`
			FROM messages m WHERE m.chat_jid = ? AND `+visible+` AND COALESCE(m.media_type,'') = '' AND `+query, args...)
		if err != nil {
			return nil, describe(err)
		}
		defer rows.Close()
		var lines []ContextLine
		for rows.Next() {
			var l ContextLine
			var t int64
			var me bool
			var sj, sn string
			if err := rows.Scan(&t, &me, &sj, &sn, &l.Text); err != nil {
				return nil, err
			}
			l.At = time.Unix(t, 0).UTC()
			if me {
				l.Sender = "Eu"
			} else {
				l.Sender = x.sender(ctx, sj, sn)
			}
			lines = append(lines, l)
		}
		return lines, rows.Err()
	}
	if out.Before, err = read(`m.ts <= ? AND m.msg_id != ? ORDER BY m.ts DESC LIMIT ?`, chatJID, ts, msgID, before); err != nil {
		return out, err
	}
	for i, j := 0, len(out.Before)-1; i < j; i, j = i+1, j-1 {
		out.Before[i], out.Before[j] = out.Before[j], out.Before[i]
	}
	if out.After, err = read(`m.ts > ? ORDER BY m.ts ASC LIMIT ?`, chatJID, ts, after); err != nil {
		return out, err
	}
	seen := map[string]bool{}
	for _, l := range append(append([]ContextLine{}, out.Before...), out.After...) {
		if l.Sender != "Eu" && !seen[l.Sender] {
			seen[l.Sender] = true
			out.People = append(out.People, l.Sender)
		}
	}
	return out, nil
}

// Prompt is the context as Whisper's initial prompt: the names first, since
// they are what speech recognition gets wrong most, then the latest lines,
// kept within the few hundred characters Whisper reads.
func (c AudioContext) Prompt() string {
	var b strings.Builder
	names := append([]string{c.ChatName}, c.People...)
	if c.Speaker != "" {
		names = append(names, c.Speaker)
	}
	b.WriteString(strings.Join(names, ", "))
	b.WriteString(". ")
	var tail []string
	size := 0
	for i := len(c.Before) - 1; i >= 0 && size < 600; i-- {
		t := strings.Join(strings.Fields(c.Before[i].Text), " ")
		tail = append([]string{t}, tail...)
		size += len(t)
	}
	b.WriteString(strings.Join(tail, " "))
	p := b.String()
	if len(p) > 800 {
		p = p[len(p)-800:]
	}
	return p
}

// AudioMessage is a voice note waiting to be transcribed.
type AudioMessage struct {
	ChatJID string
	ID      string
	At      time.Time
}

// RecentAudio lists the voice notes since a moment, newest first.
func (x *Index) RecentAudio(ctx context.Context, since time.Time, limit int) ([]AudioMessage, error) {
	rows, err := x.db.QueryContext(ctx, `SELECT chat_jid, msg_id, ts FROM messages
		WHERE media_type = 'audio' AND deleted_at IS NULL AND ts >= ? AND COALESCE(direct_path,'') != ''
		ORDER BY ts DESC LIMIT ?`, since.Unix(), limit)
	if err != nil {
		return nil, describe(err)
	}
	defer rows.Close()
	var out []AudioMessage
	for rows.Next() {
		var a AudioMessage
		var ts int64
		if err := rows.Scan(&a.ChatJID, &a.ID, &ts); err != nil {
			return nil, err
		}
		a.At = time.Unix(ts, 0).UTC()
		out = append(out, a)
	}
	return out, rows.Err()
}

// ChatKnown reports whether the store holds a chat with that JID.
func (x *Index) ChatKnown(ctx context.Context, jid string) bool {
	var one int
	return x.db.QueryRowContext(ctx, `SELECT 1 FROM chats WHERE jid = ? UNION SELECT 1 FROM messages WHERE chat_jid = ? LIMIT 1`, jid, jid).Scan(&one) == nil
}
