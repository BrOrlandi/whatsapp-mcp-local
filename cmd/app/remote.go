package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/appconfig"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/daemon"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/httpserver"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/panel"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/remote"
)

// Remote mode: the app does not run WhatsApp here. It keeps a session with
// the daemon on a server (internal/remote) and serves, on this computer's
// own loopback port, what the server serves: the window shows the server's
// panel, and the AI tools and the command line reach the server's MCP and
// API at the same address as in local mode. WhatsApp's session lives on the
// server only; this computer never starts wacli in this mode.

type remoteMode struct {
	client *remote.Client
	cancel context.CancelFunc
	srv    *http.Server
	server string

	mu       sync.Mutex
	whatsapp *serverWhatsApp
	checked  time.Time
}

// serverWhatsApp is the server's connection with WhatsApp, as its
// /api/status said last.
type serverWhatsApp struct {
	State     string `json:"state"`
	Next      string `json:"next"`
	Connected bool   `json:"connected"`
	Paired    bool   `json:"paired"`
	Name      string `json:"name,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Version   string `json:"version,omitempty"`
}

func (a *App) remoteConfigured() bool {
	r := a.config().Remote
	return r != nil && r.Server != "" && r.DeviceID != ""
}

func (a *App) currentRemote() *remoteMode {
	a.dmu.Lock()
	defer a.dmu.Unlock()
	return a.remote
}

// startRemote connects to the server and serves it on the loopback port.
func (a *App) startRemote() {
	cfg := a.config().Remote
	device := remote.Device{Server: cfg.Server, ServerKey: cfg.ServerKey, ID: cfg.DeviceID, Scope: cfg.Scope}
	key, err := loadDeviceKey(cfg.DeviceID)
	var client *remote.Client
	if err == nil {
		client, err = remote.NewClient(device, key)
	}
	if err != nil {
		a.logger.Error("remote mode cannot start: the device's key is missing", "error", err)
		a.handler.set(panel.Internal(a.withAppPages(remoteKeyMissing())))
		return
	}
	// A new key is kept only once the server has taken it.
	client.OnKey = func(private []byte) {
		if err := saveDeviceKey(cfg.DeviceID, private); err != nil {
			a.logger.Error("the device's new key could not be kept in the keychain", "error", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	rm := &remoteMode{client: client, cancel: cancel, server: cfg.Server}
	go client.Run(ctx)
	// The window's requests are marked as the window's; what reaches the
	// loopback port, from any program here, never is.
	window := a.remoteProxy(rm, true)
	proxy := a.remoteProxy(rm, false)

	// What belongs to this computer, not to the server: its AI tools' setup
	// and the app's window.
	local := (&panel.Panel{Endpoint: a.localMCPURL, Desktop: a.desktopCommand(), Host: a}).LocalHandler()
	withLocal := func(next http.Handler) http.Handler {
		mux := http.NewServeMux()
		mux.Handle("POST /api/clients/{client}", local)
		mux.Handle("POST /api/app/open", local)
		mux.Handle("/", next)
		return mux
	}

	// The AI tools and the command line of this computer: the server's MCP
	// and API, at the same address as in local mode.
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(a.config().Port)))
	if err != nil {
		a.logger.Warn("the MCP port is taken; the AI tools of this computer cannot reach the server", "port", a.config().Port, "error", err)
	} else {
		rm.srv = httpserver.NewServer(httpserver.Guard(withLocal(proxy), a.logger))
		go func() { _ = rm.srv.Serve(ln) }()
	}

	a.dmu.Lock()
	a.remote = rm
	a.dmu.Unlock()
	// The window: the server's panel, with the app's own pages and the
	// setup of this computer's AI tools.
	a.handler.set(panel.Internal(a.withAppPages(withLocal(window))))
	a.logger.Info("remote mode", "server", cfg.Server, "device", cfg.DeviceID)
	go a.followRemote(ctx, rm)
}

func (a *App) stopRemote() {
	a.dmu.Lock()
	rm := a.remote
	a.remote = nil
	a.dmu.Unlock()
	if rm == nil {
		return
	}
	rm.cancel()
	if rm.srv != nil {
		httpserver.Shutdown(rm.srv)
	}
}

func (a *App) localMCPURL() string {
	return "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(a.config().Port)) + "/mcp"
}

// remoteProxy sends a request to the server over the session. With the
// session down, a page says so instead of the server's, and anything else
// gets a 503.
func (a *App) remoteProxy(rm *remoteMode, window bool) http.Handler {
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL.Scheme, r.Out.URL.Host = "http", "remote"
			r.Out.Header.Set(panel.ServerHeader, rm.server)
			r.Out.Header.Del(remote.WindowHeader)
			if window {
				r.Out.Header.Set(remote.WindowHeader, "1")
				// The window's own requests carry no credential of a program.
				r.Out.Header.Del("Authorization")
			}
		},
		Transport:     rm.client,
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html") || r.URL.Path == "/" {
				a.renderUnavailable(w, rm)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": i18n.T("o app está sem conexão com o servidor agora; tente de novo em instantes")})
		},
	}
}

// followRemote asks the server how its WhatsApp is, every few seconds,
// for the tray and the app's pages.
func (a *App) followRemote(ctx context.Context, rm *remoteMode) {
	last := ""
	for {
		st := a.pollServer(ctx, rm)
		if a.tray != nil {
			a.tray.render(st)
		}
		if st.Sync == "logged_out" && last != "" && last != "logged_out" {
			a.notify("whatsapp-logged-out", i18n.T("WhatsApp desconectado"),
				i18n.T("O celular desconectou o servidor. Abra o WhatsApp MCP para conectar de novo."))
		}
		last = st.Sync
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func (a *App) pollServer(ctx context.Context, rm *remoteMode) daemon.Status {
	link := rm.client.Status()
	if link.State != remote.StateConnected {
		return daemon.Status{Tone: "fail", Title: linkLabel(link.State), Detail: hostOf(rm.server), Sync: "remote_" + link.State}
	}
	rctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(rctx, http.MethodGet, "http://remote/api/status", nil)
	resp, err := rm.client.RoundTrip(req)
	if err != nil {
		return daemon.Status{Tone: "warn", Title: i18n.T("Servidor sem resposta"), Detail: hostOf(rm.server)}
	}
	defer resp.Body.Close()
	var body struct {
		Version  string         `json:"version"`
		WhatsApp serverWhatsApp `json:"whatsapp"`
		Next     string         `json:"next"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return daemon.Status{Tone: "warn", Title: i18n.T("Servidor sem resposta"), Detail: hostOf(rm.server)}
	}
	wa := body.WhatsApp
	wa.Next, wa.Version = body.Next, body.Version
	rm.mu.Lock()
	rm.whatsapp, rm.checked = &wa, time.Now()
	rm.mu.Unlock()
	st := daemon.Status{Sync: wa.State, Detail: i18n.Tf("remoto: %s", hostOf(rm.server))}
	switch wa.Next {
	case "ready":
		st.Tone, st.Title = "ok", i18n.T("Conectado")
	case "syncing", "wait":
		st.Tone, st.Title = "warn", i18n.T("Sincronizando")
	case "pair", "scan":
		st.Tone, st.Title = "fail", i18n.T("WhatsApp desconectado no servidor")
	default:
		st.Tone, st.Title = "fail", i18n.T("A sincronização parou no servidor")
	}
	return st
}

