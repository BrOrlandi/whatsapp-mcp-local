package index

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// Filter narrows the messages a count, a statistic or an export covers. The
// zero value covers every visible message.
type Filter struct {
	ChatJID string
	Since   time.Time
	Until   time.Time
	// Direction is "in" (received), "out" (sent by the account) or "".
	Direction string
	// MediaType is a wacli media type (image, video, audio, document,
	// sticker…), "text" for messages without media, "any" for any media, or "".
	MediaType     string
	ExcludeGroups bool
}

func (f Filter) where() (string, []any) {
	clauses := []string{visible}
	var args []any
	if f.ChatJID != "" {
		clauses = append(clauses, "m.chat_jid = ?")
		args = append(args, f.ChatJID)
	}
	if !f.Since.IsZero() {
		clauses = append(clauses, "m.ts >= ?")
		args = append(args, f.Since.Unix())
	}
	if !f.Until.IsZero() {
		clauses = append(clauses, "m.ts < ?")
		args = append(args, f.Until.Unix())
	}
	switch f.Direction {
	case "in":
		clauses = append(clauses, "m.from_me = 0")
	case "out":
		clauses = append(clauses, "m.from_me = 1")
	}
	switch f.MediaType {
	case "":
	case "text":
		clauses = append(clauses, "COALESCE(m.media_type,'') = ''")
	case "any":
		clauses = append(clauses, "COALESCE(m.media_type,'') != ''")
	default:
		clauses = append(clauses, "m.media_type = ?")
		args = append(args, f.MediaType)
	}
	if f.ExcludeGroups {
		clauses = append(clauses, "m.chat_jid NOT LIKE '%@g.us'")
	}
	return strings.Join(clauses, " AND "), args
}

// CountMessages counts the messages a filter covers.
func (x *Index) CountMessages(ctx context.Context, f Filter) (int64, error) {
	where, args := f.where()
	var n int64
	err := x.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages m WHERE `+where, args...).Scan(&n)
	return n, describe(err)
}

// Bucket is one group of a message statistic.
type Bucket struct {
	Key   string    `json:"key"`
	Name  string    `json:"name,omitempty"`
	Count int64     `json:"count"`
	First time.Time `json:"first"`
	Last  time.Time `json:"last"`
}

// Stats counts messages grouped by chat, sender, day or month (days and months
// in this computer's time zone). Chats and senders come busiest first, days and
// months most recent first. It returns the total the filter covers and how many
// groups exist, which can exceed limit.
func (x *Index) Stats(ctx context.Context, f Filter, groupBy string, limit int) (buckets []Bucket, total int64, groups int, err error) {
	var key, order string
	switch groupBy {
	case "chat":
		key, order = "m.chat_jid", "COUNT(*) DESC"
	case "sender":
		key, order = "CASE WHEN m.from_me = 1 THEN 'me' ELSE COALESCE(m.sender_jid,'') END", "COUNT(*) DESC"
	case "day":
		key, order = "strftime('%Y-%m-%d', m.ts, 'unixepoch', 'localtime')", "k DESC"
	case "month":
		key, order = "strftime('%Y-%m', m.ts, 'unixepoch', 'localtime')", "k DESC"
	default:
		return nil, 0, 0, errInvalid("group_by must be chat, sender, day or month")
	}
	where, args := f.where()
	if err := x.db.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(DISTINCT `+key+`) FROM messages m WHERE `+where, args...).Scan(&total, &groups); err != nil {
		return nil, 0, 0, describe(err)
	}
	rows, err := x.db.QueryContext(ctx, `SELECT `+key+` AS k, COUNT(*), MIN(m.ts), MAX(m.ts) FROM messages m WHERE `+where+
		` GROUP BY k ORDER BY `+order+` LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, 0, 0, describe(err)
	}
	defer rows.Close()
	for rows.Next() {
		var b Bucket
		var first, last int64
		if err := rows.Scan(&b.Key, &b.Count, &first, &last); err != nil {
			return nil, 0, 0, err
		}
		b.First, b.Last = time.Unix(first, 0).UTC(), time.Unix(last, 0).UTC()
		buckets = append(buckets, b)
	}
	return buckets, total, groups, rows.Err()
}

// Row is one stored message, as the bulk reads return it.
type Row struct {
	ChatJID    string
	ChatName   string
	ID         string
	SenderJID  string
	SenderName string
	Timestamp  time.Time
	FromMe     bool
	Text       string
	MediaType  string
	MimeType   string
	Filename   string
	QuotedID   string
	ReactionTo string
	Reaction   string
	Forwarded  bool
	Edited     bool
	Revoked    bool
}

const rowColumns = `m.chat_jid, COALESCE(m.chat_name,''), m.msg_id, COALESCE(m.sender_jid,''), COALESCE(m.sender_name,''), m.ts, m.from_me,
	` + displayText + `, COALESCE(m.media_type,''), COALESCE(m.mime_type,''), COALESCE(m.filename,''), COALESCE(m.quoted_msg_id,''),
	COALESCE(m.reaction_to_id,''), COALESCE(m.reaction_emoji,''), m.is_forwarded, m.edited, m.revoked`

func scanRow(rows *sql.Rows) (Row, error) {
	var r Row
	var ts int64
	err := rows.Scan(&r.ChatJID, &r.ChatName, &r.ID, &r.SenderJID, &r.SenderName, &ts, &r.FromMe, &r.Text, &r.MediaType, &r.MimeType,
		&r.Filename, &r.QuotedID, &r.ReactionTo, &r.Reaction, &r.Forwarded, &r.Edited, &r.Revoked)
	r.Timestamp = time.Unix(ts, 0).UTC()
	return r, err
}

func (x *Index) rows(ctx context.Context, query string, args ...any) ([]Row, error) {
	rows, err := x.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, describe(err)
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		r, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// EachMessage calls fn for every message the filter covers, oldest first,
// streaming rather than loading them all.
func (x *Index) EachMessage(ctx context.Context, f Filter, fn func(Row) error) error {
	where, args := f.where()
	rows, err := x.db.QueryContext(ctx, `SELECT `+rowColumns+` FROM messages m WHERE `+where+` ORDER BY m.ts, m.rowid`, args...)
	if err != nil {
		return describe(err)
	}
	defer rows.Close()
	for rows.Next() {
		r, err := scanRow(rows)
		if err != nil {
			return err
		}
		if err := fn(r); err != nil {
			return err
		}
	}
	return rows.Err()
}

type invalidError string

func (e invalidError) Error() string { return string(e) }

func errInvalid(msg string) error { return invalidError(msg) }

// Canonical is the JID a person is best known by: the phone-number JID for a
// LID the session can map, the JID itself otherwise, without a device suffix.
func (n *Names) Canonical(ctx context.Context, jid string) string {
	n.refresh(ctx)
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.canonical(jid)
}
