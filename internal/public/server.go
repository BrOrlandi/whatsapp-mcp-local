package public

import (
	"bytes"
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/httpserver"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

// CookieName is the panel session's cookie.
const CookieName = "whatsapp_mcp_session"

// The public listener's own routes: the sign-in page and signing out.
const (
	LoginPath  = "/entrar"
	LogoutPath = "/sair"
)

// OwnRoutes are the routes only the public listener has.
var OwnRoutes = []string{"GET " + LoginPath, "POST " + LoginPath, "POST " + LogoutPath}

// HTTPSRefused is the answer to a request that did not prove it came over
// HTTPS, when the public address is https.
const HTTPSRefused = "this request did not arrive over HTTPS: point the tunnel or proxy at this port, or set WHATSAPP_MCP_TRUSTED_PROXY to the proxy's address"

// Server is the public listener's handler: it says who each request is,
// before any route runs, and answers what an unsigned visitor may not see.
type Server struct {
	cfg      *Config
	routes   *http.ServeMux
	logger   *slog.Logger
	sessions *sessions
	throttle *throttle
	// bearers slows down wrong keys and tokens, address by address.
	bearers *throttle
	// checks bounds the password checks of everyone together.
	checks *checks

	// state keeps the keys, logins and tokens; nil, and no agent gets in.
	state     *state.State
	access    *access
	documents *documents
	oauth     *oauthState
	// mediaLink reports whether a /media/ path is a live link, which its
	// own secret opens.
	mediaLink func(path string) bool
	now       func() time.Time
}

// New is the public listener's handler in front of the daemon's routes.
func New(cfg *Config, routes *http.ServeMux, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{cfg: cfg, routes: routes, logger: logger, sessions: newSessions(), throttle: newThrottle(),
		bearers: newThrottle(), checks: newChecks(logger), documents: newDocuments(), oauth: newOAuthState(), now: time.Now}
}

// UseState lets agents in, with the keys and logins kept in st.
func (s *Server) UseState(st *state.State) {
	s.state = st
	s.access = newAccess(s.cfg, st)
}

// Logins are the OAuth logins still valid: one approved with an older
// password is revoked here, so a list never shows it.
func (s *Server) Logins(ctx context.Context) ([]state.Connection, error) {
	if s.state == nil {
		return nil, nil
	}
	all, err := s.state.Connections(ctx)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, c := range all {
		if _, live := s.access.liveConnection(ctx, c.ID); live {
			out = append(out, c)
		}
	}
	return out, nil
}

// UseMediaLinks lets a download_media link through by its own secret.
func (s *Server) UseMediaLinks(live func(path string) bool) { s.mediaLink = live }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pr := s.cfg.proxies.read(r)
	if s.cfg.proxies.set && !pr.proxy {
		s.logger.Warn("public listener: refused a peer that is not the trusted proxy", "peer", r.RemoteAddr)
		http.Error(w, "this address answers only through its proxy", http.StatusForbidden)
		return
	}
	if s.cfg.HTTPS() && !pr.https {
		s.logger.Warn("public listener: refused a request that did not arrive over HTTPS", "peer", r.RemoteAddr,
			"proxy", pr.proxy, "x_forwarded_proto", truncate(r.Header.Get("X-Forwarded-Proto"), 20))
		http.Error(w, HTTPSRefused, http.StatusForbidden)
		return
	}
	// Whoever this is, it is not local: the caller is set here, before any
	// route runs, and a request with no credential is anonymous.
	who := caller.Caller{Kind: caller.Anonymous, Origin: s.cfg.Origin}
	ctx := caller.With(r.Context(), who)
	r = r.WithContext(ctx)

	// The MCP and the OAuth endpoints answer browser-based clients from any
	// site; they never take a cookie.
	if corsPath(r.URL.Path) {
		if r.Method == http.MethodOptions {
			preflight(w)
			return
		}
		cors(w)
	}

	switch r.URL.Path {
	case LoginPath:
		switch r.Method {
		case http.MethodGet, http.MethodHead:
			s.loginPage(w, r, "", 0, http.StatusOK)
		case http.MethodPost:
			s.login(w, r, pr)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	case LogoutPath:
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.logout(w, r)
		return
	case ProtectedResourcePath, ProtectedResourcePath + "/mcp", AuthServerPath, RegisterPath, TokenPath, AuthorizePath:
		s.oauthRoute(w, r, pr)
		return
	}

	// An agent's key or access token: it reaches the MCP, its media links
	// and the health report, never the panel (the routes check the caller).
	// A cookie is never an agent's credential.
	if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		bearer = strings.TrimSpace(bearer)
		// A valid key or token always goes through. Wrong ones slow down
		// their own address, up to 15 minutes (seconds for the address
		// every visitor shares): while it waits, its wrong tries are not
		// even counted, and the answer says when to try again.
		var agent caller.Caller
		valid := false
		if s.access != nil {
			agent, valid = s.access.authenticate(r.Context(), bearer)
		}
		if !valid {
			if wait := s.bearers.try(pr.client, pr.shared); wait > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(wait/time.Second)+1))
			} else {
				s.logger.Warn("public listener: refused a wrong key or token", "client", pr.client, "path", truncate(r.URL.Path, 40))
			}
			s.challenge(w, true)
			return
		}
		s.bearers.ok(pr.client)
		r = r.WithContext(caller.With(ctx, agent))
		_, pattern := s.routes.Handler(r)
		if _, classified := httpserver.RouteAccess(pattern); pattern != "" && !classified {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		s.routes.ServeHTTP(w, r)
		return
	}
	if r.URL.Path == "/mcp" {
		s.challenge(w, false)
		return
	}

	_, pattern := s.routes.Handler(r)
	access, classified := httpserver.RouteAccess(pattern)
	if s.signedIn(r) {
		who.Kind = caller.Panel
		r = r.WithContext(caller.With(ctx, who))
		if pattern != "" && !classified {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		s.routes.ServeHTTP(w, r)
		return
	}
	switch {
	case classified && access == caller.Open:
		s.routes.ServeHTTP(w, r)
	case classified && access == caller.Media && s.mediaLink != nil && (r.Method == http.MethodGet || r.Method == http.MethodHead) && s.mediaLink(r.URL.Path):
		// A download_media link: its own secret, random and short-lived,
		// is the credential.
		s.routes.ServeHTTP(w, r)
	case classified && access == caller.PanelAccess && (r.Method == http.MethodGet || r.Method == http.MethodHead) &&
		!strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/diagnostico/"):
		http.Redirect(w, r, LoginPath+"?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
	default:
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "sign in to the panel first", http.StatusUnauthorized)
	}
}

