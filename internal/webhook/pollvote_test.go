package webhook

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func init() {
	pollVoteWaits = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond, time.Millisecond, time.Millisecond}
}

const (
	pollGroup = "120363000000000001@g.us"
	pollVoter = "5511900000001@s.whatsapp.net"
	// A vote as wacli posts it: no choice yet, only the poll it is about.
	votePayload = `{"Chat":"120363000000000001@g.us","ID":"V1","SenderJID":"5511900000001@s.whatsapp.net","PushName":"Maria","Timestamp":"2026-10-07T10:00:05Z","PollVote":{"PollMessageID":"P1","PollChatJID":"120363000000000001@g.us","PollSenderJID":"5511900000009@s.whatsapp.net","SenderTsMS":1791367205000}}`
)

func pollRelay(t *testing.T, read PollReader) (*Relay, *receiver) {
	t.Helper()
	m, _ := manager(t)
	r := newReceiver(t, func(int32) int { return http.StatusOK })
	_, secret, _ := m.Create(context.Background(), Settings{URL: ptr(r.srv.URL), Events: ptr([]string{"message", "poll_vote"})})
	r.secret = secret
	relay, err := NewRelay(m, names{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { relay.Close() })
	if read != nil {
		relay.SetPolls(read)
	}
	req, _ := http.NewRequest(http.MethodPost, relay.URL(), bytes.NewReader([]byte(votePayload)))
	req.Header.Set("X-Wacli-Signature", Sign(relay.Secret(), []byte(votePayload)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the vote answered %d", resp.StatusCode)
	}
	return relay, r
}

func poll(voters ...PollVoter) Poll {
	tally := map[string]int{"Sim": 0, "Não": 0}
	for _, v := range voters {
		for _, s := range v.Selected {
			tally[s]++
		}
	}
	return Poll{ID: "P1", ChatJID: pollGroup, Question: "Churrasco?", Options: []string{"Sim", "Não"}, Aggregates: tally, Voters: voters}
}

var voteAt = time.UnixMilli(1791367205000).UTC()

// The vote is posted before sync stores it: the relay reads the poll again
// until the voter's line is as new as the vote, then sends the tally.
func TestPollVoteWaitsForTheStoredVote(t *testing.T) {
	var reads atomic.Int32
	_, r := pollRelay(t, func(_ context.Context, chat, id string) (Poll, error) {
		if chat != pollGroup || id != "P1" {
			t.Errorf("read poll %s in %s", id, chat)
		}
		switch reads.Add(1) {
		case 1:
			return Poll{}, errors.New("not stored yet")
		case 2: // an older vote of the same person, before the change
			return poll(PollVoter{JID: pollVoter, Name: "Maria", Selected: []string{"Não"}, VotedAt: voteAt.Add(-time.Hour)}), nil
		}
		return poll(PollVoter{JID: pollVoter, Name: "Maria", Selected: []string{"Sim"}, VotedAt: voteAt},
			PollVoter{JID: "5511900000002@s.whatsapp.net", Name: "João", Selected: []string{"Sim"}, VotedAt: voteAt.Add(-time.Minute)}), nil
	})
	waitFor(t, "the poll_vote", func() bool { return len(r.events()) >= 1 })
	ev := r.events()[0]
	v := ev.PollVote
	if ev.Kind != "poll_vote" || v == nil || ev.Message != nil {
		t.Fatalf("event: %+v", ev)
	}
	if v.PollID != "P1" || v.ChatJID != pollGroup || !v.Group || v.VoterJID != pollVoter || v.VoterName != "Maria" ||
		len(v.Selected) != 1 || v.Selected[0] != "Sim" || !v.VotedAt.Equal(voteAt) {
		t.Errorf("vote: %+v", v)
	}
	if v.Question != "Churrasco?" || v.Tally["Sim"] != 2 || v.Tally["Não"] != 0 || len(v.Voters) != 2 {
		t.Errorf("tally: %+v", v)
	}
	if n := reads.Load(); n != 3 {
		t.Errorf("read the poll %d times, want 3 (stop once the vote is in)", n)
	}
	if !r.sigOK[0] {
		t.Error("the poll_vote went out without the right signature or event header")
	}
}

// A vote taken back leaves the person out of the poll: after the reads, the
// event says so with an empty selected.
func TestPollVoteTakenBack(t *testing.T) {
	var reads atomic.Int32
	_, r := pollRelay(t, func(context.Context, string, string) (Poll, error) {
		reads.Add(1)
		return poll(), nil
	})
	waitFor(t, "the poll_vote", func() bool { return len(r.events()) >= 1 })
	v := r.events()[0].PollVote
	if v == nil || v.Selected == nil || len(v.Selected) != 0 || v.Question != "Churrasco?" {
		t.Errorf("vote taken back: %+v", v)
	}
	if n := reads.Load(); n != int32(len(pollVoteWaits)) {
		t.Errorf("read the poll %d times, want every wait (%d)", n, len(pollVoteWaits))
	}
}

// A poll the store does not have still says who voted, without the tally.
func TestPollVoteWithoutThePoll(t *testing.T) {
	_, r := pollRelay(t, func(context.Context, string, string) (Poll, error) {
		return Poll{}, errors.New("poll not found")
	})
	waitFor(t, "the poll_vote", func() bool { return len(r.events()) >= 1 })
	v := r.events()[0].PollVote
	if v == nil || v.PollID != "P1" || v.VoterJID != pollVoter || v.Tally != nil || v.Voters != nil || len(v.Selected) != 0 {
		t.Errorf("vote without the poll: %+v", v)
	}
}

// Without a poll reader, votes stay what they were: nothing a webhook gets,
// and never a message event.
func TestPollVoteOffWithoutAReader(t *testing.T) {
	_, r := pollRelay(t, nil)
	time.Sleep(100 * time.Millisecond)
	if evs := r.events(); len(evs) != 0 {
		t.Errorf("delivered %+v", evs)
	}
}
