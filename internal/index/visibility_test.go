package index

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestParserFallbackDoesNotCreateVisibleMessages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wacli.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	self := me + "@s.whatsapp.net"
	for _, row := range []struct {
		chat, id             string
		ts                   int
		text, caption, media any
		display              string
	}{
		{self, "self-real", 100, "Anotação", nil, nil, "Anotação"},
		{self, "self-event", 300, nil, nil, nil, "(message)"},
		{mae, "incoming-real", 200, "Oi", nil, nil, "Oi"},
		{mae, "incoming-event", 400, "", "", "", "(message)"},
		{lucas, "only-event", 500, nil, nil, nil, "(message)"},
		{familia, "group-event", 600, nil, nil, nil, "(message)"},
		// A person can actually type the fallback string. Media and other
		// recognized display text must also survive without raw text.
		{self, "literal-text", 80, "(message)", nil, nil, "(message)"},
		{self, "literal-caption", 70, nil, "(message)", "image", "(message)"},
		{self, "media", 60, nil, nil, "image", "(message)"},
		{self, "recognized-display", 50, nil, nil, nil, "[location]"},
	} {
		_, err := db.Exec(`INSERT INTO messages (chat_jid, msg_id, ts, from_me, text, media_caption, media_type, display_text)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, row.chat, row.id, row.ts, row.chat == self, row.text, row.caption, row.media, row.display)
		if err != nil {
			t.Fatal(err)
		}
	}
	x, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	ctx := context.Background()
	// Filter before LIMIT and sorting: the empty events must neither add
	// chats nor make an existing chat look more recent.
	chats, err := x.RecentChats(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 2 || chats[0].JID != mae || chats[0].LastText != "Oi" || chats[0].At.Unix() != 200 ||
		chats[1].JID != self || chats[1].LastText != "Anotação" || chats[1].At.Unix() != 100 {
		t.Fatalf("unexpected chat previews: %+v", chats)
	}
	all, err := x.RecentChats(ctx, 100)
	if err != nil || len(all) != 2 {
		t.Fatalf("placeholder-only chats should be absent: %+v, %v", all, err)
	}
	bubbles, err := x.Conversation(ctx, self, 100)
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []string{"recognized-display", "media", "literal-caption", "literal-text", "self-real"}
	wantTexts := []string{"[location]", "", "(message)", "(message)", "Anotação"}
	if len(bubbles) != len(wantIDs) {
		t.Fatalf("unexpected conversation: %+v", bubbles)
	}
	for i, b := range bubbles {
		if b.ID != wantIDs[i] || b.Text != wantTexts[i] {
			t.Errorf("bubble %d = %+v; want %s with text %q", i, b, wantIDs[i], wantTexts[i])
		}
	}
	for _, jid := range []string{lucas, familia} {
		bubbles, err := x.Conversation(ctx, jid, 100)
		if err != nil || len(bubbles) != 0 {
			t.Errorf("placeholder-only conversation = %+v, %v", bubbles, err)
		}
	}
	count, err := x.CountMessages(ctx, Filter{})
	if err != nil || count != 6 {
		t.Errorf("visible count = %d, %v; want 6", count, err)
	}
	// The filter never removes evidence from the index.
	if _, err := x.MessageByID(ctx, "self-event", self); err != nil {
		t.Errorf("placeholder should remain available by ID: %v", err)
	}
}
