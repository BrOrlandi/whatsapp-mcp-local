package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/appconfig"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/daemon"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/mcp"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/panel"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/platform"
)

// bundleID names the app to the system: single instance, login item,
// notifications.
const bundleID = "com.brorlandi.whatsapp-mcp"

type App struct {
	logger   *slog.Logger
	logFile  *os.File
	logPath  string
	logDir   string
	dataDir  string
	storeDir string
	wacliBin string
	bridge   string
	hidden   bool

	cfgMu      sync.Mutex
	cfg        appconfig.Config
	portLocked bool

	wails  *application.App
	winMu  sync.Mutex
	window *application.WebviewWindow
	// showMu keeps the window's coming and going in order; dockedAt is when
	// the app last came into the Dock.
	showMu   sync.Mutex
	dockedAt time.Time
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
	logger, logFile, err := openLog(logDir)
	if err != nil {
		return nil, err
	}
	mcp.Version = version
	a := &App{logger: logger, logFile: logFile, logPath: logFile.Name(), logDir: logDir, dataDir: dataDir, hidden: hidden}
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
			UniqueID:               instanceID(),
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { go a.showWindow("") },
		},
		ShouldQuit: a.shouldQuit,
		OnShutdown: a.shutdown,
	})

	// The tray first: the gateway, started next, reports to it at once.
	a.setMenu()
	a.tray = newTray(a)
	a.prepare()

	a.wails.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) { a.showWindow("") })
	if runtime.GOOS == "darwin" {
		// macOS says whether it opened the app at login only once the app has
		// started, so the window is made then, if at all.
		a.wails.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
			a.waitOutOfDock()
			if a.hidden || launchedAtLogin() {
				a.logger.Info("opened at login: staying in the tray")
				return
			}
			a.showWindow("")
		})
	} else {
		// Elsewhere --hidden says so from the start, and GTK wants its windows
		// made before the application runs.
		a.createWindow(a.hidden, "/")
	}
	go a.firstRun()
	go a.updates.loop()
	return a.wails.Run()
}

// prepare starts the gateway the window shows.
func (a *App) prepare() {
	a.startDaemon()
}

// instanceID is one per data folder: an app pointed at another folder with
// WHATSAPP_MCP_DATA, for development or a demo, runs beside the real one.
func instanceID() string {
	dir := os.Getenv("WHATSAPP_MCP_DATA")
	if dir == "" {
		return bundleID
	}
	sum := sha256.Sum256([]byte(dir))
	return bundleID + "." + hex.EncodeToString(sum[:4])
}

func (a *App) startDaemon() {
	desktop := panel.DesktopCommand{Command: a.bridge}
	if appImage := os.Getenv("APPIMAGE"); appImage != "" {
		desktop = panel.DesktopCommand{Command: appImage, Args: []string{"bridge"}}
	} else if desktop.Command == "" {
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
// the checkbox there and in the tray turns it off. On later runs it makes
// sure the registration still stands: a new version, signed anew or moved,
// can lose it.
func (a *App) firstRun() {
	first := !a.config().HasSeen("first-run")
	if first || (a.config().Autostart && !a.autostartRegistered()) {
		if err := a.SetAutostart(a.config().Autostart); err != nil {
			a.logger.Warn("could not register the app to open at login", "error", err)
		}
	}
	if first {
		_ = a.updateConfig(func(c *appconfig.Config) { c.Seen = append(c.Seen, "first-run") })
	}
}

func (a *App) autostartRegistered() bool {
	if os.Getenv("APPIMAGE") != "" {
		return false // rewriting the entry is cheap and keeps its path current
	}
	on, err := a.wails.Autostart.IsEnabled()
	return err == nil && on
}

func (a *App) onClose(w *application.WebviewWindow, e *application.WindowEvent) {
	// Quitting, or the window hideWindow let go: it closes.
	if a.quitting.Load() || a.win() != w {
		return
	}
	e.Cancel()
	go a.closeWindow()
}

// closeWindow is what closing the window does, and on macOS ⌘Q too: the app
// stays in the tray, working, unless the person turned that off.
func (a *App) closeWindow() {
	switch {
	case !a.canHide:
		if w := a.win(); w != nil {
			w.Minimise()
		}
	case a.config().CloseToTray:
		a.hideWindow()
		if !a.config().HasSeen("close") {
			_ = a.updateConfig(func(c *appconfig.Config) { c.Seen = append(c.Seen, "close") })
			a.notify("close", "O WhatsApp MCP continua rodando",
				fmt.Sprintf("Ele fica na %s, e as ferramentas de IA continuam usando o WhatsApp. Para desligar de vez, use Encerrar no ícone dele.", trayPlace()))
		}
	default:
		a.quit()
	}
}

// shouldQuit answers a request to quit the app. Encerrar quits, and so do
// logging out, shutting down and a signal; Encerrar in the macOS Dock closes
// the window, as ⌘Q does, since the app keeps working without one.
func (a *App) shouldQuit() bool {
	if a.quitting.Load() || !quitAskedByApp() {
		return true
	}
	go a.closeWindow()
	return false
}

// quitWarning is what quitting costs, said before the person confirms.
const quitWarning = "O MCP é desligado: as ferramentas de IA perdem o acesso ao WhatsApp, e este computador para de receber mensagens enquanto o app estiver fechado."

// confirmQuit asks before quitting, from the tray and the app menu.
func (a *App) confirmQuit() {
	application.InvokeSync(activate)
	d := a.wails.Dialog.Question().SetTitle("Encerrar o " + platform.AppName + "?").SetMessage(quitWarning)
	// Windows only has its own Sim and Não, which Wails names in English.
	no, yes := "Cancelar", "Encerrar"
	if runtime.GOOS == "windows" {
		no, yes = "No", "Yes"
	}
	d.AddButton(no).SetAsCancel()
	d.SetDefaultButton(d.AddButton(yes).OnClick(func() { go a.quit() }))
	d.Show()
}

func (a *App) createWindow(hidden bool, url string) *application.WebviewWindow {
	w := a.wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            platform.AppName,
		Width:            1000,
		Height:           760,
		MinWidth:         720,
		MinHeight:        560,
		URL:              url,
		Hidden:           hidden,
		BackgroundColour: application.NewRGB(246, 248, 247),
	})
	w.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) { a.onClose(w, e) })
	a.winMu.Lock()
	a.window = w
	a.winMu.Unlock()
	return w
}

