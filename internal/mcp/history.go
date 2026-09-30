package mcp

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// historyJob is one sync_history request running in the background. Each
// conversation is its own exclusive step, so sends issued meanwhile wait for
// at most one conversation rather than for the whole job.
type historyJob struct {
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
	Chats      []string       `json:"chats"`
	Done       int            `json:"chats_done"`
	Added      int            `json:"messages_added"`
	Current    string         `json:"current_chat,omitempty"`
	Failures   map[string]any `json:"failures,omitempty"`
}

func (s *Server) syncHistory(ctx context.Context, a arguments) map[string]any {
	s.historyMu.Lock()
	if s.history != nil && s.history.FinishedAt == nil {
		job := *s.history
		s.historyMu.Unlock()
		return textResult(map[string]any{"started": false, "reason": "a history request is already running", "job": job}, false)
	}
	s.historyMu.Unlock()

	var chats []string
	if jid := strings.TrimSpace(a.ChatJID); jid != "" {
		chats = []string{jid}
	} else {
		n := a.Chats
		if n <= 0 {
			n = 10
		}
		active, err := s.index.ActiveChats(ctx, min(n, 50))
		if err != nil {
			return toolError("%v", err)
		}
		chats = active
	}
	if len(chats) == 0 {
		return toolError("the index holds no conversation to anchor on yet; history can only be requested backwards from a message this machine already has")
	}
	count := a.Count
	if count <= 0 {
		count = 50
	}
	count = min(count, 500)
	rounds := min(max(a.Rounds, 1), 20)

	job := &historyJob{StartedAt: time.Now().UTC(), Chats: chats, Failures: map[string]any{}}
	s.historyMu.Lock()
	s.history = job
	s.historyMu.Unlock()

	go s.runHistory(job, count, rounds)
	return textResult(map[string]any{"started": true, "chats": chats, "count": count, "rounds": rounds,
		"next": "the phone answers in the background; follow it in whatsapp_status and read the conversation again when it finishes"}, false)
}

func (s *Server) runHistory(job *historyJob, count, rounds int) {
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

func (s *Server) historyStatus() *historyJob {
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
