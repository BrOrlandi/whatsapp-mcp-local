package wacli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/platform"
)

// Pairing is the state of linking this machine to a WhatsApp account, shown
// by the control panel while `wacli auth` runs behind it.
type Pairing struct {
	State         string    `json:"state"` // idle, starting, qr, code, syncing, done, error, cancelled
	QR            string    `json:"-"`
	QRVersion     int       `json:"qr_version"`
	Code          string    `json:"code,omitempty"`
	Phone         string    `json:"phone,omitempty"`
	Synced        int64     `json:"messages_synced"`
	Conversations int       `json:"conversations"`
	Error         string    `json:"error,omitempty"`
	StartedAt     time.Time `json:"started_at,omitempty"`
	UpdatedAt     time.Time `json:"updated_at,omitempty"`
}

type pairer struct {
	mu     sync.Mutex
	state  Pairing
	cancel func()
	// gen numbers each pairing. Switching from QR to a phone code cancels one
	// pairing and starts the next at once; the cancelled one keeps winding down
	// for a moment, and must not write its ending over the new one's state.
	gen int
}

var ErrPairingRunning = errors.New("a pairing is already in progress")

// HandoverAfter is how long auth keeps running once the phone has linked,
// before sync takes the rest of the history over.
var HandoverAfter = 20 * time.Second

// Pairing reports the current pairing, if any.
func (s *Supervisor) Pairing() Pairing {
	s.pair.mu.Lock()
	defer s.pair.mu.Unlock()
	if s.pair.state.State == "" {
		return Pairing{State: "idle"}
	}
	return s.pair.state
}

// updatePairing changes the state of pairing number gen, and does nothing
// once a newer pairing has started.
func (s *Supervisor) updatePairing(gen int, f func(*Pairing)) {
	s.pair.mu.Lock()
	defer s.pair.mu.Unlock()
	if gen != s.pair.gen {
		return
	}
	f(&s.pair.state)
	s.pair.state.UpdatedAt = time.Now()
}

// StartPairing links this machine to WhatsApp in the background: it stops
// sync, runs `wacli auth` (QR code, or a code typed on the phone when phone
// is given), lets its first history sync run until idle, and starts sync
// again. Progress is read with Pairing.
func (s *Supervisor) StartPairing(phone string) error {
	s.pair.mu.Lock()
	switch s.pair.state.State {
	case "starting", "qr", "code", "syncing":
		s.pair.mu.Unlock()
		return ErrPairingRunning
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	s.pair.gen++
	gen := s.pair.gen
	s.pair.cancel = cancel
	s.pair.state = Pairing{State: "starting", Phone: strings.TrimSpace(phone), StartedAt: time.Now(), UpdatedAt: time.Now()}
	s.pair.mu.Unlock()

	s.pairWG.Add(1)
	go func() {
		defer s.pairWG.Done()
		defer cancel()
		err := s.Exclusive(ctx, "pairing", func(ctx context.Context) error { return s.runAuth(ctx, gen, phone) })
		s.updatePairing(gen, func(p *Pairing) {
			switch {
			case p.State == "cancelled":
			case err == nil:
				p.State, p.QR, p.Code, p.Error = "done", "", "", ""
			default:
				p.State, p.QR, p.Code, p.Error = "error", "", "", err.Error()
			}
		})
		s.logger.Info("pairing finished", "state", s.Pairing().State, "error", err)
	}()
	return nil
}

// CancelPairing stops a pairing that has not finished.
func (s *Supervisor) CancelPairing() {
	s.pair.mu.Lock()
	cancel := s.pair.cancel
	if st := s.pair.state.State; st == "starting" || st == "qr" || st == "code" || st == "syncing" {
		s.pair.state.State = "cancelled"
		s.pair.state.QR, s.pair.state.Code = "", ""
	}
	s.pair.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Supervisor) runAuth(ctx context.Context, gen int, phone string) error {
	args := []string{"auth", "--events", "--idle-exit", "45s"}
	if phone = strings.TrimSpace(phone); phone != "" {
		args = append(args, "--phone", phone)
	}
	cmd := exec.Command(s.cli.Bin, args...)
	cmd.Env = append(os.Environ(), "WACLI_STORE_DIR="+s.cli.StoreDir, "NO_COLOR=1")
	platform.Prepare(cmd)
	cmd.WaitDelay = 5 * time.Second
	cmd.Stdout = io.Discard
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start wacli auth: %w", err)
	}
	if err := platform.Started(cmd); err != nil {
		s.logger.Warn("wacli auth is not tied to this process", "error", err)
	}
	// An interrupt that is ignored for 20 seconds becomes a kill, so a stuck
	// auth can never hold the store, or the daemon's shutdown, forever.
	exited := make(chan struct{})
	defer close(exited)
	interrupt := func() {
		_ = platform.Interrupt(cmd.Process.Pid)
		go func() {
			select {
			case <-exited:
			case <-time.After(20 * time.Second):
				_ = platform.Kill(cmd.Process.Pid)
			}
		}()
	}
	stop := context.AfterFunc(ctx, interrupt)
	defer stop()

	// Once the phone has linked the device, the pairing's work is done. The
	// history the phone goes on to send arrives just as well through sync,
	// which WhatsApp delivers it to on reconnect, so auth hands over shortly
	// after instead of holding the store, and every send, until the whole
	// history is in: for a large account that is well over half an hour.
	var connected atomic.Bool
	var handover *time.Timer
	onConnected := func() {
		if !connected.Swap(true) {
			handover = time.AfterFunc(HandoverAfter, interrupt)
		}
	}
	lastLine := s.readPairingEvents(gen, stderr, onConnected)
	err = cmd.Wait()
	if handover != nil {
		handover.Stop()
	}
	if connected.Load() {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		var status struct {
			Authenticated bool `json:"authenticated"`
		}
		if s.cli.Decode(cctx, &status, "--read-only", "auth", "status") == nil && status.Authenticated {
			return nil
		}
	}
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return errors.New("pairing took longer than 30 minutes and was stopped")
		}
		return errors.New("pairing was cancelled")
	}
	if err != nil {
		if lastLine != "" {
			return errors.New(lastLine)
		}
		return err
	}
	// A clean exit is not proof of pairing: confirm the store is authenticated.
	var status struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := s.cli.Decode(ctx, &status, "--read-only", "auth", "status"); err != nil {
		return err
	}
	if !status.Authenticated {
		return errors.New("wacli auth ended without pairing; try again")
	}
	return nil
}

