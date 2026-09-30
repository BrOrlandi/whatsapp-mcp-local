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
	"syscall"
	"time"
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
}

var ErrPairingRunning = errors.New("a pairing is already in progress")

// Pairing reports the current pairing, if any.
func (s *Supervisor) Pairing() Pairing {
	s.pair.mu.Lock()
	defer s.pair.mu.Unlock()
	if s.pair.state.State == "" {
		return Pairing{State: "idle"}
	}
	return s.pair.state
}

func (s *Supervisor) updatePairing(f func(*Pairing)) {
	s.pair.mu.Lock()
	defer s.pair.mu.Unlock()
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
	s.pair.cancel = cancel
	s.pair.state = Pairing{State: "starting", Phone: strings.TrimSpace(phone), StartedAt: time.Now(), UpdatedAt: time.Now()}
	s.pair.mu.Unlock()

	s.pairWG.Add(1)
	go func() {
		defer s.pairWG.Done()
		defer cancel()
		err := s.Exclusive(ctx, "pairing", func(ctx context.Context) error { return s.runAuth(ctx, phone) })
		s.updatePairing(func(p *Pairing) {
			switch {
			case p.State == "cancelled":
			case err == nil:
				p.State, p.QR, p.Code = "done", "", ""
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

func (s *Supervisor) runAuth(ctx context.Context, phone string) error {
	args := []string{"auth", "--events", "--idle-exit", "45s"}
	if phone = strings.TrimSpace(phone); phone != "" {
		args = append(args, "--phone", phone)
	}
	cmd := exec.Command(s.cli.Bin, args...)
	cmd.Env = append(os.Environ(), "WACLI_STORE_DIR="+s.cli.StoreDir, "NO_COLOR=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 5 * time.Second
	cmd.Stdout = io.Discard
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start wacli auth: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { signalGroup(cmd.Process, syscall.SIGINT) })
	defer stop()

	lastLine := s.readPairingEvents(stderr)
	err = cmd.Wait()
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
func (s *Supervisor) readPairingEvents(r io.Reader) string {
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
			s.updatePairing(func(p *Pairing) {
				if p.State != "cancelled" {
					p.State, p.QR = "qr", code
					p.QRVersion++
				}
			})
		case "pair_code":
			code, _ := ev.Data["code"].(string)
			s.updatePairing(func(p *Pairing) {
				if p.State != "cancelled" {
					p.State, p.Code = "code", code
				}
			})
		case "connected":
			s.updatePairing(func(p *Pairing) {
				if p.State != "cancelled" {
					p.State, p.QR, p.Code = "syncing", "", ""
				}
			})
		case "progress", "idle_exit":
			if n, ok := ev.Data["messages_synced"].(float64); ok {
				s.updatePairing(func(p *Pairing) { p.Synced = int64(n) })
			}
		case "history_sync":
			if n, ok := ev.Data["conversations"].(float64); ok {
				s.updatePairing(func(p *Pairing) { p.Conversations += int(n) })
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
