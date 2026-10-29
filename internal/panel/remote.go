package panel

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/remote"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

// Remote access (internal/remote): the daemon pairs devices with one-time
// codes, lists them and revokes them; and every page served to a paired
// app says so, with the server's name and how the two connections stand.

// ServerHeader is how the app tells the server the address it reached it
// at, for the remote banner; the server shows only its host.
const ServerHeader = "X-WhatsApp-MCP-Server"

// remoteBanner is the strip at the top of every page the app shows from a
// server: which server, and the two connections, the app's with the server
// (which the app keeps current on the page) and the server's with WhatsApp.
type remoteBanner struct {
	Server        string
	Device        string
	WhatsAppTone  string
	WhatsAppLabel string
}

func (p *Panel) remoteBanner(r *http.Request, s snapshot) *remoteBanner {
	d, ok := remote.DeviceFrom(r.Context())
	if !ok {
		return nil
	}
	server := ""
	if u, err := url.Parse(r.Header.Get(ServerHeader)); err == nil && u.Scheme == "https" {
		server = u.Host
	}
	return &remoteBanner{Server: server, Device: d.Name, WhatsAppTone: s.SyncTone, WhatsAppLabel: s.SyncLabel}
}

// registerRemote adds the routes that manage remote access.
func (p *Panel) registerRemote(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/remote/codigo", p.api(p.apiRemoteCode))
	mux.HandleFunc("GET /api/remote/dispositivos", p.api(p.apiRemoteDevices))
	mux.HandleFunc("DELETE /api/remote/dispositivos/{id}", p.api(p.apiRemoteRevoke))
	mux.HandleFunc("GET /api/remote/token", p.remoteToken)
	p.registerAPIKeys(mux)
}

// apiRemoteToken gives the server's token to the window of a paired admin
// app, and to nothing else: the app writes it into the AI tools it connects
// on its own computer, whose requests reach the server as a program's and
// need it, as a program here would.
func (p *Panel) remoteToken(w http.ResponseWriter, r *http.Request) {
	d, ok := remote.DeviceFrom(r.Context())
	internal, _ := r.Context().Value(internalKey{}).(bool)
	if !ok || d.Scope != remote.ScopeAdmin || !internal {
		http.Error(w, "only the window of a paired app", http.StatusForbidden)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"token": p.Token})
}

func (p *Panel) apiRemoteCode(r *http.Request) (any, error) {
	if p.Remote == nil {
		return nil, userError{i18n.T("o acesso remoto não está disponível")}
	}
	var body struct {
		Name  string `json:"name"`
		Scope string `json:"scope"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Scope == "" {
		body.Scope = remote.ScopeAdmin
	}
	code, err := p.Remote.NewCode(body.Name, body.Scope)
	if err != nil {
		return nil, userError{err.Error()}
	}
	return code, nil
}

// remoteDevice is a paired device as the API shows it; this is the device
// the request came from, when it came from one.
type remoteDevice struct {
	state.RemoteClient
	This bool `json:"this_device,omitempty"`
}

func (p *Panel) apiRemoteDevices(r *http.Request) (any, error) {
	list, err := p.State.RemoteClients(r.Context())
	if err != nil {
		return nil, err
	}
	me, _ := remote.DeviceFrom(r.Context())
	out := []remoteDevice{}
	for _, c := range list {
		out = append(out, remoteDevice{RemoteClient: c, This: c.ID == me.ID})
	}
	fingerprint := ""
	if p.Remote != nil {
		fingerprint = remote.Fingerprint(p.Remote.PublicKey())
	}
	return map[string]any{"devices": out, "server_fingerprint": fingerprint}, nil
}

func (p *Panel) apiRemoteRevoke(r *http.Request) (any, error) {
	if p.Remote == nil {
		return nil, userError{i18n.T("o acesso remoto não está disponível")}
	}
	// Revoke also ends the device's open sessions.
	err := p.Remote.Revoke(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if errors.Is(err, state.ErrNoRemoteClient) {
		return nil, userError{i18n.T("nenhum dispositivo ativo tem esse id")}
	}
	if err != nil {
		return nil, err
	}
	return map[string]bool{"revoked": true}, nil
}

// devices are the paired devices, for the Configurações card.
func (p *Panel) devices(r *http.Request) []remoteDevice {
	list, err := p.State.RemoteClients(r.Context())
	if err != nil {
		return nil
	}
	me, _ := remote.DeviceFrom(r.Context())
	var out []remoteDevice
	for _, c := range list {
		out = append(out, remoteDevice{RemoteClient: c, This: c.ID == me.ID})
	}
	return out
}

// LocalHandler answers, in the app's remote mode, what belongs to this
// computer and not to the server: setting this computer's AI tools up (POST
// /api/clients/{client}; they reach the MCP through the app, at this
// computer's own address) and bringing the app's window up (POST
// /api/app/open, with a Host).
func (p *Panel) LocalHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/clients/{client}", p.api(p.apiAddClient))
	if p.Host != nil {
		mux.HandleFunc("POST /api/app/open", p.api(p.apiOpenApp))
	}
	return mux
}
