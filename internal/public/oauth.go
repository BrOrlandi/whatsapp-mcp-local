package public

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

// The OAuth 2.1 server for MCP clients (MCP authorization, 2026-07-28):
// protected resource metadata (RFC 9728), authorization server metadata
// (RFC 8414), clients by metadata document or by registration (RFC 7591),
// the authorization code with PKCE S256 only, resource indicators (RFC
// 8707), and the issuer in every authorization response (RFC 9207). The
// consent is the panel's password.
const (
	ProtectedResourcePath = "/.well-known/oauth-protected-resource"
	AuthServerPath        = "/.well-known/oauth-authorization-server"
	AuthorizePath         = "/oauth/authorize"
	TokenPath             = "/oauth/token"
	RegisterPath          = "/oauth/register"
)

// OAuthRoutes are the public listener's OAuth routes, all open: they are
// how an agent comes in. (OwnRoutes are the panel's sign-in.)
var OAuthRoutes = []string{
	"GET " + ProtectedResourcePath, "GET " + ProtectedResourcePath + "/mcp", "GET " + AuthServerPath,
	"POST " + RegisterPath, "GET " + AuthorizePath, "POST " + AuthorizePath, "POST " + TokenPath,
}

const (
	codeTTL         = time.Minute
	maxClients      = 500
	registerPerHour = 20
	// clientsPerAddress is how many registrations one address (or /64)
	// may have waiting for a sign-in: 2% of maxClients.
	clientsPerAddress = 10
	unusedClientFor   = time.Hour
)

// pending is an authorization code waiting to be exchanged.
type pending struct {
	clientID, clientName, redirectURI, challenge, resource string
	readOnly                                               bool
	expires                                                time.Time
	// connection is set once the code was exchanged: a second use of the
	// code ends that connection.
	connection string
}

type oauthState struct {
	mu    sync.Mutex
	codes map[string]*pending // by the code's hash
	// registrations counts recent registrations by client address.
	registrations *window
}

func newOAuthState() *oauthState {
	return &oauthState{codes: map[string]*pending{}, registrations: newWindow(4096)}
}

func (s *Server) resourceURL() string { return s.cfg.Origin + "/mcp" }
func (s *Server) metadataURL() string { return s.cfg.Origin + ProtectedResourcePath }

// cors lets a page on any site call the MCP and the OAuth endpoints, as
// browser-based MCP clients do: no cookie is ever sent or accepted there,
// so nothing of the panel's session reaches them.
func cors(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Expose-Headers", "WWW-Authenticate, Mcp-Session-Id, Mcp-Protocol-Version")
}

// preflight answers a browser's CORS check.
func preflight(w http.ResponseWriter) {
	cors(w)
	h := w.Header()
	h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, Mcp-Protocol-Version, Mcp-Session-Id, Mcp-Method, Last-Event-ID")
	h.Set("Access-Control-Max-Age", "600")
	w.WriteHeader(http.StatusNoContent)
}

func corsPath(p string) bool {
	return p == "/mcp" || strings.HasPrefix(p, "/.well-known/") || p == TokenPath || p == RegisterPath
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func oauthError(w http.ResponseWriter, status int, code, description string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": description})
}

// challenge is the 401 that sends an agent to the metadata.
func (s *Server) challenge(w http.ResponseWriter, invalid bool) {
	v := `Bearer resource_metadata="` + s.metadataURL() + `"`
	if invalid {
		v += `, error="invalid_token", error_description="the key or token is not valid"`
	}
	w.Header().Set("WWW-Authenticate", v)
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized",
		"error_description": "this MCP asks for an API key or an OAuth login: see " + s.metadataURL()})
}

func (s *Server) protectedResource(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 s.resourceURL(),
		"authorization_servers":    []string{s.cfg.Origin},
		"bearer_methods_supported": []string{"header"},
		"resource_name":            "WhatsApp MCP",
	})
}

func (s *Server) authServer(w http.ResponseWriter) {
	o := s.cfg.Origin
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                         o,
		"authorization_endpoint":                         o + AuthorizePath,
		"token_endpoint":                                 o + TokenPath,
		"registration_endpoint":                          o + RegisterPath,
		"response_types_supported":                       []string{"code"},
		"response_modes_supported":                       []string{"query"},
		"grant_types_supported":                          []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":               []string{"S256"},
		"token_endpoint_auth_methods_supported":          []string{"none"},
		"client_id_metadata_document_supported":          true,
		"authorization_response_iss_parameter_supported": true,
	})
}