func linkLabel(state string) string {
	switch state {
	case remote.StateConnected:
		return i18n.T("Conectado")
	case remote.StateConnecting:
		return i18n.T("Conectando")
	case remote.StateReconnecting:
		return i18n.T("Reconectando")
	case remote.StateRefused:
		return i18n.T("Recusado pelo servidor")
	}
	return i18n.T("Indisponível")
}

func hostOf(server string) string {
	if u, err := url.Parse(server); err == nil && u.Host != "" {
		return u.Host
	}
	return server
}

// ---- the app's own pages ----

// withAppPages adds the app's own pages, under /__app/, to what the window
// shows: they answer the window only, never the loopback port.
func (a *App) withAppPages(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /__app/remoto", a.remotePage)
	mux.HandleFunc("GET /__app/remote/status", a.remoteStatusAPI)
	mux.HandleFunc("POST /__app/remoto/parear", a.remotePair)
	mux.HandleFunc("POST /__app/remoto/local", a.remoteToLocal)
	mux.HandleFunc("POST /__app/remoto/tentar", func(w http.ResponseWriter, r *http.Request) {
		if rm := a.currentRemote(); rm != nil {
			rm.client.Retry()
		}
		http.Redirect(w, r, "/__app/remoto", http.StatusSeeOther)
	})
	mux.HandleFunc("POST /__app/remoto/girar", a.remoteRotate)
	mux.Handle("/", next)
	return mux
}

