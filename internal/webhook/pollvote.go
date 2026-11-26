package webhook

import (
	"context"
	"strings"
	"time"
)

// PollVote says someone voted in a poll, changed their vote or took it back.
// WhatsApp sends the vote encrypted; wacli posts it before it has decrypted
// and stored the choice, so the relay reads the poll again until the vote is
// there and sends the whole tally with it, the shape get_poll_results reads.
type PollVote struct {
	PollID    string `json:"poll_id"`
	ChatJID   string `json:"chat_jid"`
	ChatName  string `json:"chat_name,omitempty"`
	Group     bool   `json:"group"`
	VoterJID  string `json:"voter_jid"`
	VoterName string `json:"voter_name,omitempty"`
	FromMe    bool   `json:"from_me"`
	// Selected is what the voter picked now; empty when they took the vote back.
	Selected []string  `json:"selected"`
	VotedAt  time.Time `json:"voted_at"`
	// The poll and its tally after this vote. They are missing when the poll
	// is not in the local store (sent before this computer was linked).
	Question string         `json:"question,omitempty"`
	Options  []string       `json:"options,omitempty"`
	Tally    map[string]int `json:"tally,omitempty"`
	Voters   []PollVoter    `json:"voters,omitempty"`
}

// PollVoter is one person's current vote.
type PollVoter struct {
	JID      string    `json:"jid"`
	Name     string    `json:"name,omitempty"`
	Selected []string  `json:"selected"`
	VotedAt  time.Time `json:"voted_at"`
}

// Poll is a poll as wacli's "poll show" answers it.
type Poll struct {
	ID         string         `json:"msg_id"`
	ChatJID    string         `json:"chat_jid"`
	Question   string         `json:"question"`
	Options    []string       `json:"options"`
	Aggregates map[string]int `json:"aggregates"`
	Voters     []PollVoter    `json:"voters"`
}

// PollReader reads a poll from the local store. The daemon wires it to
// "wacli --read-only poll show"; without one, votes are not sent.
type PollReader func(ctx context.Context, chat, id string) (Poll, error)

// SetPolls lets the relay read polls, which turns on poll_vote events.
func (r *Relay) SetPolls(read PollReader) { r.polls = read }

// Waits between reads of a poll while its vote is still being stored. Sync
// usually stores it within a second; the reads stop after about eight.
var pollVoteWaits = []time.Duration{300 * time.Millisecond, 700 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second}

type pollVotePayload struct {
	PollMessageID string
	PollChatJID   string
	PollSenderJID string
	SenderTsMS    int64
}

// pollVote reads the poll until the vote that arrived is in it, then sends
// the event. It runs on its own goroutine: the relay answers sync at once.
func (r *Relay) pollVote(p wacliPayload, chatName, voterName string) {
	if r.polls == nil {
		return
	}
	chat := p.PollVote.PollChatJID
	if chat == "" {
		chat = p.Chat
	}
	at := p.Timestamp.UTC()
	if p.PollVote.SenderTsMS > 0 {
		at = time.UnixMilli(p.PollVote.SenderTsMS).UTC()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var poll Poll
	var voter *PollVoter
	read := false
	for _, wait := range pollVoteWaits {
		time.Sleep(wait)
		got, err := r.polls(ctx, chat, p.PollVote.PollMessageID)
		if err != nil {
			continue
		}
		poll, read = got, true
		voter = r.findVoter(ctx, poll.Voters, p.SenderJID)
		// VotedAt has whole seconds; the vote is in once it is not older
		// than the one that arrived.
		if voter != nil && !voter.VotedAt.Before(at.Truncate(time.Second)) {
			break
		}
	}
	v := &PollVote{PollID: p.PollVote.PollMessageID, ChatJID: chat, ChatName: chatName, Group: strings.HasSuffix(chat, "@g.us"),
		VoterJID: p.SenderJID, VoterName: voterName, FromMe: p.FromMe, Selected: []string{}, VotedAt: at}
	if voter != nil && !voter.VotedAt.Before(at.Truncate(time.Second)) {
		v.Selected = append(v.Selected, voter.Selected...)
		if voter.Name != "" && v.VoterName == "" {
			v.VoterName = voter.Name
		}
	}
	if read {
		v.Question, v.Options, v.Tally, v.Voters = poll.Question, poll.Options, poll.Aggregates, poll.Voters
	}
	r.m.Dispatch(ctx, Event{Kind: "poll_vote", ID: randomHex(8), At: time.Now().UTC(), PollVote: v, chat: chat, fromMe: p.FromMe})
}

// findVoter finds the voter's line, by JID or by the same person under
// another JID (a phone number and its LID).
func (r *Relay) findVoter(ctx context.Context, voters []PollVoter, jid string) *PollVoter {
	for i := range voters {
		if voters[i].JID == jid {
			return &voters[i]
		}
	}
	if r.m.same != nil {
		for i := range voters {
			if r.m.same(ctx, voters[i].JID, jid) {
				return &voters[i]
			}
		}
	}
	return nil
}
