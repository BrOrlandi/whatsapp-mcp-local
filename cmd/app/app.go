package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/appconfig"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/daemon"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/legacy"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/mcp"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/panel"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/platform"
)

// bundleID names the app to the system: single instance, login item,
// notifications.
const bundleID = "com.brorlandi.whatsapp-mcp"

type App struct {
	logger   *slog.Logger
	logPath  string
	dataDir  string
	storeDir string
	wacliBin string
	bridge   string
	hidden   bool

	cfgMu      sync.Mutex
	cfg        appconfig.Config
	portLocked bool

	wails    *application.App
	window   *application.WebviewWindow
	tray     *tray
	notifier *notifications.NotificationService
	handler  swapHandler
	updates  *updates

	dmu    sync.Mutex
	daemon *daemon.Daemon
	follow context.CancelFunc

	// canHide is false where there is no tray to hide the window into.
	canHide  bool
	quitting atomic.Bool
}

func newApp(hidden bool) (*App, error) {
	dataDir, err := platform.DataDir()
	if err != nil {
		return nil, err
	}
	logDir, err := platform.LogDir()
	if err != nil {
		return nil, err
	}
	logger, logPath, err := openLog(logDir)
	if err != nil {
		return nil, err
	}
	mcp.Version = version
	a := &App{logger: logger, logPath: logPath, dataDir: dataDir, hidden: hidden}
	a.storeDir = os.Getenv("WACLI_STORE_DIR")
	if a.storeDir == "" {
		a.storeDir = filepath.Join(dataDir, "wacli")
	}
	a.wacliBin = os.Getenv("WACLI_BIN")
	if a.wacliBin == "" {
		a.wacliBin = platform.Bundled("wacli")
	}
	if a.wacliBin == "" {
		a.wacliBin = platform.LookPath("wacli") // a development build, outside the bundle
	}
	a.bridge = platform.Bundled("whatsapp-mcp-bridge")
	if a.cfg, err = appconfig.Load(dataDir); err != nil {
		logger.Warn("config.json could not be read; using the defaults", "error", err)
	}
	if port, ok, err := appconfig.PortFromEnv(); ok && err == nil {
		a.cfg.Port, a.portLocked = port, true
	}
	a.canHide = trayAvailable()
	a.updates = newUpdates(a)
	logger.Info("starting", "version", version, "data", dataDir, "wacli", a.wacliBin, "bridge", a.bridge, "hidden", hidden)
	return a, nil
}

func (a *App) config() appconfig.Config {
	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()
	return a.cfg
}

// updateConfig changes config.json. A port fixed by the environment is the
// app's to use, never to keep.
func (a *App) updateConfig(f func(*appconfig.Config)) error {
	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()
	f(&a.cfg)
	saved := a.cfg
	if a.portLocked {
		saved.Port = appconfig.DefaultPort
		if disk, err := appconfig.Load(a.dataDir); err == nil {
			saved.Port = disk.Port
		}
	}
	return appconfig.Save(a.dataDir, saved)
}

func (a *App) run() error {
	a.notifier = notifications.New()
	a.wails = application.New(application.Options{
		Name:        platform.AppName,
		Description: "Seu WhatsApp nas suas ferramentas de IA, neste computador.",
		Icon:        appIcon,
		Logger:      a.logger,
		Services:    []application.Service{application.NewService(a.notifier)},
		Assets:      application.AssetOptions{Handler: &a.handler, DisableLogging: true},
		Mac: application.MacOptions{
			// The app starts out of the Dock; the window brings it in.
			ActivationPolicy: application.ActivationPolicyAccessory,
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Windows: application.WindowsOptions{DisableQuitOnLastWindowClosed: true},
		Linux:   application.LinuxOptions{DisableQuitOnLastWindowClosed: true, ProgramName: "whatsapp-mcp"},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               bundleID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { a.showWindow("") },
		},
		OnShutdown: a.shutdown,
	})

	a.prepare()
	a.window = a.wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     platform.AppName,
		Width:     1000,
		Height:    760,
		MinWidth:  720,
		MinHeight: 560,
		URL:       "/",
		// On macOS the window waits until the app knows whether the system
		// opened it at login; elsewhere --hidden says so.
		Hidden:           true,
		BackgroundColour: application.NewRGB(246, 248, 247),
	})
	a.window.RegisterHook(events.Common.WindowClosing, a.onClose)
	a.setMenu()
	a.tray = newTray(a)

	a.wails.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) { a.showWindow("") })
	a.wails.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		if a.hidden || launchedAtLogin() {
			a.logger.Info("opened at login: staying in the tray")
			return
		}
		a.showWindow("")
	})
	go a.firstRun()
	go a.updates.loop()
	return a.wails.Run()
}

