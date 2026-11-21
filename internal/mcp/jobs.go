package mcp

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/index"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

// A transcription outlives the call that asked for it. On a slow processor a
// voice note can take longer than an AI tool waits for an answer; were the
// work tied to the call, it would be killed when the tool gave up and start
// over from nothing on the next try, never finishing. So it runs on its own,
// keeps the transcript when it ends, and a call for the same voice note while
// it runs joins it instead of starting another.

var (
	// transcribeWait is the most a call waits for a transcription before
	// answering that it is still going, under the five minutes AI tools
	// commonly wait for a tool.
	transcribeWait = 240 * time.Second
	// transcribeTimeout bounds a transcription from the moment it is asked
	// for, the wait for its turn included; the engine bounds the run itself.
	transcribeTimeout = 3 * time.Hour
	// retryAfter is when a call answered "still going" is told to ask again.
	retryAfter = 30 * time.Second
)

// waitLimit is transcribeWait, or WHATSAPP_MCP_TRANSCRIPTION_WAIT (seconds)
// for AI tools that give up sooner than five minutes.
func waitLimit() time.Duration {
	if v, err := strconv.Atoi(os.Getenv("WHATSAPP_MCP_TRANSCRIPTION_WAIT")); err == nil && v > 0 {
		return time.Duration(v) * time.Second
	}
	return transcribeWait
}

// transcription is one voice note being transcribed.
type transcription struct {
	started time.Time
	done    chan struct{}
	// Set once done is closed.
	t        state.Transcript
	audioCtx index.AudioContext
	backend  string
	fallback string
	err      error
}

// transcriptions are the ones running, by chat and message.
type transcriptions struct {
	mu   sync.Mutex
	jobs map[string]*transcription
}

// work is what a transcription does: make the transcript and keep it.
type work func(ctx context.Context) (t state.Transcript, audioCtx index.AudioContext, backend, fallback string, err error)

// start returns the running transcription of a voice note, or starts one.
// joined says it was already running.
func (ts *transcriptions) start(chatJID, messageID string, do work) (job *transcription, joined bool) {
	key := chatJID + "\x00" + messageID
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if j, ok := ts.jobs[key]; ok {
		return j, true
	}
	if ts.jobs == nil {
		ts.jobs = map[string]*transcription{}
	}
	j := &transcription{started: time.Now(), done: make(chan struct{})}
	ts.jobs[key] = j
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), transcribeTimeout)
		defer cancel()
		j.t, j.audioCtx, j.backend, j.fallback, j.err = do(ctx)
		// The transcript is kept (do saves it) before the job leaves the
		// map, so a call that misses one finds the other.
		ts.mu.Lock()
		delete(ts.jobs, key)
		ts.mu.Unlock()
		close(j.done)
	}()
	return j, false
}

// running counts the transcriptions in progress.
func (ts *transcriptions) running() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return len(ts.jobs)
}

// wait waits for the transcription for up to transcribeWait, and less when
// the call's own deadline comes first. It reports whether it finished.
func (j *transcription) wait(ctx context.Context) bool {
	limit := waitLimit()
	if dl, ok := ctx.Deadline(); ok {
		// Answer a little before the caller gives up, so "still going"
		// reaches it.
		if left := time.Until(dl) - 2*time.Second; left < limit {
			limit = left
		}
	}
	if limit <= 0 {
		select {
		case <-j.done:
			return true
		default:
			return false
		}
	}
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case <-j.done:
		return true
	case <-timer.C:
		return false
	case <-ctx.Done():
		return false
	}
}