// readPairingEvents follows wacli auth's events and returns the last line it
// could not parse, which is where wacli writes a fatal error.
func (s *Supervisor) readPairingEvents(gen int, r io.Reader, onConnected func()) string {
	var last string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		var ev struct {
			Event string         `json:"event"`
			Data  map[string]any `json:"data"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil || ev.Event == "" {
			if line != "" {
				last = line
			}
			continue
		}
		s.record(Event{At: time.Now(), Event: "auth:" + ev.Event, Data: redactQR(ev)})
		switch ev.Event {
		case "qr_code":
			code, _ := ev.Data["code"].(string)
			s.updatePairing(gen, func(p *Pairing) {
				if p.State != "cancelled" {
					p.State, p.QR = "qr", code
					p.QRVersion++
				}
			})
		case "pair_code":
			code, _ := ev.Data["code"].(string)
			s.updatePairing(gen, func(p *Pairing) {
				if p.State != "cancelled" {
					p.State, p.Code = "code", code
				}
			})
		case "connected":
			onConnected()
			s.updatePairing(gen, func(p *Pairing) {
				if p.State != "cancelled" {
					p.State, p.QR, p.Code = "syncing", "", ""
				}
			})
		case "progress", "idle_exit":
			if n, ok := ev.Data["messages_synced"].(float64); ok {
				s.updatePairing(gen, func(p *Pairing) { p.Synced = int64(n) })
			}
		case "history_sync":
			if n, ok := ev.Data["conversations"].(float64); ok {
				s.updatePairing(gen, func(p *Pairing) { p.Conversations += int(n) })
			}
		case "error":
			if msg, ok := ev.Data["message"].(string); ok {
				last = msg
			}
		}
	}
	return last
}

// redactQR keeps pairing secrets out of the status report.
func redactQR(ev struct {
	Event string         `json:"event"`
	Data  map[string]any `json:"data"`
}) map[string]any {
	if ev.Event == "qr_code" || ev.Event == "pair_code" {
		return nil
	}
	return ev.Data
}

// Logout unlinks this machine from WhatsApp.
func (s *Supervisor) Logout(ctx context.Context) error {
	return s.Exclusive(ctx, "logout", func(ctx context.Context) error {
		_, err := s.cli.Run(ctx, "auth", "logout")
		return err
	})
}

// Account is the WhatsApp account this store is paired with.
type Account struct {
	Authenticated bool   `json:"authenticated"`
	Phone         string `json:"phone,omitempty"`
	JID           string `json:"linked_jid,omitempty"`
}

func (s *Supervisor) Account(ctx context.Context) (Account, error) {
	var a Account
	err := s.cli.Decode(ctx, &a, "--read-only", "auth", "status")
	return a, err
}
