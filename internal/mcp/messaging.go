package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/index"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/wacli"
)

// delegatedOrExclusive runs a chat-state command through the running sync,
// and falls back to pausing it when the wacli in use is older than the one
// whose sync accepts that command (the command line may run any wacli).
func (s *Server) delegatedOrExclusive(ctx context.Context, reason string, args ...string) (map[string]any, error) {
	out, err := s.delegated(ctx, args...)
	if wacli.IsLockError(err) {
		var o map[string]any
		err = s.exclusive(ctx, reason, &o, args...)
		return o, err
	}
	return out, err
}

// resolveRecipient turns what a send was given (a JID, a phone number or a
// name) into the chat it reaches. A name matching several chats returns them
// all, since wacli refuses to guess between them.
func (s *Server) resolveRecipient(ctx context.Context, to string) (string, []index.Contact, error) {
	to = strings.TrimSpace(to)
	if strings.Contains(to, "@") {
		return to, nil, nil
	}
	if digits := phoneDigits(to); digits != "" {
		return digits + "@s.whatsapp.net", nil, nil
	}
	matches := s.index.Names.Contacts(ctx, to, 5)
	if groups, err := s.index.Groups(ctx, to, 5); err == nil {
		for _, g := range groups {
			matches = append(matches, index.Contact{JID: g.JID, Name: g.Name})
		}
	}
	switch len(matches) {
	case 0:
		return "", nil, fmt.Errorf("no contact or group on this machine is called %q; use a JID or a phone number with country code", to)
	case 1:
		return matches[0].JID, nil, nil
	}
	return "", matches, nil
}

// phoneDigits returns the digits of a phone number written with the usual
// punctuation, or "" when the text is not one.
func phoneDigits(v string) string {
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case strings.ContainsRune("+-() .", r):
		default:
			return ""
		}
	}
	if b.Len() < 8 {
		return ""
	}
	return b.String()
}

// dryRun answers a send asked as a draft: what would go, and to whom, with
// nothing sent.
func (s *Server) dryRun(ctx context.Context, to string, draft map[string]any) map[string]any {
	jid, candidates, err := s.resolveRecipient(ctx, to)
	if err != nil {
		return toolError("%v", err)
	}
	result := map[string]any{"dry_run": true, "sent": false, "draft": draft}
	if len(candidates) > 0 {
		result["recipient_candidates"] = candidates
		result["next"] = "nothing was sent. The name matches several chats, and sending to it would fail: send to the JID of the right one"
		return textResult(result, false)
	}
	result["recipient"] = map[string]any{"jid": jid, "name": s.index.Names.Name(ctx, jid)}
	if !s.index.ChatKnown(ctx, jid) {
		result["note"] = "this machine has no conversation with this recipient yet; check_numbers confirms that a number has WhatsApp"
	}
	result["next"] = "nothing was sent. Show the draft to the user and, once they approve it, call again without dry_run"
	return textResult(result, false)
}

// reply is a resolved reply_to: the arguments that make wacli quote the
// message, and what the quoted message says, for drafts.
type reply struct {
	args   []string
	quoted map[string]any
}

// replyArgs resolves the message a send quotes. WhatsApp quotes a message of
// the same conversation, so one found elsewhere is refused rather than sent
// as a reply that would not show.
func (s *Server) replyArgs(ctx context.Context, id, to string) (*reply, map[string]any) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	chat, _, _ := s.resolveRecipient(ctx, to)
	m, err := s.index.MessageByID(ctx, id, chat)
	if errors.Is(err, index.ErrNotFound) && chat != "" {
		// The conversation may be stored under the person's other identity
		// (phone number or LID), which wacli resolves on its own.
		if other, err2 := s.index.MessageByID(ctx, id, ""); err2 == nil {
			if !s.index.Names.Same(ctx, other.ChatJID, chat) {
				return nil, toolError("message %s belongs to %s (%s), not to the conversation being sent to; a reply quotes a message of the same conversation",
					id, s.index.Names.Name(ctx, other.ChatJID), other.ChatJID)
			}
			m, err = other, nil
		}
	}
	if errors.Is(err, index.ErrNotFound) {
		return nil, toolError("no indexed message has id %s; the reading tools return the ids this machine knows", id)
	}
	if err != nil {
		return nil, toolError("%v", err)
	}
	if m.Revoked {
		return nil, toolError("message %s was deleted, so it cannot be quoted", id)
	}
	args := []string{"--reply-to", m.ID}
	if strings.HasSuffix(m.ChatJID, "@g.us") && !m.FromMe && m.SenderJID != "" {
		args = append(args, "--reply-to-sender", m.SenderJID)
	}
	quoted := map[string]any{"id": m.ID, "from_me": m.FromMe, "timestamp": m.Timestamp, "text": clip(m.Text, 200)}
	if !m.FromMe {
		quoted["sender"] = s.index.Names.Name(ctx, m.SenderJID)
	}
	if m.MediaType != "" {
		quoted["media_type"] = m.MediaType
	}
	return &reply{args: args, quoted: quoted}, nil
}

