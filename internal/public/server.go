package public

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
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
	// refused are wrong keys and tokens already looked up.
	refused *refused
	// bearerLog and peerLog keep repeated refusals to a line a minute.
	bearerLog, peerLog *logLimiter
	// checks bounds the password checks of everyone together.
	checks *checks
	// browsers are the known browsers, which have a reserve in checks.
	browsers browsers
	// earlierKeys is set on a server that has keys of a 2.0.0-beta.
	earlierKeys atomic.Bool

	// state keeps the keys, logins and tokens; nil, and no agent gets in.
	state     *state.State
	access    *access
	documents *documents
	oauth     *oauthState
	// mediaLink reports whether a /media/ path is a live link, which its
	// own secret opens.
	mediaLink func(path string) bool
	// phone is the paired number's digits, which the consent page shows
	// masked; nil or empty, it says no number is connected.
	phone func(ctx context.Context) string
	now   func() time.Time
}

// New is the public listener's handler in front of the daemon's routes.
func New(cfg *Config, routes *http.ServeMux, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{cfg: cfg, routes: routes, logger: logger, sessions: newSessions(), throttle: newThrottle(),
		bearers: newThrottle(), refused: newRefused(), checks: newChecks(logger), bearerLog: &logLimiter{interval: time.Minute}, peerLog: &logLimiter{interval: time.Minute}, documents: newDocuments(), oauth: newOAuthState(), now: time.Now}
}

