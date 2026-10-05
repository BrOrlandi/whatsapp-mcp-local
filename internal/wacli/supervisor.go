package wacli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/platform"
)

// Supervisor owns the single `wacli sync --follow` process for a store.
//
// wacli lets exactly one process hold a store's lock. While sync holds it, the
// send-like commands are delegated to it over a local socket, and everything
// else (history backfill, archive/pin/mute, live lookups) needs the lock
// itself. The supervisor is what makes both kinds safe to call at any moment:
// delegated operations share a read lock and wait for the socket, exclusive
// operations take the write lock, stop sync, run, and start it again. The
// seconds sync is down are covered by WhatsApp's offline queue, which it
// replays on reconnect.
type Supervisor struct {
	cli    *CLI
	logger *slog.Logger

	ops sync.RWMutex // delegated operations read-lock it, exclusive ones write-lock it

	mu        sync.Mutex
	changed   chan struct{}
	want      bool
	proc      *os.Process
	exited    chan struct{}
	wake      chan struct{}
	state     string
	since     time.Time
	lastError string
	restarts  int
	pausedFor string
	events    []Event

	pair   pairer
	pairWG sync.WaitGroup

	history HistoryProgress
}

// HistoryProgress is the history the phone has been sending to the running
// sync: after pairing, most of an account's past arrives this way.
type HistoryProgress struct {
	Messages      int64     `json:"messages_synced"`
	Conversations int       `json:"conversations"`
	LastAt        time.Time `json:"last_at,omitempty"`
}

// Event is one lifecycle line sync printed, kept for the status report.
type Event struct {
	At    time.Time      `json:"at"`
	Event string         `json:"event"`
	Data  map[string]any `json:"data,omitempty"`
}

// Status is what the supervisor knows about sync right now.
type Status struct {
	State     string          `json:"state"`
	Since     time.Time       `json:"since"`
	LastError string          `json:"last_error,omitempty"`
	Restarts  int             `json:"restarts"`
	PausedFor string          `json:"paused_for,omitempty"`
	Delegate  bool            `json:"delegate_socket_ready"`
	History   HistoryProgress `json:"history"`
	Recent    []Event         `json:"recent_events,omitempty"`
}

const maxEvents = 30

func NewSupervisor(cli *CLI, logger *slog.Logger) *Supervisor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Supervisor{cli: cli, logger: logger, want: true, wake: make(chan struct{}, 1), changed: make(chan struct{}, 1),
		state: "starting", since: time.Now()}
}

// Changes signals, without blocking, every time sync's state changes, so a
// tray icon can follow it without asking over and over.
func (s *Supervisor) Changes() <-chan struct{} { return s.changed }

func (s *Supervisor) signalChange() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

func (s *Supervisor) setState(state, lastError string) {
	s.mu.Lock()
	changed := s.state != state
	if changed {
		s.state = state
		s.since = time.Now()
	}
	if lastError != "" {
		s.lastError = lastError
	}
	s.mu.Unlock()
	if changed {
		s.signalChange()
	}
}

// Status reports the supervisor's view of sync.
func (s *Supervisor) Status() Status {
	s.mu.Lock()
	st := Status{State: s.state, Since: s.since, LastError: s.lastError, Restarts: s.restarts, PausedFor: s.pausedFor, History: s.history}
	st.Recent = append([]Event(nil), s.events...)
	running := s.proc != nil
	s.mu.Unlock()
	st.Delegate = running && s.socketReady()
	return st
}

