package main

import (
	"context"
	"errors"
	"html"
	"os"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/appconfig"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/daemon"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/panel"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/platform"
)

// The app is the panel's Host: what the Configurações page changes.
var _ panel.Host = (*App)(nil)

func (a *App) Settings() panel.HostSettings {
	c := a.config()
	return panel.HostSettings{
		Autostart:   c.Autostart,
		CloseToTray: c.CloseToTray,
		CanHide:     a.canHide,
		PortLocked:  a.portLocked,
		DataDir:     a.dataDir,
		Version:     version,
	}
}

// SetAutostart registers the app to open at login, in the tray, or stops it.
func (a *App) SetAutostart(on bool) error {
	var err error
	switch appImage := os.Getenv("APPIMAGE"); {
	case appImage != "":
		// An AppImage runs from a temporary mount: login must start the
		// AppImage file itself.
		err = xdgAutostart(on, appImage)
	case on:
		err = a.wails.Autostart.EnableWithOptions(application.AutostartOptions{Identifier: bundleID, Arguments: []string{"--hidden"}})
	default:
		err = a.wails.Autostart.Disable()
	}
	if err != nil {
		return err
	}
	if err := a.updateConfig(func(c *appconfig.Config) { c.Autostart = on }); err != nil {
		return err
	}
	a.tray.setAutostart(on)
	return nil
}

func (a *App) SetCloseToTray(on bool) error {
	return a.updateConfig(func(c *appconfig.Config) { c.CloseToTray = on })
}

// SetPort moves the MCP and writes the port into config.json, where the
// bridge finds it.
func (a *App) SetPort(port int) error {
	if a.portLocked {
		return errors.New("a porta está definida pela variável de ambiente WHATSAPP_MCP_PORT")
	}
	d := a.current()
	if d == nil {
		return errors.New("o WhatsApp MCP ainda está iniciando")
	}
	if err := d.SetPort(port); err != nil {
		var inUse daemon.ErrPortInUse
		if errors.As(err, &inUse) {
			return errors.New("a porta " + itoa(port) + " já está em uso por outro programa")
		}
		return err
	}
	return a.updateConfig(func(c *appconfig.Config) { c.Port = port })
}

func (a *App) OpenURL(url string) error { return a.wails.Browser.OpenURL(url) }

func (a *App) OpenDataFolder() error {
	if err := os.MkdirAll(a.dataDir, 0o700); err != nil {
		return err
	}
	return platform.Open(a.dataDir)
}

func (a *App) Update() panel.UpdateState { return a.updates.state() }

func (a *App) CheckUpdate() { a.updates.check() }

func (a *App) InstallUpdate() error { return a.updates.install() }

// EraseEverything unlinks this computer from WhatsApp, stops the gateway,
// deletes the data folder and the login item, and quits.
func (a *App) EraseEverything() error {
	d := a.current()
	if d != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		err := d.Supervisor().Logout(ctx)
		cancel()
		if err != nil {
			a.logger.Warn("WhatsApp logout failed; erasing anyway", "error", err)
		}
	}
	_ = a.SetAutostart(false)
	go func() {
		// Let the page say it is done before the window goes.
		time.Sleep(1500 * time.Millisecond)
		a.shutdown()
		if err := os.RemoveAll(a.dataDir); err != nil {
			a.logger.Error("the data folder could not be deleted", "error", err)
		}
		a.quit()
	}()
	return nil
}

// setMenu is the application menu, at the top of the screen on macOS.
// Windows and Linux would draw it inside the window, where the panel's own
// navigation already is.
func (a *App) setMenu() {
	if runtime.GOOS != "darwin" {
		return
	}
	menu := a.wails.NewMenu()
	app := menu.AddSubmenu(platform.AppName)
	app.AddRole(application.About)
	app.AddSeparator()
	app.Add("Configurações…").SetAccelerator("CmdOrCtrl+,").OnClick(func(*application.Context) { a.showWindow("/configuracoes") })
	app.AddSeparator()
	app.AddRole(application.Hide)
	app.AddRole(application.HideOthers)
	app.AddRole(application.UnHide)
	app.AddSeparator()
	app.Add("Sair do " + platform.AppName).SetAccelerator("CmdOrCtrl+q").OnClick(func(*application.Context) { a.quit() })
	menu.AddRole(application.EditMenu)
	menu.AddRole(application.WindowMenu)
	a.wails.Menu.SetApplicationMenu(menu)
}

func htmlEscape(s string) string { return html.EscapeString(s) }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