// mentionArgs turns the people a text mentions into wacli's flags. WhatsApp
// highlights a mention only where the text says @<number>, so a mention the
// text does not place is refused instead of sent as a plain notification.
func mentionArgs(text string, mentions []string) ([]string, error) {
	var args []string
	for _, m := range mentions {
		m = strings.TrimSpace(m)
		user := m
		if u, _, ok := strings.Cut(m, "@"); ok {
			user, _, _ = strings.Cut(u, ":")
		} else {
			user = phoneDigits(m)
			m = user
		}
		if user == "" {
			return nil, fmt.Errorf("mention %q is neither a phone number with country code nor a JID", m)
		}
		if !strings.Contains(text, "@"+user) {
			return nil, fmt.Errorf("the text must contain @%s where the mention goes; WhatsApp shows it as the person's name", user)
		}
		args = append(args, "--mention", m)
	}
	return args, nil
}

// clip shortens a text to n runes, marking the cut.
func clip(v string, n int) string {
	r := []rune(v)
	if n <= 0 || len(r) <= n {
		return v
	}
	return string(r[:n]) + "…"
}

func (s *Server) forwardMessage(ctx context.Context, a arguments) map[string]any {
	if strings.TrimSpace(a.To) == "" {
		return toolError("to is required")
	}
	m, fail := s.message(ctx, a)
	if fail != nil {
		return fail
	}
	if m.Revoked {
		return toolError("message %s was deleted, so it cannot be forwarded", m.ID)
	}
	if a.DryRun {
		draft := map[string]any{"forward": map[string]any{"id": m.ID, "from": s.index.Names.Name(ctx, m.ChatJID),
			"timestamp": m.Timestamp, "text": clip(m.Text, 200), "media_type": m.MediaType, "filename": m.Filename}}
		return s.dryRun(ctx, a.To, draft)
	}
	var out map[string]any
	err := s.exclusive(ctx, "forward message", &out, "messages", "forward", "--chat", m.ChatJID, "--id", m.ID, "--to", a.To)
	return sendResult(out, err)
}

func (s *Server) markChatRead(ctx context.Context, a arguments) map[string]any {
	chat := strings.TrimSpace(a.ChatJID)
	if chat == "" {
		return toolError("chat_jid is required")
	}
	receipts := a.Receipts == nil || *a.Receipts
	args := []string{"chats", "mark-read", "--chat", chat}
	if receipts {
		args = append(args, "--receipts")
	}
	out, err := s.delegatedOrExclusive(ctx, "mark chat read", args...)
	if err != nil {
		return toolError("%v", err)
	}
	return textResult(map[string]any{"done": true, "chat_jid": chat, "receipts": receipts, "result": out}, false)
}

func (s *Server) sendTyping(ctx context.Context, a arguments) map[string]any {
	to := strings.TrimSpace(a.To)
	if to == "" {
		return toolError("to is required")
	}
	typing := a.Typing == nil || *a.Typing
	args := []string{"presence", "paused", "--to", to}
	if typing {
		args = []string{"presence", "typing", "--to", to}
		if a.Audio {
			args = append(args, "--media", "audio")
		}
	}
	if _, err := s.delegatedOrExclusive(ctx, "typing indicator", args...); err != nil {
		return toolError("%v", err)
	}
	result := map[string]any{"done": true, "to": to, "typing": typing}
	if typing {
		result["note"] = "WhatsApp clears the indicator by itself after a few seconds, and when a message is sent"
	}
	return textResult(result, false)
}