// register is dynamic client registration (RFC 7591), kept for clients
// that do not use a metadata document. It is open to the internet, so it is
// limited per address and in total.
func (s *Server) register(w http.ResponseWriter, r *http.Request, pr peer) {
	if s.state == nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "sign-in for agents is not available")
		return
	}
	now := s.now()
	limit := registerPerHour
	if pr.shared {
		limit *= 5
	}
	if !s.oauth.registrations.allow(pr.client, limit) {
		w.Header().Set("Retry-After", "3600")
		oauthError(w, http.StatusTooManyRequests, "too_many_requests", "too many registrations from this address; try again later")
		return
	}

	var req struct {
		RedirectURIs []string `json:"redirect_uris"`
		ClientName   string   `json:"client_name"`
		GrantTypes   []string `json:"grant_types"`
		Response     []string `json:"response_types"`
		AuthMethod   string   `json:"token_endpoint_auth_method"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req) != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "the body must be a JSON client metadata document")
		return
	}
	if len(req.RedirectURIs) == 0 || len(req.RedirectURIs) > 10 {
		oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", "give between 1 and 10 redirect_uris")
		return
	}
	for _, u := range req.RedirectURIs {
		if !validRedirect(u) || len(u) > 2000 {
			oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uris must be https, or http on a loopback address")
			return
		}
	}
	for _, g := range req.GrantTypes {
		if g != "authorization_code" && g != "refresh_token" {
			oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "only authorization_code and refresh_token are supported")
			return
		}
	}
	ctx := r.Context()
	// Registrations nobody signed in with go after an hour, and one address
	// (or /64) holds at most a small share of the total.
	_, _ = s.state.PruneOAuthClients(ctx, now.Add(-unusedClientFor))
	from := addrKey(pr.client)
	if n, err := s.state.UnusedOAuthClientsFrom(ctx, from); err == nil && n >= clientsPerAddress && !pr.shared {
		w.Header().Set("Retry-After", "3600")
		oauthError(w, http.StatusTooManyRequests, "too_many_requests", "too many clients from this address are waiting for a sign-in; try again later")
		return
	}
	if n, err := s.state.OAuthClientCount(ctx); err == nil && n >= maxClients {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "too many clients are registered; try again later")
		return
	}
	host := ""
	if u, err := url.Parse(req.RedirectURIs[0]); err == nil {
		host = u.Hostname()
	}
	c := registeredClient(cleanName(req.ClientName, host), req.RedirectURIs, now)
	c.From = from
	if err := s.state.AddOAuthClient(ctx, c); err != nil {
		oauthError(w, http.StatusInternalServerError, "server_error", "could not register the client")
		return
	}
	s.logger.Info("oauth client registered", "client", c.ID, "name", c.Name, "address", pr.client)
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id": c.ID, "client_id_issued_at": c.CreatedAt.Unix(), "client_name": c.Name,
		"redirect_uris": c.RedirectURIs, "grant_types": []string{"authorization_code", "refresh_token"},
		"response_types": []string{"code"}, "token_endpoint_auth_method": "none",
	})
}

// authRequest is an authorization request, as it arrives and as the
// consent form carries it back.
type authRequest struct {
	ClientID, RedirectURI, State, Challenge, Method, Resource, ResponseType string
}

func readAuthRequest(v url.Values) authRequest {
	return authRequest{ClientID: v.Get("client_id"), RedirectURI: v.Get("redirect_uri"), State: v.Get("state"),
		Challenge: v.Get("code_challenge"), Method: v.Get("code_challenge_method"), Resource: v.Get("resource"),
		ResponseType: v.Get("response_type")}
}

// resourceOK reports whether a resource indicator names this server's MCP
// (its address, or the origin alone); none given means this one.
func (s *Server) resourceOK(resource string) bool {
	if resource == "" {
		return true
	}
	u, err := url.Parse(resource)
	if err != nil || u.Fragment != "" {
		return false
	}
	got := strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + strings.TrimSuffix(u.EscapedPath(), "/")
	return got == s.cfg.Origin+"/mcp" || got == s.cfg.Origin
}

// sendBack redirects to the client with the result, and the issuer (RFC
// 9207) in every case.
func (s *Server) sendBack(w http.ResponseWriter, r *http.Request, ar authRequest, params url.Values) {
	u, _ := url.Parse(ar.RedirectURI)
	q := u.Query()
	for k, v := range params {
		q[k] = v
	}
	if ar.State != "" {
		q.Set("state", ar.State)
	}
	q.Set("iss", s.cfg.Origin)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}

// checkRequest validates an authorization request. A problem with the
// client or its redirect address is shown here, never sent to an address
// not proven to be the client's; anything else goes back to the client.
func (s *Server) checkRequest(ctx context.Context, ar authRequest, addr string) (c client, problem string, backErr string) {
	if s.state == nil {
		return c, i18n.T("o login de agentes não está disponível neste servidor"), ""
	}
	if ar.ClientID == "" {
		return c, i18n.T("o pedido de acesso não diz qual aplicativo o fez"), ""
	}
	c, err := s.resolveClient(ctx, ar.ClientID, addr)
	if err != nil {
		return c, err.Error(), ""
	}
	if ar.RedirectURI == "" || !redirectAllowed(c.RedirectURIs, ar.RedirectURI) {
		return c, i18n.T("o endereço de retorno deste pedido não é um dos que o aplicativo declarou; por segurança, o acesso não continua"), ""
	}
	switch {
	case ar.ResponseType != "code":
		return c, "", "unsupported_response_type"
	case ar.Method != "S256" || len(ar.Challenge) < 43 || len(ar.Challenge) > 128:
		return c, "", "invalid_request"
	case !s.resourceOK(ar.Resource):
		return c, "", "invalid_target"
	}
	return c, "", ""
}

func (s *Server) authorizePage(w http.ResponseWriter, r *http.Request, pr peer) {
	ar := readAuthRequest(r.URL.Query())
	c, problem, backErr := s.checkRequest(r.Context(), ar, pr.client)
	switch {
	case problem != "":
		s.consentError(w, problem)
	case backErr != "":
		// Anyone may register a client, so nothing is redirected to its
		// address before the person decides: the error is shown here.
		s.consentError(w, requestProblem(backErr))
	default:
		s.consent(w, r, ar, c, "", http.StatusOK)
	}
}

// requestProblem says, for the person, what is wrong with a request.
func requestProblem(code string) string {
	switch code {
	case "invalid_target":
		return i18n.T("o pedido de acesso é para outro endereço, não para este servidor")
	case "unsupported_response_type":
		return i18n.T("o aplicativo pediu um tipo de acesso que este servidor não oferece")
	}
	return i18n.T("o pedido de acesso não usa a proteção que este servidor exige (PKCE S256)")
}

// authorize is the consent form's answer.
func (s *Server) authorize(w http.ResponseWriter, r *http.Request, pr peer) {
	if !s.fromOwnPage(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ar := readAuthRequest(r.PostForm)
	c, problem, backErr := s.checkRequest(r.Context(), ar, pr.client)
	switch {
	case problem != "":
		s.consentError(w, problem)
		return
	case backErr != "":
		s.consentError(w, requestProblem(backErr))
		return
	}
	if r.PostFormValue("decision") != "allow" {
		s.logger.Info("oauth: access refused by the person", "client", c.Name)
		s.sendBack(w, r, ar, url.Values{"error": {"access_denied"}})
		return
	}
	// The person chooses what the app may do: nothing is preselected.
	access := r.PostFormValue("access")
	if access != "full" && access != "read" {
		s.consent(w, r, ar, c, i18n.T("Escolha o que este aplicativo pode fazer: tudo, ou só ler."), http.StatusBadRequest)
		return
	}
	hash, err := s.cfg.Password.Current()
	if err != nil {
		http.Error(w, "the panel password is not set up", http.StatusServiceUnavailable)
		return
	}
	// The password, unless signed in and this client was approved to come
	// back to this host before: a registered client chooses its own name,
	// so a session alone never lets in one the person has not seen yet.
	if s.needsPassword(r, ar) {
		if wait := s.throttle.try(pr.client, pr.shared); wait > 0 {
			s.consent(w, r, ar, c, i18n.Tf("Muitas tentativas com a senha errada. Tente de novo em %d segundos.", int(wait/time.Second)+1), http.StatusTooManyRequests)
			return
		}
		right, busy := s.checkPassword(r.Context(), pr, hash, r.PostFormValue("password"))
		if busy {
			w.Header().Set("Retry-After", "5")
			s.consent(w, r, ar, c, i18n.T("O servidor está recebendo muitas tentativas de senha agora. Tente de novo em alguns segundos."), http.StatusTooManyRequests)
			return
		}
		if !right {
			s.logger.Warn("oauth: wrong password on the consent page", "client", pr.client)
			s.consent(w, r, ar, c, i18n.T("Senha errada."), http.StatusUnauthorized)
			return
		}
	}
	code := secret(codePrefix)
	s.oauth.mu.Lock()
	now := s.now()
	for k, p := range s.oauth.codes {
		if now.After(p.expires.Add(10 * time.Minute)) {
			delete(s.oauth.codes, k)
		}
	}
	s.oauth.codes[hashSecret(code)] = &pending{clientID: c.ID, clientName: c.Name, redirectURI: ar.RedirectURI,
		challenge: ar.Challenge, resource: ar.Resource, readOnly: access == "read", expires: now.Add(codeTTL)}
	s.oauth.mu.Unlock()
	_ = s.state.Approve(r.Context(), c.ID, redirectHost(ar.RedirectURI), now)
	s.logger.Info("oauth: access approved", "client", c.Name, "host", clientHost(c, ar.RedirectURI), "read_only", access == "read", "address", pr.client)
	s.sendBack(w, r, ar, url.Values{"code": {code}})
}

// token is the token endpoint: a code for the first tokens, a refresh token
// for the next ones.
func (s *Server) token(w http.ResponseWriter, r *http.Request, pr peer) {
	if s.state == nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "sign-in for agents is not available")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "the body must be application/x-www-form-urlencoded")
		return
	}
	f := r.PostForm
	if f.Get("client_secret") != "" || r.Header.Get("Authorization") != "" {
		oauthError(w, http.StatusUnauthorized, "invalid_client", "this server has public clients only (token_endpoint_auth_method none)")
		return
	}
	switch f.Get("grant_type") {
	case "authorization_code":
		s.exchangeCode(w, r.Context(), f)
	case "refresh_token":
		s.refresh(w, r.Context(), f, pr)
	default:
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "use authorization_code or refresh_token")
	}
}

// afterCodeClaimed runs once a code is claimed for an exchange; tests use
// it to replay the code at that moment.
var afterCodeClaimed = func() {}

func (s *Server) exchangeCode(w http.ResponseWriter, ctx context.Context, f url.Values) {
	key := hashSecret(f.Get("code"))
	now := s.now()
	s.oauth.mu.Lock()
	p := s.oauth.codes[key]
	if p == nil || now.After(p.expires) && p.connection == "" {
		s.oauth.mu.Unlock()
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the code is not valid")
		return
	}
	if p.connection != "" {
		// A code used twice: whoever holds it is not to be trusted, and the
		// connection it made ends.
		conn := p.connection
		s.oauth.mu.Unlock()
		_ = s.state.RevokeConnection(ctx, conn, now)
		s.logger.Warn("oauth: an authorization code was used twice; its connection is revoked", "connection", conn)
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the code was already used")
		return
	}
	verifier := f.Get("code_verifier")
	sum := sha256.Sum256([]byte(verifier))
	ok := f.Get("client_id") == p.clientID && f.Get("redirect_uri") == p.redirectURI && len(verifier) >= 43 && len(verifier) <= 128 &&
		subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(p.challenge)) == 1 &&
		(f.Get("resource") == "" || s.resourceOK(f.Get("resource")))
	if !ok {
		delete(s.oauth.codes, key)
		s.oauth.mu.Unlock()
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the code does not match this request (client, redirect_uri, code_verifier or resource)")
		return
	}
	conn := state.Connection{ID: shortID("c_"), ClientID: p.clientID, ClientName: p.clientName, ReadOnly: p.readOnly,
		Resource: s.resourceURL(), CreatedAt: now.UTC()}
	hash, err := s.cfg.Password.Current()
	if err != nil {
		s.oauth.mu.Unlock()
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "the panel password is not set up")
		return
	}
	conn.Password = passwordFingerprint(hash)
	// The connection is kept before the code is marked used and the lock
	// let go: a replay of the code always finds the connection to end.
	if err := s.state.AddConnection(ctx, conn); err != nil {
		delete(s.oauth.codes, key)
		s.oauth.mu.Unlock()
		oauthError(w, http.StatusInternalServerError, "server_error", "could not keep the connection")
		return
	}
	p.connection = conn.ID
	s.oauth.mu.Unlock()
	afterCodeClaimed()
	if !isDocumentID(p.clientID) {
		_ = s.state.TouchOAuthClient(ctx, p.clientID, now)
	}
	s.tokens(w, ctx, conn.ID)
	s.logger.Info("oauth: connection made", "connection", conn.ID, "client", conn.ClientName, "read_only", conn.ReadOnly)
}

func (s *Server) refresh(w http.ResponseWriter, ctx context.Context, f url.Values, pr peer) {
	rt := f.Get("refresh_token")
	t, err := s.state.TokenByHash(ctx, hashSecret(rt))
	if err != nil || t.Kind != "refresh" {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the refresh token is not valid")
		return
	}
	conn, live := s.access.liveConnection(ctx, t.Connection)
	if !live || f.Get("client_id") != conn.ClientID || (f.Get("resource") != "" && !s.resourceOK(f.Get("resource"))) {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the refresh token is not valid")
		return
	}
	spent, err := s.state.SpendRefreshToken(ctx, hashSecret(rt), s.now())
	if err == nil && spent {
		// The new access token replaces the old ones.
		_ = s.state.DeleteAccessTokens(ctx, conn.ID)
	}
	if err != nil {
		oauthError(w, http.StatusInternalServerError, "server_error", "could not refresh")
		return
	}
	if !spent {
		// A refresh token used twice was copied: the connection ends.
		_ = s.state.RevokeConnection(ctx, conn.ID, s.now())
		s.logger.Warn("oauth: a refresh token was used twice; its connection is revoked", "connection", conn.ID, "address", pr.client)
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the refresh token was already used; the connection is revoked")
		return
	}
	s.tokens(w, ctx, conn.ID)
}

func (s *Server) tokens(w http.ResponseWriter, ctx context.Context, connection string) {
	at, rt, err := s.access.issue(ctx, connection)
	if err != nil {
		oauthError(w, http.StatusInternalServerError, "server_error", "could not issue tokens")
		return
	}
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusOK, map[string]any{"access_token": at, "token_type": "Bearer",
		"expires_in": int(accessTTL / time.Second), "refresh_token": rt})
}

// ---- the consent page ----

var consentTemplate = template.Must(template.New("consent").Parse(`<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title><link rel="icon" href="/favicon.svg">
<style>
body{margin:0;min-height:100vh;display:grid;place-items:center;font:16px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif;background:#f4f1ea;color:#1f2a24}
main{width:min(440px,calc(100% - 32px));background:#fff;border-radius:14px;padding:28px;box-shadow:0 2px 14px rgba(0,0,0,.08);margin:24px 0}
h1{font-size:20px;margin:0 0 8px}p{margin:0 0 14px}.muted{color:#5b6660;font-size:14px}
.who{background:#f4f1ea;border-radius:8px;padding:10px 12px;margin-bottom:14px;font-size:14px;overflow-wrap:anywhere}
fieldset{border:1px solid #c9cfc9;border-radius:8px;margin:0 0 14px;padding:8px 12px}legend{font-weight:600;padding:0 4px}
fieldset label{display:block;margin:6px 0;font-weight:400}
label.field{display:block;font-weight:600;margin-bottom:6px}
input[type=password]{box-sizing:border-box;width:100%;padding:10px 12px;font:inherit;border:1px solid #c9cfc9;border-radius:8px;margin-bottom:14px}
.actions{display:flex;gap:10px}button{flex:1;padding:11px;font:inherit;font-weight:600;border-radius:8px;cursor:pointer}
.allow{border:0;background:#1f7a4d;color:#fff}.deny{border:1px solid #c9cfc9;background:transparent;color:inherit}
.error,.warn{background:#fbeed2;color:#6b4a00;border-radius:8px;padding:10px 12px;margin-bottom:14px}
@media (prefers-color-scheme:dark){body{background:#151a17;color:#e6ebe7}main{background:#1f2622;box-shadow:none}.muted{color:#a6b0aa}.who{background:#151a17}input{background:#151a17;color:inherit;border-color:#3a443e}}
</style></head>
<body><main>
<h1>{{.Title}}</h1>
{{if .Problem}}<div class="error" role="alert">{{.Problem}}</div>{{else}}
<p class="lead"><strong>{{.Lead}}</strong></p>
<div class="who">{{.Back}}<br><span class="muted">{{.From}}</span></div>
{{if .Loopback}}<div class="warn">{{.LoopbackWarning}}</div>{{end}}
{{if .Error}}<div class="error" role="alert">{{.Error}}</div>{{end}}
<form method="post" action="` + AuthorizePath + `">
{{range $k, $v := .Hidden}}<input type="hidden" name="{{$k}}" value="{{$v}}">{{end}}
<fieldset><legend>{{.AccessLabel}}</legend>
<label><input type="radio" name="access" value="full" required> {{.Full}}</label>
<label><input type="radio" name="access" value="read" required> {{.Read}}</label>
</fieldset>
{{if .AskPassword}}<label class="field" for="password">{{.PasswordLabel}}</label>
<input id="password" name="password" type="password" autocomplete="current-password" autofocus>{{end}}
<div class="actions"><button class="deny" type="submit" name="decision" value="deny" formnovalidate>{{.Deny}}</button>
<button class="allow" type="submit" name="decision" value="allow">{{.Allow}}</button></div>
</form>
<p class="muted" style="margin-top:14px">{{.Note}}</p>{{end}}
</main></body></html>`))

func pageHeaders(w http.ResponseWriter, status int) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Frame-Options", "DENY")
	// form-action stays open: the approval redirects to the client's own
	// address, which a stricter policy would block in some browsers.
	h.Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; frame-ancestors 'none'")
	// same-origin, not no-referrer: a form posted from a no-referrer page
	// carries Origin: null, which the same-origin check refuses.
	h.Set("Referrer-Policy", "same-origin")
	w.WriteHeader(status)
}

func (s *Server) consentError(w http.ResponseWriter, problem string) {
	var body bytes.Buffer
	_ = consentTemplate.Execute(&body, map[string]any{"Lang": i18n.Lang(), "Title": i18n.T("Pedido de acesso recusado"), "Problem": problem})
	pageHeaders(w, http.StatusBadRequest)
	_, _ = w.Write(body.Bytes())
}

// redirectHost is the host a redirect address sends the person back to,
// without a port.
func redirectHost(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return strings.ToLower(u.Hostname())
	}
	return ""
}

// clientHost is what the consent page leads with: the host the person can
// check. A registered client chooses its own name, so it is the host the
// person is sent back to; a client known by its metadata document, the
// document's host.
func clientHost(c client, redirect string) string {
	if c.Document {
		if u, err := url.Parse(c.ID); err == nil {
			return strings.ToLower(u.Hostname())
		}
	}
	return redirectHost(redirect)
}

// needsPassword reports whether approving this client asks for the panel's
// password: always without a session, and the first time this client is
// approved to go back to this host.
func (s *Server) needsPassword(r *http.Request, ar authRequest) bool {
	if !s.signedIn(r) {
		return true
	}
	ok, err := s.state.Approved(r.Context(), ar.ClientID, redirectHost(ar.RedirectURI))
	return err != nil || !ok
}

func (s *Server) consent(w http.ResponseWriter, r *http.Request, ar authRequest, c client, problem string, status int) {
	back := ar.RedirectURI
	if u, err := url.Parse(ar.RedirectURI); err == nil {
		back = u.Host
	}
	var body bytes.Buffer
	_ = consentTemplate.Execute(&body, map[string]any{
		"Lang": i18n.Lang(), "Title": i18n.T("Permitir acesso ao seu WhatsApp"),
		"Lead":            i18n.Tf("%s quer usar o seu WhatsApp por este servidor.", clientHost(c, ar.RedirectURI)),
		"From":            i18n.Tf("O aplicativo se apresenta como “%s”: esse nome é dele e não é verificado.", c.Name),
		"Back":            i18n.Tf("Depois de aprovar, você volta para: %s", back),
		"Loopback":        loopbackOnly(c),
		"LoopbackWarning": i18n.T("Este aplicativo volta para um programa no computador de quem está conectando. Qualquer programa ali pode usar esse nome: só aprove se foi você que pediu agora."),
		"Error":           problem,
		"AccessLabel":     i18n.T("O que ele pode fazer"),
		"Full":            i18n.T("Tudo: ler, buscar, enviar e organizar"),
		"Read":            i18n.T("Só ler: vê e busca, mas não envia, não apaga e não mexe em grupos"),
		"AskPassword":     s.needsPassword(r, ar), "PasswordLabel": i18n.T("Senha do painel"),
		"Allow": i18n.T("Permitir"), "Deny": i18n.T("Recusar"),
		"Note": i18n.T("Você pode cortar este acesso quando quiser, no painel, em Conexões."),
		"Hidden": map[string]string{"client_id": ar.ClientID, "redirect_uri": ar.RedirectURI, "state": ar.State,
			"code_challenge": ar.Challenge, "code_challenge_method": ar.Method, "resource": ar.Resource, "response_type": ar.ResponseType},
	})
	pageHeaders(w, status)
	_, _ = w.Write(body.Bytes())
}