// Run keeps sync alive until ctx ends. It waits while the store is not paired,
// restarts sync with backoff when it exits unexpectedly, and stays down while
// an exclusive operation holds the store.
func (s *Supervisor) Run(ctx context.Context) {
	// A pairing runs wacli in its own process group, which a stop of the
	// daemon does not reach: end it explicitly, and wait for it, so it never
	// outlives the daemon holding the store lock.
	defer s.pairWG.Wait()
	defer s.CancelPairing()
	s.reclaimOrphan()
	backoff := 2 * time.Second
	for ctx.Err() == nil {
		s.mu.Lock()
		want := s.want
		s.mu.Unlock()
		if !want {
			s.waitWake(ctx, time.Minute)
			continue
		}

		if paired, err := s.paired(ctx); err != nil || !paired {
			if err != nil {
				s.setState("error", err.Error())
			} else {
				s.setState("not_paired", "")
			}
			s.waitWake(ctx, 15*time.Second)
			continue
		}

		started := time.Now()
		err := s.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		want = s.want
		if want {
			s.restarts++
		}
		s.mu.Unlock()
		if !want {
			continue
		}
		if err != nil {
			s.mu.Lock()
			reason := s.lastError
			s.mu.Unlock()
			if reason == "" {
				reason = err.Error()
			}
			s.setState("exited", reason)
			s.logger.Warn("wacli sync exited", "error", err, "reason", reason)
		} else {
			s.setState("exited", "")
		}
		if time.Since(started) > 5*time.Minute {
			backoff = 2 * time.Second
		}
		if s.reclaimOrphan() {
			backoff = 2 * time.Second
		}
		s.waitWake(ctx, backoff)
		backoff = min(backoff*2, time.Minute)
	}
}

func (s *Supervisor) waitWake(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-s.wake:
	case <-t.C:
	}
}

func (s *Supervisor) nudge() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Supervisor) paired(ctx context.Context) (bool, error) {
	var status struct {
		Authenticated bool `json:"authenticated"`
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := s.cli.Decode(cctx, &status, "--read-only", "auth", "status"); err != nil {
		return false, err
	}
	return status.Authenticated, nil
}

// runOnce starts sync and blocks until it exits.
func (s *Supervisor) runOnce(ctx context.Context) error {
	cmd := exec.Command(s.cli.Bin, "sync", "--follow", "--events",
		"--max-reconnect", "0", "--presence-mode", "quiet", "--refresh-contacts", "--refresh-groups")
	cmd.Env = append(os.Environ(), "WACLI_STORE_DIR="+s.cli.StoreDir, "NO_COLOR=1")
	// Its own process group, so a stop reaches every process it started, and a
	// bounded wait for its pipes, so a stray child cannot hang the supervisor.
	platform.Prepare(cmd)
	cmd.WaitDelay = 5 * time.Second
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	cmd.Stdout = io.Discard

	s.mu.Lock()
	if !s.want {
		s.mu.Unlock()
		return nil
	}
	if err := cmd.Start(); err != nil {
		s.mu.Unlock()
		return fmt.Errorf("could not start wacli sync: %w", err)
	}
	if err := platform.Started(cmd); err != nil {
		s.logger.Warn("wacli sync is not tied to this process", "error", err)
	}
	exited := make(chan struct{})
	s.proc, s.exited = cmd.Process, exited
	s.state, s.since = "starting", time.Now()
	s.mu.Unlock()
	s.signalChange()

	// A stop of the daemon interrupts sync, and gives it the same 20 seconds
	// a pause does to close its store before it is killed.
	stop := context.AfterFunc(ctx, func() {
		_ = platform.Interrupt(cmd.Process.Pid)
		select {
		case <-exited:
		case <-time.After(20 * time.Second):
			_ = platform.Kill(cmd.Process.Pid)
		}
	})
	defer stop()

	s.readEvents(stderr)
	err = cmd.Wait()

	s.mu.Lock()
	s.proc, s.exited = nil, nil
	s.mu.Unlock()
	close(exited)
	return err
}

func (s *Supervisor) readEvents(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var ev struct {
			Event string         `json:"event"`
			Data  map[string]any `json:"data"`
		}
		if json.Unmarshal(scanner.Bytes(), &ev) != nil || ev.Event == "" {
			continue
		}
		if ev.Event == "progress" || ev.Event == "history_sync" {
			// Frequent and uninteresting once seen; keep only the latest.
			s.mu.Lock()
			if n := len(s.events); n > 0 && s.events[n-1].Event == ev.Event {
				s.events[n-1] = Event{At: time.Now(), Event: ev.Event, Data: ev.Data}
				s.mu.Unlock()
				continue
			}
			s.mu.Unlock()
		}
		s.record(Event{At: time.Now(), Event: ev.Event, Data: ev.Data})
		switch ev.Event {
		case "history_sync":
			if n, ok := ev.Data["conversations"].(float64); ok {
				s.mu.Lock()
				s.history.Conversations += int(n)
				s.history.LastAt = time.Now()
				s.mu.Unlock()
			}
		case "progress":
			if n, ok := ev.Data["messages_synced"].(float64); ok {
				s.mu.Lock()
				s.history.Messages = int64(n)
				s.history.LastAt = time.Now()
				s.mu.Unlock()
			}
		case "error":
			// wacli's own words say more than the exit status that follows.
			if msg, ok := ev.Data["message"].(string); ok {
				s.mu.Lock()
				s.lastError = strings.SplitN(msg, "\n", 2)[0]
				s.mu.Unlock()
			}
		case "connected":
			s.mu.Lock()
			s.lastError = ""
			s.mu.Unlock()
			s.setState("connected", "")
		case "disconnected", "reconnecting":
			s.setState("reconnecting", "")
		case "logged_out":
			s.setState("logged_out", "WhatsApp revoked this linked device; pair again with `wacli auth`")
		case "stream_replaced":
			s.setState("reconnecting", "another client took over this session")
		}
	}
}