// UseState lets agents in, with the keys and logins kept in st.
func (s *Server) UseState(st *state.State) {
	s.state = st
	s.access = newAccess(s.cfg, st)
	s.browsers.mu.Lock()
	s.browsers.loaded = false
	s.browsers.mu.Unlock()
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

// UsePhone tells the consent page which number it gives access to: phone
// returns its digits, empty while none is paired. The page shows it masked.
func (s *Server) UsePhone(phone func(ctx context.Context) string) { s.phone = phone }

func (s *Server) ownPhone(ctx context.Context) string {
	if s.phone == nil {
		return ""
	}
	return s.phone(ctx)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pr := s.cfg.proxies.read(r)
	if s.cfg.proxies.set && !pr.proxy {
		if ok, skipped := s.peerLog.allow(s.now()); ok {
			s.logger.Warn("public listener: refused a peer that is not the trusted proxy", "peer", r.RemoteAddr, "more_since_last", skipped)
		}
		http.Error(w, "this address answers only through its proxy", http.StatusForbidden)
		return
	}
	if s.cfg.HTTPS() && !pr.https {
		if ok, skipped := s.peerLog.allow(s.now()); ok {
			s.logger.Warn("public listener: refused a request that did not arrive over HTTPS", "peer", r.RemoteAddr,
				"proxy", pr.proxy, "x_forwarded_proto", truncate(r.Header.Get("X-Forwarded-Proto"), 20), "more_since_last", skipped)
		}
		http.Error(w, HTTPSRefused, http.StatusForbidden)
		return
	}
	// Over https, browsers are told to keep to it: the address only, not
	// the domain's other names, which may be someone else's, and for a day,
	// since it holds for every port of that name.
	if s.cfg.HTTPS() {
		w.Header().Set("Strict-Transport-Security", "max-age=86400")
	}
	// Every body is bounded before anything reads it; the routes read what
	// they need (a form, a JSON-RPC message) within the server's read
	// timeout, which does not cut a long tool call short.
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
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
		// A valid key or token always goes through, even from an address
		// that waits: a stale key on a shared address (a NAT, a proxy that
		// does not say whom it forwards) must not lock the owner's agents
		// out, so a new well-formed one is still looked up (a hash and an
		// indexed read). Wrong ones slow down their own address, up to 15
		// minutes (seconds for the address every visitor shares); the
		// answer says when to try again, and while the address waits its
		// wrong tries are not counted. While it waits, a wrong one it sent
		// within the last minute is refused again without a second lookup,
		// so a client stuck on a stale key does not read state.db at every
		// try. One that is not even shaped like a key or token is never
		// looked up.
		var agent caller.Caller
		valid := false
		if s.access != nil && (wellFormed(bearer) || s.earlierKeys.Load() && earlierKeyShape(bearer)) &&
			!(s.bearers.wait(pr.client) > 0 && s.refused.has(bearer, s.now())) {
			if agent, valid = authenticate(s.access, r.Context(), bearer); !valid {
				s.refused.add(bearer, s.now())
			}
		}
		if !valid {
			wait := s.bearers.try(pr.client, pr.shared)
			if wait > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(wait/time.Second)+1))
			}
			if ok, skipped := s.bearerLog.allow(s.now()); ok {
				s.logger.Warn("public listener: refused a wrong key or token", "client", pr.client, "path", logPath(r.URL.Path), "more_since_last", skipped)
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

// maxBody bounds every request body on the public address.
const maxBody = 2 << 20

// wellFormed reports whether a Bearer credential has the shape of a key or
// an access token this server hands out.
func wellFormed(bearer string) bool {
	rest, ok := strings.CutPrefix(bearer, KeyPrefix)
	if !ok {
		if rest, ok = strings.CutPrefix(bearer, accessPrefix); !ok {
			return false
		}
	}
	if len(rest) != 43 {
		return false
	}
	for _, c := range rest {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// earlierKeyShape reports whether a Bearer credential may be a key of a
// 2.0.0-beta test version: those were any secret of 32 characters or more.
// Only a server that has such keys looks one up (AcceptEarlierKeys).
func earlierKeyShape(bearer string) bool {
	if len(bearer) < legacyKeyMin || len(bearer) > 200 {
		return false
	}
	for _, prefix := range []string{KeyPrefix, accessPrefix, refreshPrefix, codePrefix, clientPrefix} {
		if strings.HasPrefix(bearer, prefix) {
			return false
		}
	}
	for _, c := range bearer {
		if c <= ' ' || c > '~' {
			return false
		}
	}
	return true
}

// AcceptEarlierKeys lets in the keys a 2.0.0-beta test version made, which
// ImportLegacyKeys brought into state.db: without it, a credential that is
// not shaped like this version's is refused without being looked up.
func (s *Server) AcceptEarlierKeys() { s.earlierKeys.Store(true) }

// logPath is a path as the log shows it: a media link's secret is left out.
func logPath(p string) string {
	if strings.HasPrefix(p, "/media/") {
		return "/media/…"
	}
	return truncate(p, 40)
}

// NewHTTPServer is the public listener's HTTP server: bounded headers, a
// request read whole within half a minute, idle connections closed after
// two minutes. No write timeout: a tool call may take minutes.
func NewHTTPServer(h http.Handler) *http.Server {
	return &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: readTimeout,
		IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 32 << 10}
}

// readTimeout is how long a request may take to arrive whole; tests shorten it.
var readTimeout = 30 * time.Second

// checkPassword checks a password typed on the sign-in or the consent page,
// in its turn among everyone's: busy means the turn did not come within a
// few seconds. A known browser (known, the hash of its cookie, or "")
// draws on its own reserve of turns. A right
// password clears the address's wrong tries and makes the browser known.
func (s *Server) checkPassword(w http.ResponseWriter, r *http.Request, pr peer, hash, password, known string) (right, busy bool) {
	if password == "" {
		return false, false
	}
	release, ok := s.checks.acquire(r.Context(), known)
	if !ok {
		if known == "" {
			s.throttle.undo(pr.client, pr.shared)
		}
		return false, true
	}
	defer release()
	right = verifyPassword(hash, password)
	if right {
		s.throttle.ok(pr.client)
		s.rememberBrowser(w, r, hash)
	}
	return right, false
}

// authenticate looks a Bearer credential up; tests count the lookups.
var authenticate = (*access).authenticate

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
	// tries sent at once from one address do not all slip in. A known
	// browser skips the address's count (its own reserve bounds it): behind
	// a proxy that forwards no address, everyone shares one, and a flood
	// would lock the owner out.
	known := s.knownBrowser(r, hash)
	if wait := s.throttle.tryUnless(known != "", pr.client, pr.shared); wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Round(time.Second)/time.Second)+1))
		s.loginPage(w, r, "", wait, http.StatusTooManyRequests)
		return
	}
	right, busy := s.checkPassword(w, r, pr, hash, password, known)
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

// SafeReturn is where to go after signing in: a page of this server, never
// another site.
func SafeReturn(to string) string { return safeReturn(to) }

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

var loginTemplate = page("login", `<main class="narrow">
<h1>{{.Title}}</h1><p class="lead">{{.Lead}}</p>
{{if .Error}}<div class="error" role="alert">{{.Error}}</div>{{end}}
<form method="post" action="`+LoginPath+`">
<input type="hidden" name="next" value="{{.Return}}">
<label class="field" for="password">{{.Label}}</label>
<input id="password" name="password" type="password" autocomplete="current-password" required autofocus>
<button class="allow wide" type="submit">{{.Button}}</button>
</form></main>`)

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