// oauthRoute answers the discovery documents and the OAuth endpoints.
func (s *Server) oauthRoute(w http.ResponseWriter, r *http.Request, pr peer) {
	get := r.Method == http.MethodGet || r.Method == http.MethodHead
	switch {
	case (r.URL.Path == ProtectedResourcePath || r.URL.Path == ProtectedResourcePath+"/mcp") && get:
		s.protectedResource(w)
	case r.URL.Path == AuthServerPath && get:
		s.authServer(w)
	case r.URL.Path == RegisterPath && r.Method == http.MethodPost:
		s.register(w, r, pr)
	case r.URL.Path == TokenPath && r.Method == http.MethodPost:
		s.token(w, r, pr)
	case r.URL.Path == AuthorizePath && get:
		s.authorizePage(w, r, pr)
	case r.URL.Path == AuthorizePath && r.Method == http.MethodPost:
		s.authorize(w, r, pr)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// checkPassword checks a password typed on the sign-in or the consent page,
// in its turn among everyone's: busy means the turn did not come within a
// few seconds. A right password clears the address's wrong tries and gives
// it a reserve of turns of its own.
func (s *Server) checkPassword(ctx context.Context, pr peer, hash, password string) (right, busy bool) {
	if password == "" {
		return false, false
	}
	release, ok := s.checks.acquire(ctx, pr.client)
	if !ok {
		return false, true
	}
	right = verifyPassword(hash, password)
	release()
	if right {
		s.throttle.ok(pr.client)
		if !pr.shared {
			s.checks.trust(pr.client)
		}
	}
	return right, false
}

// verifyPassword is verify, replaceable in tests.
var verifyPassword = verify

// signedIn reports whether the request carries a live panel session, made
// with the password in force.
func (s *Server) signedIn(r *http.Request) bool {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return false
	}
	hash, err := s.cfg.Password.Current()
	if err != nil {
		return false
	}
	return s.sessions.valid(c.Value, fingerprint(hash))
}

// fromOwnPage reports whether a sign-in or sign-out was sent by the panel's
// own page at the public address: a browser always sends Origin on a form
// post, and another site cannot send this one.
func (s *Server) fromOwnPage(r *http.Request) bool {
	site := r.Header.Get("Sec-Fetch-Site")
	return r.Header.Get("Origin") == s.cfg.Origin && (site == "" || site == "same-origin")
}

func (s *Server) login(w http.ResponseWriter, r *http.Request, pr peer) {
	if !s.fromOwnPage(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	password := r.PostFormValue("password")
	hash, err := s.cfg.Password.Current()
	if err != nil {
		s.logger.Error("public listener: the panel password is missing or invalid; nobody can sign in", "error", err)
		http.Error(w, "the panel password is not set up", http.StatusServiceUnavailable)
		return
	}
	// The try is counted before the password is checked, in one step, so
	// tries sent at once from one address do not all slip in.
	if wait := s.throttle.try(pr.client, pr.shared); wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Round(time.Second)/time.Second)+1))
		s.loginPage(w, r, "", wait, http.StatusTooManyRequests)
		return
	}
	right, busy := s.checkPassword(r.Context(), pr, hash, password)
	if busy {
		w.Header().Set("Retry-After", "5")
		s.loginPage(w, r, i18n.T("O servidor está recebendo muitas tentativas de senha agora. Tente de novo em alguns segundos."), 0, http.StatusTooManyRequests)
		return
	}
	if !right {
		s.logger.Warn("panel sign-in refused: wrong password", "client", pr.client)
		s.loginPage(w, r, i18n.T("Senha errada."), 0, http.StatusUnauthorized)
		return
	}
	id := s.sessions.create(fingerprint(hash))
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: id, Path: "/", HttpOnly: true, Secure: s.cfg.HTTPS(),
		SameSite: http.SameSiteLaxMode, MaxAge: int(sessionMaxAge / time.Second)})
	s.logger.Info("panel sign-in", "client", pr.client)
	http.Redirect(w, r, safeReturn(r.PostFormValue("next")), http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if !s.fromOwnPage(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if c, err := r.Cookie(CookieName); err == nil {
		s.sessions.end(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", HttpOnly: true, Secure: s.cfg.HTTPS(),
		SameSite: http.SameSiteLaxMode, MaxAge: -1})
	http.Redirect(w, r, LoginPath, http.StatusSeeOther)
}

// safeReturn is where to go after signing in: a page of this server, never
// another site.
func safeReturn(to string) string {
	// A browser drops tabs and line breaks from a Location and reads a
	// backslash as a slash, so "/\t/evil.example" would leave the site:
	// neither is accepted anywhere in the address.
	if !strings.HasPrefix(to, "/") || strings.HasPrefix(to, "//") || strings.ContainsRune(to, '\\') ||
		strings.ContainsFunc(to, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "/"
	}
	return to
}

var loginTemplate = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title><link rel="icon" href="/favicon.svg">
<style>
body{margin:0;min-height:100vh;display:grid;place-items:center;font:16px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif;background:#f4f1ea;color:#1f2a24}
main{width:min(360px,calc(100% - 32px));background:#fff;border-radius:14px;padding:28px;box-shadow:0 2px 14px rgba(0,0,0,.08)}
h1{font-size:20px;margin:0 0 4px}p{margin:0 0 18px;color:#5b6660}
label{display:block;font-weight:600;margin-bottom:6px}
input{box-sizing:border-box;width:100%;padding:10px 12px;font:inherit;border:1px solid #c9cfc9;border-radius:8px}
button{margin-top:16px;width:100%;padding:11px;font:inherit;font-weight:600;border:0;border-radius:8px;background:#1f7a4d;color:#fff;cursor:pointer}
.error{background:#fbeed2;color:#6b4a00;border-radius:8px;padding:10px 12px;margin-bottom:14px}
@media (prefers-color-scheme:dark){body{background:#151a17;color:#e6ebe7}main{background:#1f2622;box-shadow:none}p{color:#a6b0aa}input{background:#151a17;color:inherit;border-color:#3a443e}}
</style></head>
<body><main>
<h1>{{.Title}}</h1><p>{{.Lead}}</p>
{{if .Error}}<div class="error" role="alert">{{.Error}}</div>{{end}}
<form method="post" action="` + LoginPath + `">
<input type="hidden" name="next" value="{{.Return}}">
<label for="password">{{.Label}}</label>
<input id="password" name="password" type="password" autocomplete="current-password" required autofocus>
<button type="submit">{{.Button}}</button>
</form></main></body></html>`))

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request, problem string, wait time.Duration, status int) {
	if wait > 0 {
		problem = i18n.Tf("Muitas tentativas com a senha errada. Tente de novo em %d segundos.", int(wait.Round(time.Second)/time.Second)+1)
	}
	ret := r.URL.Query().Get("next")
	if r.Method == http.MethodPost {
		ret = r.PostFormValue("next")
	}
	var body bytes.Buffer
	_ = loginTemplate.Execute(&body, map[string]string{
		"Lang": i18n.Lang(), "Title": i18n.T("Entrar no painel"), "Lead": i18n.T("WhatsApp MCP neste servidor. Digite a senha do painel."),
		"Label": i18n.T("Senha"), "Button": i18n.T("Entrar"), "Error": problem, "Return": safeReturn(ret),
	})
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; frame-ancestors 'none'; form-action 'self'")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}

func truncate(v string, n int) string {
	if len(v) > n {
		return v[:n]
	}
	return v
}
