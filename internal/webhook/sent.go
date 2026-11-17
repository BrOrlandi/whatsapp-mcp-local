package webhook

import (
	"context"
	"strings"
	"time"
)

// Sent is a message the account sent through the tools. WhatsApp does not
// echo a device's own sends back to it, so sync never posts these: the tools
// report them here instead, and the webhooks that include the account's own
// messages get them like the ones sent from the phone.
type Sent struct {
	ID         string
	Chat       string // as wacli answered the send; a LID is turned into the phone JID
	Text       string
	ReplyTo    string
	ReactionTo string // set for a reaction, with Emoji (empty when removed)
	Emoji      string
	Forwarded  bool
	Media      *Media
	Location   map[string]any
	Poll       map[string]any
}

// canonical is the part of the index that names a person by one JID.
type canonical interface {
	Canonical(ctx context.Context, jid string) string
}

// Sent dispatches a message the tools sent.
func (r *Relay) Sent(ctx context.Context, s Sent) {
	if r == nil || s.ID == "" || s.Chat == "" {
		return
	}
	chat := s.Chat
	if c, ok := r.names.(canonical); ok {
		chat = c.Canonical(ctx, chat)
	}
	now := time.Now().UTC()
	m := &Message{ID: s.ID, ChatJID: chat, ChatName: r.names.Name(ctx, chat), Group: strings.HasSuffix(chat, "@g.us"),
		Timestamp: now, FromMe: true, Text: s.Text, Forwarded: s.Forwarded, Media: s.Media}
	if s.Location != nil {
		m.Location = s.Location
	}
	if s.Poll != nil {
		m.Poll = s.Poll
	}
	if s.Media != nil && s.Media.Caption != "" {
		m.Text = s.Media.Caption
	}
	if s.ReplyTo != "" {
		m.ReplyTo = &Reference{ID: s.ReplyTo}
	}
	ev := Event{Kind: "message", ID: randomHex(8), At: now, Message: m, chat: chat, fromMe: true}
	if s.ReactionTo != "" {
		ev.Kind = "reaction"
		m.Text = ""
		m.Reaction = &Reaction{To: s.ReactionTo, Emoji: s.Emoji}
	}
	r.m.Dispatch(ctx, ev)
}