// prepare decides what the window shows first: an earlier installation to
// move, or the panel.
func (a *App) prepare() {
	if a.config().Legacy == "" && !paired(a.storeDir) && os.Getenv("WACLI_STORE_DIR") == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		offer := legacy.Find(ctx, a.wacliBin)
		cancel()
		if offer != nil {
			a.logger.Info("found an earlier installation", "store", offer.StoreDir, "service", offer.Service)
			a.handler.set(panel.MigrationHandler(panel.MigrationOffer{
				Name: offer.Name, Phone: offer.Phone, StoreDir: offer.StoreDir, DataDir: offer.DataDir, Service: offer.Service != "",
			}, func(keep bool) error { return a.decideLegacy(offer, keep) }))
			return
		}
	}
	a.startDaemon()
}

func paired(storeDir string) bool {
	_, err := os.Stat(filepath.Join(storeDir, "session.db"))
	return err == nil
}

// decideLegacy moves the earlier installation in, or leaves it be, and then
// starts the gateway.
func (a *App) decideLegacy(offer *legacy.Install, keep bool) error {
	if keep {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		err := legacy.Move(ctx, offer, a.storeDir, a.dataDir)
		cancel()
		if err != nil && !paired(a.storeDir) {
			a.logger.Error("moving the earlier installation failed", "error", err)
			return err
		}
		if err != nil {
			a.logger.Warn("the earlier installation moved, with problems", "error", err)
		}
	}
	if err := a.updateConfig(func(c *appconfig.Config) {
		c.Legacy = "declined"
		if keep {
			// The port the earlier installation served on, so Claude Code's
			// address keeps working.
			c.Legacy = "migrated"
			if offer.Port > 0 && !a.portLocked {
				c.Port = offer.Port
			}
		}
	}); err != nil {
		a.logger.Warn("config.json could not be saved", "error", err)
	}
	a.startDaemon()
	if keep {
		if d := a.current(); d != nil {
			if err := d.Panel().AdoptDesktop(); err != nil {
				a.logger.Warn("Claude Desktop could not be pointed at the app", "error", err)
			}
		}
	}
	return nil
}

func (a *App) startDaemon() {
	desktop := panel.DesktopCommand{Command: a.bridge}
	if desktop.Command == "" {
		// A development build: the bridge is built beside the app.
		desktop.Command = filepath.Join(filepath.Dir(platform.Executable()), platform.ExeName("whatsapp-mcp-bridge"))
	}
	d, err := daemon.Start(daemon.Config{
		Port: a.config().Port, Token: os.Getenv("WHATSAPP_MCP_TOKEN"),
		WacliBin: a.wacliBin, StoreDir: a.storeDir, DataDir: a.dataDir,
		LogPath: a.logPath, Version: version, Logger: a.logger, Desktop: desktop,
		Panel: func(p *panel.Panel) { p.Host = a },
	})
	if err != nil {
		a.logger.Error("the gateway did not start", "error", err)
		a.handler.set(startFailure(err, a.logPath))
		return
	}
	a.dmu.Lock()
	a.daemon = d
	ctx, cancel := context.WithCancel(context.Background())
	a.follow = cancel
	a.dmu.Unlock()
	a.handler.set(d.AppHandler())
	go a.followStatus(ctx, d)
}

func (a *App) current() *daemon.Daemon {
	a.dmu.Lock()
	defer a.dmu.Unlock()
	return a.daemon
}

// followStatus keeps the tray in step with the gateway, and tells the person
// when WhatsApp needs them.
func (a *App) followStatus(ctx context.Context, d *daemon.Daemon) {
	updates, stop := d.Watch()
	defer stop()
	last := ""
	for {
		select {
		case <-ctx.Done():
			return
		case st := <-updates:
			if a.tray != nil {
				a.tray.render(st)
			}
			if st.Sync == "logged_out" && last != "" && last != "logged_out" {
				a.notify("whatsapp-logged-out", "WhatsApp desconectado",
					"O celular desconectou este computador. Abra o WhatsApp MCP para conectar de novo.")
			}
			last = st.Sync
		}
	}
}