func (s *Supervisor) record(ev Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
	if len(s.events) > maxEvents {
		s.events = s.events[len(s.events)-maxEvents:]
	}
}

func (s *Supervisor) socketPath() string {
	return filepath.Join(s.cli.StoreDir, ".send.sock")
}

func (s *Supervisor) socketReady() bool {
	conn, err := net.DialTimeout("unix", s.socketPath(), 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// stopSync stops the sync process and keeps it down until startSync.
func (s *Supervisor) stopSync(reason string) {
	s.mu.Lock()
	s.want = false
	s.pausedFor = reason
	proc, exited := s.proc, s.exited
	s.state, s.since = "paused", time.Now()
	s.mu.Unlock()
	s.signalChange()
	if proc == nil {
		return
	}
	_ = platform.Interrupt(proc.Pid)
	select {
	case <-exited:
	case <-time.After(20 * time.Second):
		_ = platform.Kill(proc.Pid)
		<-exited
	}
}

// startSync lets sync run again and waits until it has been launched (or the
// loop has decided it cannot run). Exclusive calls it while still holding the
// operations lock, so a send queued behind a pause cannot slip in, take the
// store lock itself, and make the restarting sync fail on it.
func (s *Supervisor) startSync() {
	s.mu.Lock()
	s.want = true
	s.pausedFor = ""
	s.mu.Unlock()
	s.nudge()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		launched := s.proc != nil || s.state != "paused"
		s.mu.Unlock()
		if launched {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Delegated runs an operation that wacli hands to the running sync process.
// When sync is up it waits for its delegate socket, so a send issued seconds
// after a restart does not fail on the lock.
func (s *Supervisor) Delegated(ctx context.Context, fn func(context.Context) error) error {
	s.ops.RLock()
	defer s.ops.RUnlock()
	s.awaitDelegate(ctx)
	err := fn(ctx)
	if IsLockError(err) {
		// Sync was between holding the lock and opening its socket.
		s.awaitDelegate(ctx)
		err = fn(ctx)
	}
	return err
}

func (s *Supervisor) awaitDelegate(ctx context.Context) {
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		s.mu.Lock()
		running := s.proc != nil
		s.mu.Unlock()
		if !running || s.socketReady() {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// Exclusive stops sync, runs an operation that needs the store lock, and
// starts sync again whatever the outcome.
func (s *Supervisor) Exclusive(ctx context.Context, reason string, fn func(context.Context) error) error {
	s.ops.Lock()
	defer s.ops.Unlock()
	s.stopSync(reason)
	defer s.startSync()
	err := fn(ctx)
	if IsLockError(err) {
		// The lock is an flock the kernel releases as sync exits; give a
		// straggling exit a moment before concluding someone else holds it.
		time.Sleep(time.Second)
		err = fn(ctx)
	}
	if IsLockError(err) {
		return errors.Join(err, errors.New("the store is locked by a wacli process this gateway does not own; stop any `wacli sync` you started by hand"))
	}
	return err
}
