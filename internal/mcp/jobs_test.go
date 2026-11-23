package mcp

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/index"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

func shortWait(t *testing.T, d time.Duration) {
	t.Helper()
	old := transcribeWait
	transcribeWait = d
	t.Cleanup(func() { transcribeWait = old })
}

// A transcription goes on when the call that asked for it stops waiting, and
// a second call for the same voice note joins it instead of starting over.
func TestATranscriptionOutlivesItsCaller(t *testing.T) {
	shortWait(t, 50*time.Millisecond)
	var ts transcriptions
	release := make(chan struct{})
	var runs atomic.Int32
	var workCtxErr atomic.Value
	do := func(ctx context.Context) (state.Transcript, index.AudioContext, string, string, string, error) {
		runs.Add(1)
		<-release
		if err := ctx.Err(); err != nil {
			workCtxErr.Store(err)
		}
		return state.Transcript{ChatJID: "c", MessageID: "m", Text: "olá"}, index.AudioContext{}, "remote", "", "", nil
	}

	// The first caller gives up, as an AI tool does after its timeout.
	caller, cancel := context.WithCancel(context.Background())
	job, joined := ts.start("c", "m", do)
	if joined {
		t.Fatal("the first call joined something")
	}
	cancel()
	if job.wait(caller) {
		t.Fatal("finished before the work did")
	}

	// The second call within the wait limit answers "still going".
	job2, joined := ts.start("c", "m", do)
	if !joined || job2 != job {
		t.Fatal("a call for the same voice note started another transcription")
	}
	start := time.Now()
	if job2.wait(context.Background()) {
		t.Fatal("finished before the work did")
	}
	if d := time.Since(start); d < 40*time.Millisecond || d > time.Second {
		t.Errorf("waited %s, want about the wait limit", d)
	}
	if ts.running() != 1 {
		t.Errorf("running = %d", ts.running())
	}

	// Another voice note is not held behind this one here.
	other, joined := ts.start("c", "other", func(ctx context.Context) (state.Transcript, index.AudioContext, string, string, string, error) {
		return state.Transcript{Text: "outro"}, index.AudioContext{}, "local", "", "", nil
	})
	if joined || !other.wait(context.Background()) || other.t.Text != "outro" {
		t.Fatal("another voice note did not run on its own")
	}

	close(release)
	if !job.wait(context.Background()) {
		t.Fatal("did not finish")
	}
	if job.t.Text != "olá" || job.backend != "remote" || job.err != nil {
		t.Errorf("job = %+v", job)
	}
	if err := workCtxErr.Load(); err != nil {
		t.Errorf("the caller giving up cancelled the work: %v", err)
	}
	if runs.Load() != 1 {
		t.Errorf("ran %d times", runs.Load())
	}
	if ts.running() != 0 {
		t.Errorf("a finished transcription stays listed: %d", ts.running())
	}
	// Once it is over, a new call (refresh) runs it again.
	again, joined := ts.start("c", "m", func(ctx context.Context) (state.Transcript, index.AudioContext, string, string, string, error) {
		runs.Add(1)
		return state.Transcript{}, index.AudioContext{}, "local", "", "", nil
	})
	if joined || !again.wait(context.Background()) || runs.Load() != 2 {
		t.Errorf("joined %v, runs %d", joined, runs.Load())
	}
}

// The wait ends before the caller's own deadline, so the caller hears "still
// going" instead of timing out.
func TestTheWaitEndsBeforeTheCallerGivesUp(t *testing.T) {
	shortWait(t, time.Minute)
	var ts transcriptions
	release := make(chan struct{})
	defer close(release)
	job, _ := ts.start("c", "m", func(ctx context.Context) (state.Transcript, index.AudioContext, string, string, string, error) {
		<-release
		return state.Transcript{}, index.AudioContext{}, "", "", "", nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	start := time.Now()
	if job.wait(ctx) {
		t.Fatal("finished before the work did")
	}
	if d := time.Since(start); d > 1500*time.Millisecond {
		t.Errorf("waited %s, past the caller's deadline less the margin", d)
	}
	if ctx.Err() != nil {
		t.Error("answered after the caller's deadline")
	}
}

func TestAFailedTranscriptionCanBeTriedAgain(t *testing.T) {
	var ts transcriptions
	job, _ := ts.start("c", "m", func(ctx context.Context) (state.Transcript, index.AudioContext, string, string, string, error) {
		return state.Transcript{}, index.AudioContext{}, "", "", "", context.DeadlineExceeded
	})
	if !job.wait(context.Background()) || job.err == nil {
		t.Fatal("the error was lost")
	}
	if _, joined := ts.start("c", "m", func(ctx context.Context) (state.Transcript, index.AudioContext, string, string, string, error) {
		return state.Transcript{}, index.AudioContext{}, "", "", "", nil
	}); joined {
		t.Error("a failed transcription is joined instead of tried again")
	}
}