type remoteView struct {
	Lang, CSS template.CSS
	OK, Error string
	Remote    bool
	Server    string
	Device    string
	Finger    string
	Link      remote.Status
	LinkLabel string
	LinkTone  string
	WhatsApp  *serverWhatsApp
	WALabel   string
	WATone    string
	Checked   time.Time
	Hostname  string
}

func (a *App) remoteView(r *http.Request) remoteView {
	v := remoteView{Lang: template.CSS(i18n.Lang()), CSS: template.CSS(panel.Stylesheet()), OK: r.URL.Query().Get("ok"), Error: r.URL.Query().Get("erro")}
	v.Hostname, _ = os.Hostname()
	v.Hostname = strings.TrimSuffix(v.Hostname, ".local")
	cfg := a.config().Remote
	rm := a.currentRemote()
	if cfg == nil || rm == nil {
		return v
	}
	v.Remote, v.Server, v.Device = true, hostOf(cfg.Server), cfg.DeviceID
	v.Finger = remote.Fingerprint(cfg.ServerKey)
	v.Link = rm.client.Status()
	v.LinkLabel = linkLabel(v.Link.State)
	v.LinkTone = map[string]string{remote.StateConnected: "ok", remote.StateConnecting: "warn", remote.StateReconnecting: "warn"}[v.Link.State]
	if v.LinkTone == "" {
		v.LinkTone = "fail"
	}
	rm.mu.Lock()
	wa, checked := rm.whatsapp, rm.checked
	rm.mu.Unlock()
	if wa != nil && v.Link.State == remote.StateConnected {
		v.WhatsApp, v.Checked = wa, checked
		switch wa.Next {
		case "ready":
			v.WALabel, v.WATone = i18n.T("Conectado"), "ok"
		case "syncing", "wait":
			v.WALabel, v.WATone = i18n.T("Sincronizando"), "warn"
		case "pair", "scan":
			v.WALabel, v.WATone = i18n.T("Desconectado: conecte pelo painel do servidor"), "fail"
		default:
			v.WALabel, v.WATone = i18n.T("A sincronização parou"), "fail"
		}
	} else {
		v.WALabel, v.WATone = i18n.T("Desconhecido: sem conexão com o servidor"), "off"
	}
	return v
}

func (a *App) remotePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = remoteTemplate.Execute(w, a.remoteView(r))
}

func (a *App) remoteStatusAPI(w http.ResponseWriter, r *http.Request) {
	v := a.remoteView(r)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"remote": v.Remote, "server": v.Server, "link": v.Link, "whatsapp": v.WhatsApp})
}

func (a *App) renderUnavailable(w http.ResponseWriter, rm *remoteMode) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusServiceUnavailable)
	req, _ := http.NewRequest(http.MethodGet, "/__app/remoto", nil)
	v := a.remoteView(req)
	v.Error = i18n.T("O app está sem conexão com o servidor agora. Ele tenta de novo sozinho; esta página se atualiza.")
	_ = remoteTemplate.Execute(w, v)
}