// firstRun turns "open at login" on, as the first window offers it checked;
// the checkbox there and in the tray turns it off.
func (a *App) firstRun() {
	if a.config().HasSeen("first-run") {
		return
	}
	if err := a.SetAutostart(a.config().Autostart); err != nil {
		a.logger.Warn("could not register the app to open at login", "error", err)
	}
	_ = a.updateConfig(func(c *appconfig.Config) { c.Seen = append(c.Seen, "first-run") })
}

func (a *App) onClose(e *application.WindowEvent) {
	if a.quitting.Load() {
		return
	}
	e.Cancel()
	switch {
	case !a.canHide:
		a.window.Minimise()
	case a.config().CloseToTray:
		a.hideWindow()
		if !a.config().HasSeen("close") {
			_ = a.updateConfig(func(c *appconfig.Config) { c.Seen = append(c.Seen, "close") })
			a.notify("close", "O WhatsApp MCP continua rodando",
				fmt.Sprintf("Ele fica na %s, e as ferramentas de IA continuam usando o WhatsApp. Para encerrar, use Sair.", trayPlace()))
		}
	default:
		go a.quit()
	}
}

// showWindow brings the window up, at path when one is given.
func (a *App) showWindow(path string) {
	if a.window == nil {
		return
	}
	application.InvokeSync(func() {
		showInDock(true)
		if path != "" {
			a.window.SetURL(path)
		}
		a.window.Show()
		a.window.UnMinimise()
		a.window.Focus()
	})
}

func (a *App) hideWindow() {
	application.InvokeSync(func() {
		a.window.Hide()
		showInDock(false)
	})
}

func (a *App) quit() {
	a.quitting.Store(true)
	a.wails.Quit()
}

// shutdown stops the gateway with care, whatever ends the app: Sair, Cmd+Q,
// logging out, shutting the computer down.
func (a *App) shutdown() {
	a.quitting.Store(true)
	a.dmu.Lock()
	d, cancel := a.daemon, a.follow
	a.daemon = nil
	a.dmu.Unlock()
	if cancel != nil {
		cancel()
	}
	if d != nil {
		a.logger.Info("stopping the gateway")
		d.Stop()
	}
	a.logger.Info("stopped")
}

// notify shows a system notification. It is best effort: the person may
// have turned them off, and a development build has no bundle to send from.
func (a *App) notify(id, title, body string) {
	go func() {
		if ok, err := a.notifier.CheckNotificationAuthorization(); err == nil && !ok {
			if ok, err = a.notifier.RequestNotificationAuthorization(); err != nil || !ok {
				return
			}
		}
		if err := a.notifier.SendNotification(notifications.NotificationOptions{ID: id + "-" + fmt.Sprint(time.Now().Unix()), Title: title, Body: body}); err != nil {
			a.logger.Debug("notification not sent", "error", err)
		}
	}()
}

func trayPlace() string {
	switch runtime.GOOS {
	case "darwin":
		return "barra de menus"
	case "windows":
		return "área de notificação"
	}
	return "bandeja do sistema"
}

// swapHandler is what the window loads: the migration page first, when
// there is one, then the panel.
type swapHandler struct {
	h atomic.Value
}

func (s *swapHandler) set(h http.Handler) { s.h.Store(&h) }

func (s *swapHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h, ok := s.h.Load().(*http.Handler); ok {
		(*h).ServeHTTP(w, r)
		return
	}
	http.Error(w, "starting", http.StatusServiceUnavailable)
}

// startFailure is the window's page when the gateway cannot start.
func startFailure(err error, logPath string) http.Handler {
	msg := err.Error()
	if errors.Is(err, daemon.ErrWacliMissing) {
		msg = "O wacli, que conecta ao WhatsApp, não foi encontrado dentro do app. Reinstale o WhatsApp MCP."
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><html lang="pt-BR"><meta charset="utf-8"><title>WhatsApp MCP</title>
<body style="font:16px/1.6 -apple-system,system-ui,sans-serif;max-width:560px;margin:80px auto;padding:0 24px;color:#17322c">
<h1 style="font-size:1.4rem">O WhatsApp MCP não conseguiu iniciar</h1><p>%s</p>
<p style="color:#5b6f6a">Detalhes no log: <code>%s</code></p></body></html>`, htmlEscape(msg), htmlEscape(logPath))
	})
}
