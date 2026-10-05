package main

import (
	"errors"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/panel"
)

// updates looks for a new version of the app and installs it.
type updates struct {
	app *App
}

func newUpdates(a *App) *updates { return &updates{app: a} }

func (u *updates) state() panel.UpdateState {
	return panel.UpdateState{State: "idle", Current: version}
}

func (u *updates) check() {}

func (u *updates) install() error { return errors.New("nenhuma atualização disponível") }

func (u *updates) loop() {}