func back(w http.ResponseWriter, r *http.Request, okMsg string, err error) {
	q := url.Values{}
	if err != nil {
		q.Set("erro", err.Error())
	} else if okMsg != "" {
		q.Set("ok", okMsg)
	}
	http.Redirect(w, r, "/__app/remoto?"+q.Encode(), http.StatusSeeOther)
}

// remotePair pairs this app with a server and switches to remote mode. The
// code travels in the form's body only, and is never kept or logged.
func (a *App) remotePair(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	server, code, name := r.FormValue("server"), r.FormValue("code"), strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name, _ = os.Hostname()
	}
	if _, err := remote.ServerURL(server); err != nil {
		back(w, r, "", errors.New(i18n.T("o endereço do servidor precisa começar com https:// (a conexão remota é só por HTTPS)")))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	device, key, err := remote.Pair(ctx, server, code, name)
	if err != nil {
		a.logger.Warn("remote pairing failed", "server", hostOf(server), "error", err)
		back(w, r, "", errors.New(pairingError(err)))
		return
	}
	if err := saveDeviceKey(device.ID, key); err != nil {
		back(w, r, "", fmt.Errorf("%s: %v", i18n.T("não foi possível guardar a chave no Keychain"), err))
		return
	}
	old := a.config().Remote
	if err := a.updateConfig(func(c *appconfig.Config) {
		c.Remote = &appconfig.Remote{Server: device.Server, ServerKey: device.ServerKey, DeviceID: device.ID, Scope: device.Scope}
	}); err != nil {
		back(w, r, "", err)
		return
	}
	if old != nil && old.DeviceID != device.ID {
		_ = deleteDeviceKey(old.DeviceID)
	}
	a.logger.Info("paired with a server; switching to remote mode", "server", hostOf(device.Server), "device", device.ID)
	setAside := r.FormValue("set_aside") == "1"
	a.switchToRemote(setAside)
	back(w, r, i18n.Tf("Conectado ao servidor %s. O WhatsApp deste app agora é o do servidor.", hostOf(device.Server)), nil)
}

// switchToRemote stops the local gateway, so this computer never holds the
// WhatsApp session too, and starts remote mode.
func (a *App) switchToRemote(setAside bool) {
	a.stopRemote()
	a.dmu.Lock()
	d, cancel := a.daemon, a.follow
	a.daemon, a.follow = nil, nil
	a.dmu.Unlock()
	if cancel != nil {
		cancel()
	}
	if d != nil {
		a.logger.Info("stopping the local gateway for remote mode")
		d.Stop()
	}
	if setAside {
		// The WhatsApp session of this computer went to the server: keep it
		// aside, under another name, so this computer never opens it again.
		aside := a.storeDir + ".no-servidor-" + time.Now().Format("20060102-150405")
		if err := os.Rename(a.storeDir, aside); err != nil && !os.IsNotExist(err) {
			a.logger.Error("could not set the local WhatsApp session aside", "error", err)
		} else {
			a.logger.Info("the local WhatsApp session was set aside", "to", aside)
		}
	}
	a.startRemote()
}

// localPhone is the number whose WhatsApp session this computer's store
// holds, "" when none.
func (a *App) localPhone() string {
	db, err := sql.Open("sqlite", "file:"+(&url.URL{Path: filepath.Join(a.storeDir, "session.db")}).EscapedPath()+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		return ""
	}
	defer db.Close()
	var jid string
	if db.QueryRow(`SELECT jid FROM whatsmeow_device LIMIT 1`).Scan(&jid) != nil {
		return ""
	}
	user, _, _ := strings.Cut(jid, "@")
	user, _, _ = strings.Cut(user, ":")
	return digitsOf(user)
}

