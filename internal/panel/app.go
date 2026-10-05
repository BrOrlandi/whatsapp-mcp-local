package panel

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Host is the desktop app around the panel: the settings and actions that
// only exist inside it. The command line has none, and leaves it nil.
type Host interface {
	Settings() HostSettings
	SetAutostart(on bool) error
	SetCloseToTray(on bool) error
	// SetPort moves the MCP to another port and keeps it there.
	SetPort(port int) error
	OpenURL(url string) error
	OpenDataFolder() error
	Update() UpdateState
	CheckUpdate()
	InstallUpdate() error
	// Quit closes the app for good, as the page asks once the person confirmed.
	Quit()
	// EraseEverything unlinks WhatsApp, deletes the data folder and quits.
	EraseEverything() error
}

// HostSettings is what the Configurações page shows.
type HostSettings struct {
	Autostart   bool
	CloseToTray bool
	// CanHide is false where there is no tray to hide into (GNOME without
	// the AppIndicator extension): closing the window then minimises it.
	CanHide bool
	// PortLocked is set when WHATSAPP_MCP_PORT fixes the port.
	PortLocked bool
	DataDir    string
	Version    string
}

// UpdateState is where the app's update stands.
type UpdateState struct {
	// State is idle, checking, current, available, downloading, ready,
	// manual (a package to install by hand) or error.
	State     string    `json:"state"`
	Current   string    `json:"current"`
	Latest    string    `json:"latest,omitempty"`
	Page      string    `json:"page,omitempty"`
	Progress  int       `json:"progress,omitempty"`
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checked_at,omitempty"`
}

func (p *Panel) registerApp(mux *http.ServeMux) {
	mux.HandleFunc("GET /configuracoes", p.page(p.configuracoes))
	mux.HandleFunc("POST /configuracoes/porta", p.form(p.savePort))
	mux.HandleFunc("POST /configuracoes/encerrar", p.quit)
	mux.HandleFunc("POST /configuracoes/apagar", p.erase)
	mux.HandleFunc("POST /api/configuracoes", p.api(p.apiSettings))
	mux.HandleFunc("POST /api/abrir", p.api(p.apiOpen))
	mux.HandleFunc("POST /api/abrir-pasta", p.api(func(*http.Request) (any, error) { return map[string]bool{"ok": true}, p.Host.OpenDataFolder() }))
	mux.HandleFunc("GET /api/atualizacao", p.api(func(*http.Request) (any, error) { return p.Host.Update(), nil }))
	mux.HandleFunc("POST /api/atualizacao/verificar", p.api(func(*http.Request) (any, error) { p.Host.CheckUpdate(); return p.Host.Update(), nil }))
	mux.HandleFunc("POST /api/atualizacao/instalar", p.api(func(*http.Request) (any, error) {
		if err := p.Host.InstallUpdate(); err != nil {
			return nil, err
		}
		return p.Host.Update(), nil
	}))
}

func (p *Panel) configuracoes(w http.ResponseWriter, r *http.Request) (string, any) {
	s := p.snapshot(r.Context())
	set := p.setup(r.Context())
	return "configuracoes", struct {
		layout
		Setup       setup
		Port        int
		PortChanged bool
	}{p.layout(r, "Configurações", "configuracoes", s), set, portOf(p.endpoint()), r.URL.Query().Get("porta") == "1"}
}

func portOf(endpoint string) int {
	u, err := url.Parse(endpoint)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(u.Port())
	return n
}

func (p *Panel) savePort(r *http.Request) (string, error) {
	if p.Host.Settings().PortLocked {
		return "/configuracoes", userError{"a porta está definida pela variável de ambiente WHATSAPP_MCP_PORT"}
	}
	port, err := strconv.Atoi(strings.TrimSpace(r.FormValue("port")))
	if err != nil || port < 1024 || port > 65535 {
		return "/configuracoes", userError{"a porta precisa ser um número de 1024 a 65535"}
	}
	back := "/configuracoes"
	if r.FormValue("from") != "" && strings.HasPrefix(r.FormValue("from"), "/") {
		back = r.FormValue("from")
	}
	if portOf(p.endpoint()) == port && p.MCPProblem() == nil {
		return back, nil
	}
	if err := p.Host.SetPort(port); err != nil {
		return back, userError{err.Error()}
	}
	return "/configuracoes?porta=1&ok=" + url.QueryEscape("O MCP agora responde em "+p.endpoint()+"."), nil
}

func (p *Panel) apiSettings(r *http.Request) (any, error) {
	var body struct {
		Autostart   *bool `json:"autostart"`
		CloseToTray *bool `json:"close_to_tray"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, userError{"pedido inválido"}
	}
	if body.Autostart != nil {
		if err := p.Host.SetAutostart(*body.Autostart); err != nil {
			return nil, err
		}
	}
	if body.CloseToTray != nil {
		if err := p.Host.SetCloseToTray(*body.CloseToTray); err != nil {
			return nil, err
		}
	}
	return p.Host.Settings(), nil
}

// apiOpen opens a link in the system's browser: the app's window shows only
// the panel.
func (p *Panel) apiOpen(r *http.Request) (any, error) {
	var body struct {
		URL string `json:"url"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	u, err := url.Parse(body.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, userError{"endereço inválido"}
	}
	return map[string]bool{"ok": true}, p.Host.OpenURL(u.String())
}

func (p *Panel) quit(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	p.Host.Quit()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = p.pages.ExecuteTemplate(w, "encerrado", layout{Title: "Encerrando", App: true})
}

func (p *Panel) erase(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.FormValue("confirm") != "apagar" {
		http.Redirect(w, r, "/configuracoes?erro="+url.QueryEscape("para apagar, digite apagar na caixa de confirmação"), http.StatusSeeOther)
		return
	}
	if err := p.Host.EraseEverything(); err != nil {
		http.Redirect(w, r, "/configuracoes?erro="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = p.pages.ExecuteTemplate(w, "apagado", layout{Title: "Dados apagados", App: true})
}
