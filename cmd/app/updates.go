package main

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/brand"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/panel"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/platform"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/updater"
)

// updates looks for a new version once a day, tells the person, and installs
// it when they ask.
type updates struct {
	app *App
	u   *updater.Updater

	mu        sync.Mutex
	announced string
}

func newUpdates(a *App) *updates {
	return &updates{app: a, u: &updater.Updater{
		Repo:    strings.TrimPrefix(brand.RepositoryURL, "https://github.com/"),
		Version: strings.TrimPrefix(version, "v"),
		Logger:  a.logger,
		Dir:     filepath.Join(a.dataDir, "updates"),
		Target:  updater.DetectTarget(platform.Executable()),
	}}
}

func (u *updates) state() panel.UpdateState {
	s := u.u.State()
	return panel.UpdateState{State: s.Phase, Current: version, Latest: s.Latest, Page: s.Page, Progress: s.Progress, Error: s.Error, CheckedAt: s.CheckedAt}
}

// check looks now, in the background.
func (u *updates) check() {
	go func() {
		found, _ := u.u.Check(context.Background())
		if found {
			u.announce()
		}
	}()
	// Let the page see "checking" rather than the state before.
	time.Sleep(150 * time.Millisecond)
}

// install downloads, checks and installs the new version in the background,
// then quits so the new one can start. The page follows the progress.
func (u *updates) install() error {
	if st := u.u.State(); st.Phase == updater.Downloading || st.Phase == updater.Ready {
		return updater.ErrBusy
	}
	started := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		close(started)
		if err := u.u.Apply(ctx); err != nil {
			return
		}
		u.app.logger.Info("restarting into the new version", "version", u.u.State().Latest)
		time.Sleep(time.Second)
		u.app.quit()
	}()
	<-started
	time.Sleep(100 * time.Millisecond)
	return nil
}

// loop checks a minute after start and then once a day.
func (u *updates) loop() {
	if !u.u.Enabled() {
		return
	}
	time.Sleep(time.Minute)
	for {
		if found, _ := u.u.Check(context.Background()); found {
			u.announce()
		}
		time.Sleep(24 * time.Hour)
	}
}

// announce tells the person once per version: a notification, and a line in
// the tray's menu.
func (u *updates) announce() {
	s := u.u.State()
	u.mu.Lock()
	seen := u.announced == s.Latest
	u.announced = s.Latest
	u.mu.Unlock()
	if seen {
		return
	}
	u.app.tray.offerUpdate(s.Latest)
	u.app.notify("update-"+s.Latest, "Versão "+s.Latest+" do WhatsApp MCP disponível", "Abra Configurações para instalar.")
}
