package index

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// schema is the part of wacli's store these queries read.
const schema = `
CREATE TABLE chats (jid TEXT PRIMARY KEY, kind TEXT NOT NULL, name TEXT, last_message_ts INTEGER, archived INTEGER NOT NULL DEFAULT 0,
	pinned INTEGER NOT NULL DEFAULT 0, muted_until INTEGER NOT NULL DEFAULT 0, unread INTEGER NOT NULL DEFAULT 0, unread_count INTEGER NOT NULL DEFAULT 0);
CREATE TABLE contacts (jid TEXT PRIMARY KEY, phone TEXT, push_name TEXT, full_name TEXT, first_name TEXT, business_name TEXT, system_name TEXT, updated_at INTEGER NOT NULL);
CREATE TABLE groups (jid TEXT PRIMARY KEY, name TEXT, owner_jid TEXT, created_ts INTEGER, is_parent INTEGER NOT NULL DEFAULT 0, linked_parent_jid TEXT, left_at INTEGER, updated_at INTEGER NOT NULL);
CREATE TABLE group_participants (group_jid TEXT NOT NULL, user_jid TEXT NOT NULL, role TEXT, updated_at INTEGER NOT NULL, PRIMARY KEY (group_jid, user_jid));
CREATE TABLE contact_aliases (jid TEXT PRIMARY KEY, alias TEXT NOT NULL, notes TEXT, updated_at INTEGER NOT NULL);
CREATE TABLE messages (rowid INTEGER PRIMARY KEY AUTOINCREMENT, chat_jid TEXT NOT NULL, chat_name TEXT, msg_id TEXT NOT NULL, sender_jid TEXT, sender_name TEXT,
	ts INTEGER NOT NULL, from_me INTEGER NOT NULL, text TEXT, display_text TEXT, quoted_msg_id TEXT, quoted_sender_jid TEXT,
	is_forwarded INTEGER NOT NULL DEFAULT 0, forwarding_score INTEGER NOT NULL DEFAULT 0, reaction_to_id TEXT, reaction_emoji TEXT,
	media_type TEXT, media_caption TEXT, filename TEXT, mime_type TEXT, direct_path TEXT, local_path TEXT, revoked INTEGER NOT NULL DEFAULT 0,
	deleted_for_me INTEGER NOT NULL DEFAULT 0, deleted_at INTEGER, edited INTEGER NOT NULL DEFAULT 0, UNIQUE(chat_jid, msg_id));`

const (
	mae     = "5511900000001@s.whatsapp.net"
	lucas   = "5511900000002@s.whatsapp.net"
	familia = "120363000000000001@g.us"
	me      = "5511999999999"
)

