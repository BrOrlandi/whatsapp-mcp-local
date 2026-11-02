package panel

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"
)

// Claude Desktop keeps its configuration in memory and writes it back to the
// file now and then while it is open, from that copy: a server added to the
// file while it runs is gone a few seconds later, before it ever loads it.
// So the server is written with the app closed: the panel closes Claude
// Desktop, writes the configuration, and opens it again, which is also the
// restart that makes it load the server.

// errDesktopStillOpen is Claude Desktop still running after being asked to
// close.
var errDesktopStillOpen = errors.New("Claude Desktop did not close")

// addDesktopReopening adds the server to Claude Desktop with the app closed,
// and opens it again. reopened says the app was (re)opened by the panel;
// when it could not be closed, the server is written anyway and watched, so
// it is written again once the person closes the app.
func (p *Panel) addDesktopReopening(ctx context.Context, replace bool) (reopened bool, err error) {
	// A question for the person comes before closing anything.
	if info := p.desktopInfo(); info.Other != "" && !replace {
		return false, errConflict{"Claude Desktop", info.Other}
	}
	if !desktopControl {
		return false, p.addClaudeDesktop(replace)
	}
	target := desktopLaunchTarget()
	if desktopRunning() {
		if err := quitDesktop(ctx); err != nil {
			slog.Warn("could not close Claude Desktop", "error", err)
			if err := p.addClaudeDesktop(replace); err != nil {
				return false, err
			}
			p.watchDesktop()
			return false, nil
		}
	}
	if err := p.addClaudeDesktop(replace); err != nil {
		_ = openDesktop(target)
		return false, err
	}
	if err := openDesktop(target); err != nil {
		slog.Warn("could not open Claude Desktop", "error", err)
		p.watchDesktop()
		return false, nil
	}
	p.watchDesktop()
	return true, nil
}

var desktopWatching atomic.Bool

// watchDesktop writes the server again whenever Claude Desktop closes without
// it in the file, for a quarter of an hour: closing is when the next start
// will read the file, and an app left open may have written it away.
func (p *Panel) watchDesktop() {
	if !desktopControl || !desktopWatching.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer desktopWatching.Store(false)
		was := desktopRunning()
		deadline := time.Now().Add(15 * time.Minute)
		for time.Now().Before(deadline) {
			time.Sleep(2 * time.Second)
			now := desktopRunning()
			if was && !now {
				if info := p.desktopInfo(); !info.Configured && info.Other == "" {
					if err := p.addClaudeDesktop(false); err != nil {
						slog.Warn("could not write Claude Desktop's configuration again", "error", err)
					} else {
						slog.Info("Claude Desktop had dropped the WhatsApp server; written again after it closed")
					}
				}
			}
			was = now
		}
	}()
}

// waitDesktopClosed waits for Claude Desktop to be gone.
func waitDesktopClosed(ctx context.Context, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for desktopRunning() {
		if time.Now().After(deadline) {
			return errDesktopStillOpen
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	// Let it finish releasing its files.
	time.Sleep(500 * time.Millisecond)
	return nil
}
