package mcp

import (
	"context"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/index"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

// preview is one message as the triage lists show it: enough to decide, not
// the whole conversation.
type preview struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Sender    string    `json:"sender,omitempty"`
	SenderJID string    `json:"sender_jid,omitempty"`
	Text      string    `json:"text,omitempty"`
	MediaType string    `json:"media_type,omitempty"`
}

func (s *Server) preview(ctx context.Context, r index.Row) preview {
	m := rowMessage(r)
	s.name(ctx, &m)
	p := preview{ID: m.ID, Timestamp: m.Timestamp, Sender: m.SenderName, SenderJID: m.SenderJID, Text: clip(m.Text, 300), MediaType: m.MediaType}
	if m.MediaType == "audio" {
		if t, err := s.state.Transcript(ctx, m.ChatJID, m.ID); err == nil && p.Text == "" {
			p.Text = clip("[transcript] "+t.Text, 300)
		}
	}
	return p
}

func (s *Server) marks(ctx context.Context) map[string]state.Mark {
	marks, err := s.state.Marks(ctx)
	if err != nil {
		return map[string]state.Mark{}
	}
	return marks
}

func (s *Server) listUnread(ctx context.Context, a arguments) map[string]any {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	chats, err := s.index.UnreadChats(ctx, a.IncludeArchived, min(limit(a.Limit, 20), 100))
	if err != nil {
		return toolError("%v", err)
	}
	perChat := 5
	if a.PerChat > 0 {
		perChat = min(a.PerChat, 20)
	}
	includeMuted := a.IncludeMuted == nil || *a.IncludeMuted
	marks := s.marks(ctx)
	now := time.Now()
	out := []map[string]any{}
	total := 0
	for _, c := range chats {
		if c.Muted && !includeMuted {
			continue
		}
		n := perChat
		if c.Unread > 0 {
			n = min(c.Unread, perChat)
		} else {
			n = 1
		}
		rows, err := s.index.Incoming(ctx, c.JID, n)
		if err != nil {
			return toolError("%v", err)
		}
		msgs := make([]preview, 0, len(rows))
		for i := len(rows) - 1; i >= 0; i-- {
			msgs = append(msgs, s.preview(ctx, rows[i]))
		}
		row := map[string]any{"chat_jid": c.JID, "name": s.index.Names.Name(ctx, c.JID), "group": strings.HasSuffix(c.JID, "@g.us"),
			"unread": c.Unread, "last_message_at": c.LastMessage, "messages": msgs}
		if c.MarkedUnread {
			row["marked_unread"] = true
		}
		if c.Muted {
			row["muted"] = true
		}
		if c.Archived {
			row["archived"] = true
		}
		if m, ok := marks[c.JID]; ok && len(rows) > 0 && m.Hides(rows[0].Timestamp, now) {
			row["handled"] = true
		}
		total += c.Unread
		out = append(out, row)
	}
	return textResult(map[string]any{"chats": out, "count": len(out), "unread_messages": total,
		"note":              "unread as the phone shows it: reading a chat on the phone clears it here too. messages are the latest received in each chat, up to per_chat",
		"untrusted_content": UntrustedContent}, false)
}

// closings are the short replies that end an exchange rather than ask for
// an answer: a chat whose last message is one of them is not waiting.
var closings = map[string]bool{
	"ok": true, "okay": true, "okk": true, "blz": true, "beleza": true, "obrigado": true, "obrigada": true, "obg": true,
	"brigado": true, "brigada": true, "valeu": true, "vlw": true, "tmj": true, "show": true, "top": true, "perfeito": true,
	"combinado": true, "fechado": true, "certo": true, "certinho": true, "de nada": true, "imagina": true, "otimo": true,
	"ótimo": true, "massa": true, "joia": true, "jóia": true, "thanks": true, "thank you": true, "thx": true, "ty": true,
	"👍": true, "🙏": true, "❤️": true, "❤": true, "👌": true, "😘": true, "🙌": true, "👏": true,
}

