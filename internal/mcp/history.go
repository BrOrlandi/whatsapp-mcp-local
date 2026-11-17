package mcp

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

// HistoryJob is one sync_history request running in the background. Each
// conversation is its own exclusive step, so sends issued meanwhile wait for
// at most one conversation rather than for the whole job.
type HistoryJob struct {
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
	Chats      []string       `json:"chats"`
	Count      int            `json:"count"`
	Rounds     int            `json:"rounds"`
	Done       int            `json:"chats_done"`
	Added      int            `json:"messages_added"`
	Current    string         `json:"current_chat,omitempty"`
	Failures   map[string]any `json:"failures,omitempty"`
}

func (s *Server) syncHistory(ctx context.Context, a arguments) map[string]any {
	job, started, err := s.RequestHistory(ctx, a.ChatJID, a.Count, a.Rounds, a.Chats)
	if err != nil {
		return toolError("%v", err)
	}
	if !started {
		return textResult(map[string]any{"started": false, "reason": "a history request is already running", "job": job}, false)
	}
	return textResult(map[string]any{"started": true, "chats": job.Chats, "count": job.Count, "rounds": job.Rounds,
		"next": "the phone answers in the background; follow it in whatsapp_status and read the conversation again when it finishes"}, false)
}

// RequestHistory asks the phone for older messages in the background: for one
// conversation, or for the most recently active ones. It reports false, with
// the running job, when a request is already under way.
func (s *Server) RequestHistory(ctx context.Context, chatJID string, count, rounds, chats int) (HistoryJob, bool, error) {
	s.historyMu.Lock()
	if s.history != nil && s.history.FinishedAt == nil {
		job := *s.history
		s.historyMu.Unlock()
		return job, false, nil
	}
	s.historyMu.Unlock()

	var targets []string
	if jid := strings.TrimSpace(chatJID); jid != "" {
		targets = []string{jid}
	} else {
		if chats <= 0 {
			chats = 10
		}
		active, err := s.index.ActiveChats(ctx, min(chats, 50))
		if err != nil {
			return HistoryJob{}, false, err
		}
		targets = active
	}
	if len(targets) == 0 {
		return HistoryJob{}, false, errors.New("the index holds no conversation to anchor on yet; history can only be requested backwards from a message this machine already has")
	}
	if count <= 0 {
		count = 50
	}
	count = min(count, 500)
	rounds = min(max(rounds, 1), 20)

	job := &HistoryJob{StartedAt: time.Now().UTC(), Chats: targets, Count: count, Rounds: rounds, Failures: map[string]any{}}
	s.historyMu.Lock()
	s.history = job
	s.historyMu.Unlock()
	go s.runHistory(job, count, rounds)
	return *job, true, nil
}

func (s *Server) runHistory(job *HistoryJob, count, rounds int) {
	for _, chat := range job.Chats {
		s.historyMu.Lock()
		job.Current = chat
		s.historyMu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(rounds)*3*time.Minute)
		var out struct {
			MessagesAdded int `json:"messages_added"`
		}
		err := s.supervisor.Exclusive(ctx, "history backfill "+chat, func(ctx context.Context) error {
			return s.cli.Decode(ctx, &out, "history", "backfill", "--chat", chat,
				"--count", strconv.Itoa(count), "--requests", strconv.Itoa(rounds), "--wait", "40s")
		})
		cancel()

		s.historyMu.Lock()
		job.Done++
		job.Added += out.MessagesAdded
		if err != nil {
			job.Failures[chat] = err.Error()
		}
		s.historyMu.Unlock()
		if err != nil {
			s.logger.Warn("history backfill failed", "chat", chat, "error", err)
		}
	}
	s.historyMu.Lock()
	now := time.Now().UTC()
	job.FinishedAt = &now
	job.Current = ""
	s.historyMu.Unlock()
}

// HistoryStatus is the last history request, if any.
func (s *Server) HistoryStatus() *HistoryJob { return s.historyStatus() }

func (s *Server) historyStatus() *HistoryJob {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	if s.history == nil {
		return nil
	}
	job := *s.history
	job.Failures = map[string]any{}
	for k, v := range s.history.Failures {
		job.Failures[k] = v
	}
	return &job
}