// store builds a wacli store in a temporary folder and opens it the way the
// daemon does, read-only.
func store(t *testing.T, now time.Time) *Index {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wacli.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	ago := func(d time.Duration) int64 { return now.Add(-d).Unix() }
	exec := func(q string, args ...any) {
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO chats (jid, kind, name, last_message_ts, unread_count) VALUES (?, 'dm', 'Mãe', ?, 2), (?, 'dm', 'Lucas', ?, 0), (?, 'group', 'Família', ?, 3)`,
		mae, ago(time.Hour), lucas, ago(2*time.Hour), familia, ago(30*time.Minute))
	msg := func(chat, id, sender string, d time.Duration, fromMe bool, text, media, reactionTo string) {
		exec(`INSERT INTO messages (chat_jid, msg_id, sender_jid, ts, from_me, text, media_type, reaction_to_id) VALUES (?, ?, ?, ?, ?, ?, NULLIF(?,''), NULLIF(?,''))`,
			chat, id, sender, ago(d), fromMe, text, media, reactionTo)
	}
	// Mãe asked, I answered, she wrote twice more: waiting.
	msg(mae, "M1", mae, 5*time.Hour, false, "Vem almoçar domingo?", "", "")
	msg(mae, "M2", me+"@s.whatsapp.net", 4*time.Hour, true, "Vou sim", "", "")
	msg(mae, "M3", mae, 2*time.Hour, false, "Traz a sobremesa", "", "")
	msg(mae, "M4", mae, time.Hour, false, "E o refri", "", "")
	msg(mae, "R1", mae, 50*time.Minute, false, "", "", "M2") // a reaction: not a message
	// Lucas ended it with a thanks: not waiting once closings are ignored.
	msg(lucas, "L1", me+"@s.whatsapp.net", 3*time.Hour, true, "Te mandei o arquivo", "", "")
	msg(lucas, "L2", lucas, 2*time.Hour, false, "obrigado!", "", "")
	// Família: someone mentions me after my last message.
	msg(familia, "F1", me+"@s.whatsapp.net", 3*time.Hour, true, "Bom dia", "", "")
	msg(familia, "F2", lucas, 2*time.Hour, false, "Foto", "image", "")
	msg(familia, "F3", mae, 30*time.Minute, false, fmt.Sprintf("@%s você vem?", me), "", "")
	x, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { x.Close() })
	return x
}

func TestStatsAndCounts(t *testing.T) {
	now := time.Now()
	x := store(t, now)
	ctx := context.Background()
	n, err := x.CountMessages(ctx, Filter{ChatJID: mae})
	if err != nil || n != 4 {
		t.Fatalf("count = %d, %v; want 4 (the reaction left out)", n, err)
	}
	if n, _ := x.CountMessages(ctx, Filter{Direction: "out"}); n != 3 {
		t.Errorf("sent = %d, want 3", n)
	}
	if n, _ := x.CountMessages(ctx, Filter{MediaType: "image"}); n != 1 {
		t.Errorf("images = %d, want 1", n)
	}
	if n, _ := x.CountMessages(ctx, Filter{ExcludeGroups: true}); n != 6 {
		t.Errorf("outside groups = %d, want 6", n)
	}
	if n, _ := x.CountMessages(ctx, Filter{Since: now.Add(-150 * time.Minute)}); n != 5 {
		t.Errorf("since 2h30 = %d, want 5", n)
	}
	buckets, total, groups, err := x.Stats(ctx, Filter{}, "chat", 2)
	if err != nil {
		t.Fatal(err)
	}
	if total != 9 || groups != 3 || len(buckets) != 2 || buckets[0].Key != mae || buckets[0].Count != 4 {
		t.Errorf("by chat: %+v total %d groups %d", buckets, total, groups)
	}
	buckets, _, _, err = x.Stats(ctx, Filter{}, "sender", 10)
	if err != nil || buckets[0].Key != mae {
		t.Errorf("by sender: %+v %v", buckets, err)
	}
	if _, _, _, err := x.Stats(ctx, Filter{}, "day", 10); err != nil {
		t.Errorf("by day: %v", err)
	}
	if _, _, _, err := x.Stats(ctx, Filter{}, "week", 10); err == nil {
		t.Error("an unknown grouping should be refused")
	}
	var ids []string
	_ = x.EachMessage(ctx, Filter{ChatJID: mae}, func(r Row) error { ids = append(ids, r.ID); return nil })
	if fmt.Sprint(ids) != "[M1 M2 M3 M4]" {
		t.Errorf("export order = %v", ids)
	}
}

func TestTriageQueries(t *testing.T) {
	now := time.Now()
	x := store(t, now)
	ctx := context.Background()
	last, err := x.LastMessages(ctx, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	byChat := map[string]Row{}
	for _, r := range last {
		byChat[r.ChatJID] = r
	}
	if byChat[mae].ID != "M4" || byChat[lucas].ID != "L2" || byChat[familia].ID != "F3" {
		t.Errorf("latest per chat: %+v", byChat)
	}
	count, since, err := x.Waiting(ctx, mae)
	if err != nil || count != 2 || since.Unix() != now.Add(-2*time.Hour).Unix() {
		t.Errorf("waiting = %d since %v, %v", count, since, err)
	}
	mentions, err := x.Mentions(ctx, []string{me, "123456789012345"}, "", now.Add(-24*time.Hour), 10)
	if err != nil || len(mentions) != 1 || mentions[0].ID != "F3" {
		t.Errorf("mentions = %+v, %v", mentions, err)
	}
	if got := x.LastSent(ctx, familia); got.Unix() != now.Add(-3*time.Hour).Unix() {
		t.Errorf("last sent = %v", got)
	}
	unread, err := x.UnreadChats(ctx, false, 10)
	if err != nil || len(unread) != 2 || unread[0].JID != familia || unread[0].Unread != 3 {
		t.Errorf("unread = %+v, %v", unread, err)
	}
	in, err := x.Incoming(ctx, mae, 2)
	if err != nil || len(in) != 2 || in[0].ID != "M4" {
		t.Errorf("incoming = %+v, %v", in, err)
	}
	if !x.ChatKnown(ctx, mae) || x.ChatKnown(ctx, "5511000000000@s.whatsapp.net") {
		t.Error("ChatKnown is wrong")
	}
}