func digitsOf(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

func (a *App) remoteToLocal(w http.ResponseWriter, r *http.Request) {
	cfg := a.config().Remote
	// The same WhatsApp session must never run here and on the server at
	// once: with this computer holding the number the server has, or with
	// the server out of reach to say, going back needs the person's word
	// that the server was disconnected first.
	if local := a.localPhone(); local != "" && r.FormValue("server_disconnected") != "1" {
		server := ""
		if rm := a.currentRemote(); rm != nil {
			rm.mu.Lock()
			if rm.whatsapp != nil && rm.whatsapp.Paired && rm.client.Status().State == remote.StateConnected {
				server = digitsOf(rm.whatsapp.Phone)
			} else if rm.whatsapp != nil && !rm.whatsapp.Paired && rm.client.Status().State == remote.StateConnected {
				server = "none"
			}
			rm.mu.Unlock()
		}
		if server != "none" && (server == "" || server == local) {
			back(w, r, "", errors.New(i18n.T("este computador guarda a mesma sessão do WhatsApp que pode estar conectada no servidor: desconecte o WhatsApp no servidor antes e marque a confirmação, ou as duas cópias brigam pela sessão")))
			return
		}
	}
	a.stopRemote()
	_ = a.updateConfig(func(c *appconfig.Config) { c.Remote = nil })
	if cfg != nil && r.FormValue("forget") == "1" {
		_ = deleteDeviceKey(cfg.DeviceID)
	}
	a.logger.Info("back to local mode")
	a.startDaemon()
	http.Redirect(w, r, "/configuracoes#remoto", http.StatusSeeOther)
}

func (a *App) remoteRotate(w http.ResponseWriter, r *http.Request) {
	rm, cfg := a.currentRemote(), a.config().Remote
	if rm == nil || cfg == nil {
		back(w, r, "", errors.New(i18n.T("o app não está no modo remoto")))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	// The new key is kept (OnKey) once the next session proves it.
	if _, err := rm.client.Rotate(ctx); err != nil {
		back(w, r, "", err)
		return
	}
	back(w, r, i18n.T("A chave deste app foi trocada; a anterior deixa de valer assim que a nova conecta."), nil)
}

func remoteKeyMissing() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/__app/remoto?erro="+url.QueryEscape(i18n.T("a chave deste app não está no Keychain: pareie de novo com um código do servidor")), http.StatusSeeOther)
	})
}

var remoteTemplate = template.Must(template.New("remote").Funcs(template.FuncMap{"t": i18n.T, "tf": i18n.Tf,
	"moment": func(t time.Time) string { return t.Local().Format("15:04:05") }}).Parse(remotePageSource))

// pairingError says what went wrong with a pairing, in the person's
// language.
func pairingError(err error) string {
	switch {
	case errors.Is(err, remote.ErrNotACode), errors.Is(err, remote.ErrCodeIncomplete):
		return i18n.T("o código de pareamento está incompleto: copie de novo, inteiro (ele começa com wamcp1-)")
	case errors.Is(err, remote.ErrCodeRefused):
		return i18n.T("o servidor recusou o código: ele vale uma vez, por 10 minutos; gere outro no servidor")
	case errors.Is(err, remote.ErrServerKeyMismatch):
		return i18n.T("a chave do servidor não confere com o código: este não é o servidor que gerou o código")
	case errors.Is(err, remote.ErrNotHTTPS):
		return i18n.T("o endereço do servidor precisa começar com https:// (a conexão remota é só por HTTPS)")
	case errors.Is(err, remote.ErrRedirect):
		return i18n.T("o servidor respondeu com um redirecionamento, que a conexão remota não segue: confira o endereço")
	case errors.Is(err, remote.ErrProxyNotHTTPS):
		return i18n.T("o proxy do servidor não informou que a conexão chegou por HTTPS: confira a configuração dele")
	case strings.Contains(err.Error(), "certificate"):
		return i18n.T("o certificado HTTPS do servidor não é válido para este endereço; a conexão foi recusada")
	}
	return i18n.Tf("não foi possível falar com o servidor: %v", err)
}
