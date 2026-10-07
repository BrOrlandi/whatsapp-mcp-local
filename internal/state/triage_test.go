package state

import (
	"context"
	"testing"
	"time"
)

func TestMarks(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)
	chat := "5511900000001@s.whatsapp.net"

	if err := st.MarkHandled(ctx, chat, "respondi por email", now); err != nil {
		t.Fatal(err)
	}
	m, ok, err := st.MarkOf(ctx, chat)
	if err != nil || !ok || m.Note != "respondi por email" {
		t.Fatalf("mark = %+v %v %v", m, ok, err)
	}
	if !m.Hides(now.Add(-time.Minute), now) {
		t.Error("handled after the last message should hide the chat")
	}
	if m.Hides(now.Add(time.Minute), now) {
		t.Error("a message after the mark should bring the chat back")
	}

	until := now.Add(24 * time.Hour)
	if err := st.Snooze(ctx, chat, "", now, until); err != nil {
		t.Fatal(err)
	}
	marks, _ := st.Marks(ctx)
	m = marks[chat]
	if m.Note != "respondi por email" {
		t.Errorf("a snooze without a note keeps the old one: %q", m.Note)
	}
	if !m.Hides(now.Add(-time.Minute), now) || m.Hides(now.Add(-time.Minute), until.Add(time.Second)) {
		t.Error("a snooze hides until its time")
	}
	if m.Hides(now.Add(time.Hour), now.Add(2*time.Hour)) {
		t.Error("a message during the snooze should bring the chat back")
	}

	if err := st.ClearMark(ctx, chat); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.MarkOf(ctx, chat); ok {
		t.Error("cleared mark still there")
	}
}
