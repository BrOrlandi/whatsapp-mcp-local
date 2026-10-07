package index

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// ChatState is a chat's row in wacli's chat list: what the phone shows about it.
type ChatState struct {
	JID          string
	Unread       int
	MarkedUnread bool
	Archived     bool
	Muted        bool
	Pinned       bool
	LastMessage  time.Time
}

func scanChatState(row interface{ Scan(...any) error }, now time.Time) (ChatState, error) {
	var c ChatState
	var muted, last int64
	var unreadFlag bool
	err := row.Scan(&c.JID, &c.Unread, &unreadFlag, &c.Archived, &muted, &c.Pinned, &last)
	c.MarkedUnread = unreadFlag && c.Unread == 0
	c.Muted = muted == -1 || muted > now.Unix()
	if last > 0 {
		c.LastMessage = time.Unix(last, 0).UTC()
	}
	return c, err
}

const chatStateColumns = `jid, unread_count, unread, archived, muted_until, pinned, COALESCE(last_message_ts,0)`

// UnreadChats lists the chats the phone shows as unread, most recent first:
// with an unread count, or marked as unread by hand. The phone keeps the count;
// reading a chat there clears it here too.
func (x *Index) UnreadChats(ctx context.Context, includeArchived bool, limit int) ([]ChatState, error) {
	query := `SELECT ` + chatStateColumns + ` FROM chats WHERE (unread_count > 0 OR unread = 1)
		AND jid NOT LIKE '%@broadcast' AND jid NOT LIKE '%@newsletter'`
	if !includeArchived {
		query += ` AND archived = 0`
	}
	query += ` ORDER BY last_message_ts DESC LIMIT ?`
	rows, err := x.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, describe(err)
	}
	defer rows.Close()
	now := time.Now()
	var out []ChatState
	for rows.Next() {
		c, err := scanChatState(rows, now)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Chat is one chat's state, or ok false when the chat list has no row for it.
func (x *Index) Chat(ctx context.Context, jid string) (ChatState, bool) {
	c, err := scanChatState(x.db.QueryRowContext(ctx, `SELECT `+chatStateColumns+` FROM chats WHERE jid = ?`, jid), time.Now())
	return c, err == nil
}

// Incoming is the latest messages of one chat sent by others, newest first.
func (x *Index) Incoming(ctx context.Context, chat string, n int) ([]Row, error) {
	return x.rows(ctx, `SELECT `+rowColumns+` FROM messages m WHERE m.chat_jid = ? AND m.from_me = 0 AND `+visible+`
		ORDER BY m.ts DESC, m.rowid DESC LIMIT ?`, chat, n)
}

// LastMessages is the newest visible message of every chat active since a
// moment, newest chat first: what decides whether a chat waits for a reply.
func (x *Index) LastMessages(ctx context.Context, since time.Time) ([]Row, error) {
	return x.rows(ctx, `SELECT `+rowColumns+` FROM (
			SELECT m.*, ROW_NUMBER() OVER (PARTITION BY m.chat_jid ORDER BY m.ts DESC, m.rowid DESC) AS rn
			FROM messages m WHERE m.ts >= ? AND `+visible+`
		) m WHERE m.rn = 1 ORDER BY m.ts DESC`, since.Unix())
}

// Waiting counts what others wrote in a chat after the account's last message
// there, and when the first of it arrived.
func (x *Index) Waiting(ctx context.Context, chat string) (count int, since time.Time, err error) {
	var first sql.NullInt64
	err = x.db.QueryRowContext(ctx, `SELECT COUNT(*), MIN(m.ts) FROM messages m WHERE m.chat_jid = ? AND m.from_me = 0 AND `+visible+`
		AND m.ts > COALESCE((SELECT MAX(ts) FROM messages WHERE chat_jid = ? AND from_me = 1), 0)`, chat, chat).Scan(&count, &first)
	if first.Valid {
		since = time.Unix(first.Int64, 0).UTC()
	}
	return count, since, describe(err)
}

// LastSent is when the account last wrote in a chat.
func (x *Index) LastSent(ctx context.Context, chat string) time.Time {
	var ts sql.NullInt64
	_ = x.db.QueryRowContext(ctx, `SELECT MAX(ts) FROM messages WHERE chat_jid = ? AND from_me = 1`, chat).Scan(&ts)
	if !ts.Valid {
		return time.Time{}
	}
	return time.Unix(ts.Int64, 0).UTC()
}

// Mentions finds messages from others that mention the account, newest first.
// wacli keeps no list of who a message mentions, but WhatsApp writes every
// mention into the text as @ followed by the person's number or LID, so the
// text is searched for those. ids are the account's bare phone number and LID.
func (x *Index) Mentions(ctx context.Context, ids []string, chat string, since time.Time, limit int) ([]Row, error) {
	var likes []string
	var args []any
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			continue
		}
		likes = append(likes, `m.text LIKE ?`, `m.media_caption LIKE ?`)
		args = append(args, "%@"+id+"%", "%@"+id+"%")
	}
	if len(likes) == 0 {
		return nil, nil
	}
	query := `SELECT ` + rowColumns + ` FROM messages m WHERE m.from_me = 0 AND m.ts >= ? AND ` + visible + ` AND (` + strings.Join(likes, " OR ") + `)`
	args = append([]any{since.Unix()}, args...)
	if chat != "" {
		query += ` AND m.chat_jid = ?`
		args = append(args, chat)
	}
	query += ` ORDER BY m.ts DESC LIMIT ?`
	args = append(args, limit)
	return x.rows(ctx, query, args...)
}
