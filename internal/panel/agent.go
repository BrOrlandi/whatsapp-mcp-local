package panel

import (
	"crypto/subtle"
	"encoding/json"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/mcp"
)

// The API an AI agent on this computer uses to install, pair and check the
// app: GET /api/status says where things stand and what comes next, and the
// panel's own actions (pairing, connecting a client, opening the window)
// accept a program's request as well as the page's. docs/api.md describes it.

// fromAgent reports whether a request comes from a program on this computer
// rather than from a browser. Browsers mark every request with Sec-Fetch-Site
// or Origin, and a page cannot send JSON to another site without both; a
// program sends neither. When the daemon has a token, a program presents it,
// as on /mcp.
func (p *Panel) fromAgent(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") != "" || r.Header.Get("Origin") != "" {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); ct != "application/json" {
			return false
		}
	}
	if p.Token == "" {
		return true
	}
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return ok && subtle.ConstantTimeCompare([]byte(strings.TrimSpace(got)), []byte(p.Token)) == 1
}

// agentStatus is GET /api/status. Its fields are kept stable for the prompts
// and agents that read them.
type agentStatus struct {
	App     string `json:"app"`
	Version string `json:"version"`
	// Desktop is true in the app, false in the command line version.
	Desktop  bool          `json:"desktop"`
	MCPURL   string        `json:"mcp_url"`
	WhatsApp agentWhatsApp `json:"whatsapp"`
	Pairing  agentPairing  `json:"pairing"`
	Health   agentHealth   `json:"health"`
	Next     string        `json:"next"`
}

type agentWhatsApp struct {
	// State is sync's: starting, connected, paused, reconnecting,
	// not_paired, logged_out or stopped.
	State  string `json:"state"`
	Paired bool   `json:"paired"`
	// Connected is receiving and ready to send.
	Connected     bool       `json:"connected"`
	Phone         string     `json:"phone,omitempty"`
	Name          string     `json:"name,omitempty"`
	LastMessageAt *time.Time `json:"last_message_at,omitempty"`
}

type agentPairing struct {
	// State is idle, starting, qr, code, syncing, done, error or cancelled.
	State string `json:"state"`
	// QRPNG is the QR code to read with the phone, while State is qr.
	QRPNG string `json:"qr_png,omitempty"`
	// Code is what to type on the phone, while State is code.
	Code  string `json:"code,omitempty"`
	Error string `json:"error,omitempty"`
}

type agentHealth struct {
	Status  string      `json:"status"`
	Summary string      `json:"summary"`
	Checks  []mcp.Check `json:"checks"`
}

// apiStatus is where the app stands and what an agent does next.
func (p *Panel) apiStatus(r *http.Request) (any, error) {
	s := p.snapshot(r.Context())
	st := agentStatus{
		App: "WhatsApp MCP Local", Version: strings.TrimPrefix(mcp.Version, "v"),
		Desktop: p.Host != nil, MCPURL: p.endpoint(),
		WhatsApp: agentWhatsApp{
			State: s.Sync.State, Paired: s.Account.Authenticated,
			Connected: s.Sync.State == "connected" && s.Sync.Delegate,
			Phone:     s.Phone, Name: s.Name,
		},
		Pairing: agentPairing{State: s.Pairing.State, Code: s.Pairing.Code, Error: s.Pairing.Error},
		Health:  agentHealth{Status: s.Health.Status, Summary: s.Health.Summary, Checks: s.Health.Checks},
		Next:    nextStep(s),
	}
	if t := s.Activity.NewestIncoming; t != nil && !t.IsZero() {
		st.WhatsApp.LastMessageAt = t
	}
	if s.Pairing.State == "qr" {
		st.Pairing.QRPNG = strings.TrimSuffix(p.endpoint(), "/mcp") + "/api/pair/qr.png"
	}
	return st, nil
}

// nextStep is what an agent does next:
//
//	ready    WhatsApp is connected, receiving and ready to send
//	scan     the person reads the QR code, or types the code, on the phone
//	pair     no WhatsApp and no pairing: open the app, or POST /api/pair
//	syncing  paired; the phone is sending the history, which takes minutes
//	wait     starting, reconnecting or paused for seconds: ask again shortly
//	error    sync stopped: health says why
func nextStep(s snapshot) string {
	switch {
	case s.Account.Authenticated && s.Sync.State == "connected" && s.Sync.Delegate:
		return "ready"
	case s.Pairing.State == "qr" || s.Pairing.State == "code":
		return "scan"
	case s.Pairing.State == "starting":
		return "wait"
	case s.Pairing.State == "syncing" || (s.Account.Authenticated && s.Arriving):
		return "syncing"
	case !s.Account.Authenticated || s.Sync.State == "not_paired" || s.Sync.State == "logged_out":
		return "pair"
	case settling(s.Sync) || s.Sync.State == "reconnecting":
		return "wait"
	}
	return "error"
}

// apiOpenApp brings the app's window up, on the page given (the first one by
// default).
func (p *Panel) apiOpenApp(r *http.Request) (any, error) {
	var body struct {
		Path string `json:"path"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Path != "" && (!strings.HasPrefix(body.Path, "/") || strings.HasPrefix(body.Path, "//")) {
		return nil, userError{"path must be a page of the app, such as /status"}
	}
	p.Host.ShowWindow(body.Path)
	return map[string]bool{"ok": true}, nil
}