func (a *App) win() *application.WebviewWindow {
	a.winMu.Lock()
	defer a.winMu.Unlock()
	return a.window
}

// dockDelay is how long the app gives macOS to finish bringing it into the
// Dock before it makes the window.
const dockDelay = 200 * time.Millisecond

// showWindow brings the window up, at path when one is given. It can wait,
// so it must not run on the main thread.
func (a *App) showWindow(path string) {
	a.showMu.Lock()
	defer a.showMu.Unlock()
	w := a.win()
	if w == nil {
		if runtime.GOOS != "darwin" {
			return
		}
		// macOS treats a window made while the app is out of the Dock as a
		// menu bar app's for as long as it lives, and shows it over other
		// apps in full screen. So the window is made fresh each time, once
		// the app is in the Dock, and goes when it closes.
		var docked bool
		application.InvokeSync(func() {
			docked = inDock()
			showInDock(true)
		})
		if !docked {
			a.dockedAt = time.Now()
			time.Sleep(dockDelay)
		}
		if path == "" {
			path = "/"
		}
		w, path = a.createWindow(false, path), ""
	}
	application.InvokeSync(func() {
		showInDock(true)
		activate()
		if path != "" {
			w.SetURL(path)
		}
		w.Show()
		w.UnMinimise()
		w.Focus()
	})
}

// hideWindow leaves the app in the tray alone. It can wait, so it must not
// run on the main thread.
func (a *App) hideWindow() {
	a.showMu.Lock()
	defer a.showMu.Unlock()
	w := a.win()
	if w == nil {
		return
	}
	if runtime.GOOS != "darwin" {
		application.InvokeSync(func() { w.Hide() })
		return
	}
	a.winMu.Lock()
	a.window = nil
	a.winMu.Unlock()
	application.InvokeSync(func() { w.Hide() })
	w.Close()
	// Leaving the Dock right after coming in can leave a second icon there,
	// so a window closed at once waits out a second.
	if wait := time.Until(a.dockedAt.Add(time.Second)); wait > 0 {
		time.Sleep(wait)
	}
	application.InvokeSync(func() { showInDock(false) })
}

// waitOutOfDock waits for Wails to take the app out of the Dock as it
// starts, which it does from another goroutine: a window brought in before
// that would be left without the Dock.
func (a *App) waitOutOfDock() {
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		var out bool
		application.InvokeSync(func() { out = menuBarOnly() })
		if out {
			return
		}
	}
	a.logger.Warn("the app did not leave the Dock as it started")
}

func (a *App) quit() {
	a.quitting.Store(true)
	a.wails.Quit()
}

// shutdown stops the gateway with care, whatever ends the app: Encerrar,
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

// swapHandler is what the window loads: the panel once the gateway is up,
// or a page saying why it could not start.
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