// isClosing reports whether a message only closes the exchange: a sticker,
// or a short thanks or ok, mentions of the account aside.
func isClosing(r index.Row, ownIDs []string) bool {
	if r.MediaType == "sticker" {
		return true
	}
	if r.MediaType != "" {
		return false
	}
	t := strings.ToLower(r.Text)
	for _, id := range ownIDs {
		if id != "" {
			t = strings.ReplaceAll(t, "@"+id, "")
		}
	}
	t = strings.TrimFunc(t, func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune(".,!?;:~", r) })
	t = strings.Join(strings.Fields(t), " ")
	return closings[t]
}

func (s *Server) ownIDs(ctx context.Context) []string {
	phone, lid := s.index.Names.Own(ctx)
	var ids []string
	for _, id := range []string{phone, lid} {
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func (s *Server) listUnanswered(ctx context.Context, a arguments) map[string]any {
	since := time.Now().AddDate(0, 0, -30)
	if a.Since != "" {
		t, err := parseWhen(a.Since, "since")
		if err != nil {
			return toolError("%v", err)
		}
		since = t
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	last, err := s.index.LastMessages(ctx, since)
	if err != nil {
		return toolError("%v", err)
	}
	ignoreClosing := a.IgnoreClosing == nil || *a.IgnoreClosing
	includeMuted := a.IncludeMuted == nil || *a.IncludeMuted
	own := s.ownIDs(ctx)
	marks := s.marks(ctx)
	now := time.Now()
	type waiting struct {
		row  map[string]any
		last time.Time
	}
	var found []waiting
	seen := map[string]bool{}
	add := func(r index.Row, mention *index.Row) {
		if seen[r.ChatJID] {
			return
		}
		chat, known := s.index.Chat(ctx, r.ChatJID)
		if known && chat.Archived && !a.IncludeArchived {
			return
		}
		if known && chat.Muted && !includeMuted {
			return
		}
		handled := false
		if m, ok := marks[r.ChatJID]; ok && m.Hides(r.Timestamp, now) {
			if !a.IncludeHandled {
				return
			}
			handled = true
		}
		seen[r.ChatJID] = true
		count, first, _ := s.index.Waiting(ctx, r.ChatJID)
		row := map[string]any{"chat_jid": r.ChatJID, "name": s.index.Names.Name(ctx, r.ChatJID), "group": strings.HasSuffix(r.ChatJID, "@g.us"),
			"last_message": s.preview(ctx, r), "waiting_messages": count, "age_hours": roundHours(now.Sub(r.Timestamp))}
		if !first.IsZero() {
			row["waiting_since"] = first
		}
		if mention != nil {
			row["mention"] = s.preview(ctx, *mention)
		}
		if known && chat.Muted {
			row["muted"] = true
		}
		if handled {
			row["handled"] = true
		}
		found = append(found, waiting{row: row, last: r.Timestamp})
	}
	for _, r := range last {
		if r.FromMe {
			continue
		}
		group := strings.HasSuffix(r.ChatJID, "@g.us")
		if group && !a.IncludeGroups {
			continue
		}
		if ignoreClosing && isClosing(r, own) {
			continue
		}
		if a.MinAgeHours > 0 && now.Sub(r.Timestamp) < time.Duration(a.MinAgeHours*float64(time.Hour)) {
			continue
		}
		add(r, nil)
	}
	if a.IncludeGroupMentions && !a.IncludeGroups && len(own) > 0 {
		mentions, err := s.index.Mentions(ctx, own, "", since, 200)
		if err != nil {
			return toolError("%v", err)
		}
		for _, m := range mentions {
			if !strings.HasSuffix(m.ChatJID, "@g.us") || seen[m.ChatJID] || !m.Timestamp.After(s.index.LastSent(ctx, m.ChatJID)) {
				continue
			}
			lastRow := m
			for _, r := range last {
				if r.ChatJID == m.ChatJID {
					lastRow = r
					break
				}
			}
			mention := m
			add(lastRow, &mention)
		}
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].last.After(found[j].last) })
	n := min(limit(a.Limit, 30), 200)
	truncated := len(found) > n
	if truncated {
		found = found[:n]
	}
	rows := make([]map[string]any, 0, len(found))
	for _, f := range found {
		rows = append(rows, f.row)
	}
	result := map[string]any{"chats": rows, "count": len(rows), "truncated": truncated, "since": since,
		"note":              "a chat waits when its latest message came from the other side. mark_handled or snooze_chat take one off this list until someone writes in it again",
		"untrusted_content": UntrustedContent}
	if a.IncludeGroupMentions && len(own) == 0 {
		result["mentions_note"] = "the paired account's number is not known yet, so group mentions could not be looked for"
	}
	return textResult(result, false)
}

func roundHours(d time.Duration) float64 {
	return float64(int(d.Hours()*10)) / 10
}

func (s *Server) listMentions(ctx context.Context, a arguments) map[string]any {
	since := time.Now().AddDate(0, 0, -30)
	if a.Since != "" {
		t, err := parseWhen(a.Since, "since")
		if err != nil {
			return toolError("%v", err)
		}
		since = t
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	own := s.ownIDs(ctx)
	if len(own) == 0 {
		return toolError("the paired account's number is not known yet; pair WhatsApp first")
	}
	rows, err := s.index.Mentions(ctx, own, strings.TrimSpace(a.ChatJID), since, min(limit(a.Limit, 50), 200))
	if err != nil {
		return toolError("%v", err)
	}
	sent := map[string]time.Time{}
	out := []map[string]any{}
	for _, r := range rows {
		last, ok := sent[r.ChatJID]
		if !ok {
			last = s.index.LastSent(ctx, r.ChatJID)
			sent[r.ChatJID] = last
		}
		answered := last.After(r.Timestamp)
		if a.OnlyUnanswered && answered {
			continue
		}
		m := rowMessage(r)
		s.name(ctx, &m)
		out = append(out, map[string]any{"message": m, "answered": answered})
	}
	return textResult(map[string]any{"mentions": out, "count": len(out), "since": since,
		"note":              "answered means the account wrote in that chat after the mention. Found by the @number WhatsApp writes into a mention's text",
		"untrusted_content": UntrustedContent}, false)
}

func (s *Server) markHandled(ctx context.Context, a arguments) map[string]any {
	chat := strings.TrimSpace(a.ChatJID)
	if chat == "" {
		return toolError("chat_jid is required")
	}
	if a.Clear {
		if err := s.state.ClearMark(ctx, chat); err != nil {
			return toolError("%v", err)
		}
		return textResult(map[string]any{"cleared": true, "chat_jid": chat}, false)
	}
	if err := s.state.MarkHandled(ctx, chat, strings.TrimSpace(a.Note), time.Now()); err != nil {
		return toolError("%v", err)
	}
	return textResult(map[string]any{"handled": true, "chat_jid": chat, "name": s.index.Names.Name(ctx, chat),
		"note": "kept on this computer only: nothing is sent and the other side sees nothing. A new message in the chat brings it back to the lists"}, false)
}

func (s *Server) snoozeChat(ctx context.Context, a arguments) map[string]any {
	chat := strings.TrimSpace(a.ChatJID)
	if chat == "" {
		return toolError("chat_jid is required")
	}
	until, err := parseWhen(a.Until, "until")
	if err != nil {
		return toolError("%v", err)
	}
	if until.IsZero() || !until.After(time.Now()) {
		return toolError("until must be a moment in the future")
	}
	if err := s.state.Snooze(ctx, chat, strings.TrimSpace(a.Note), time.Now(), until); err != nil {
		return toolError("%v", err)
	}
	return textResult(map[string]any{"snoozed": true, "chat_jid": chat, "name": s.index.Names.Name(ctx, chat), "until": until,
		"note": "kept on this computer only. The chat comes back to the lists at that moment, or earlier if someone writes in it"}, false)
}
