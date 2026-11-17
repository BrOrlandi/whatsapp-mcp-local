package public

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

const origin = "https://wa.example.com"

// agentServer is the public listener with its state, in front of a
// stand-in for the daemon's routes that reports who reached it.
func agentServer(t *testing.T) (*Server, *state.State, string) {
	t.Helper()
	s, path := testServer(t, origin, "")
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s.UseState(st)
	mux := http.NewServeMux()
	report := func(w http.ResponseWriter, r *http.Request) {
		c := caller.From(r.Context())
		ro := "full"
		if c.ReadOnly() {
			ro = "read"
		}
		_, _ = w.Write([]byte(string(c.Kind) + "/" + c.ConnectionName + "/" + ro))
	}
	mux.HandleFunc("/mcp", report)
	mux.HandleFunc("/health", report)
	mux.HandleFunc("GET /status", report)
	s.routes = mux
	return s, st, path
}

// fromTunnel is a request as a tunnel on this computer forwards it.
func fromTunnel(method, target string, header map[string]string, body string) *http.Request {
	h := map[string]string{"X-Forwarded-Proto": "https", "X-Forwarded-For": "198.51.100.7"}
	for k, v := range header {
		h[k] = v
	}
	return request(method, target, "127.0.0.1:40000", h, body)
}

func jsonBody(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("%d %s: %v", rec.Code, rec.Body, err)
	}
}

func TestTheDiscoveryDocuments(t *testing.T) {
	s, _, _ := agentServer(t)
	rec := serve(s, fromTunnel("POST", "/mcp", nil, ""))
	if rec.Code != 401 || rec.Header().Get("WWW-Authenticate") != `Bearer resource_metadata="`+origin+`/.well-known/oauth-protected-resource"` {
		t.Fatalf("/mcp without a credential: %d %q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
	for _, p := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
		var prm struct {
			Resource string   `json:"resource"`
			AS       []string `json:"authorization_servers"`
		}
		jsonBody(t, serve(s, fromTunnel("GET", p, nil, "")), &prm)
		if prm.Resource != origin+"/mcp" || len(prm.AS) != 1 || prm.AS[0] != origin {
			t.Errorf("%s: %+v", p, prm)
		}
	}
	var as map[string]any
	jsonBody(t, serve(s, fromTunnel("GET", "/.well-known/oauth-authorization-server", nil, "")), &as)
	for k, want := range map[string]any{"issuer": origin, "client_id_metadata_document_supported": true,
		"authorization_response_iss_parameter_supported": true, "token_endpoint": origin + "/oauth/token"} {
		if as[k] != want {
			t.Errorf("%s = %v, want %v", k, as[k], want)
		}
	}
	for k, want := range map[string]string{"code_challenge_methods_supported": "S256", "token_endpoint_auth_methods_supported": "none"} {
		if l, _ := as[k].([]any); len(l) != 1 || l[0] != want {
			t.Errorf("%s = %v", k, as[k])
		}
	}
	if _, ok := as["scopes_supported"]; ok {
		t.Error("scopes are advertised")
	}
	// CORS for browser clients, never with credentials.
	pre := serve(s, fromTunnel("OPTIONS", "/oauth/token", map[string]string{"Origin": "https://inspector.example", "Access-Control-Request-Method": "POST"}, ""))
	if pre.Code != 204 || pre.Header().Get("Access-Control-Allow-Origin") != "*" || pre.Header().Get("Access-Control-Allow-Credentials") != "" ||
		!strings.Contains(pre.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Errorf("preflight: %d %v", pre.Code, pre.Header())
	}
	if h := rec.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(h, "WWW-Authenticate") {
		t.Errorf("/mcp does not expose WWW-Authenticate: %q", h)
	}
}

func TestAPIKeys(t *testing.T) {
	s, st, _ := agentServer(t)
	ctx := context.Background()
	now := time.Now()
	s.access.now = func() time.Time { return now }
	key, k, err := CreateKey(ctx, st, "Claude Code", false, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, KeyPrefix) || len(key) < len(KeyPrefix)+43 {
		t.Fatalf("the key %q is short", key[:len(KeyPrefix)])
	}
	roKey, _, _ := CreateKey(ctx, st, "Leitor", true, now)
	call := func(key, path string) *httptest.ResponseRecorder {
		return serve(s, fromTunnel("POST", path, map[string]string{"Authorization": "Bearer " + key}, "{}"))
	}
	if rec := call(key, "/mcp"); rec.Code != 200 || rec.Body.String() != "apikey/Claude Code/full" {
		t.Fatalf("a key: %d %s", rec.Code, rec.Body)
	}
	if rec := call(roKey, "/mcp"); rec.Body.String() != "apikey/Leitor/read" {
		t.Fatalf("a read-only key: %s", rec.Body)
	}
	// Only its hash is kept.
	if _, err := st.APIKeyByHash(ctx, key); err == nil {
		t.Fatal("the key is stored as it is")
	}
	// The key survives a restart: another server on the same state.
	s2, _, _ := agentServer(t)
	s2.UseState(st)
	if rec := call2(s2, key); rec.Code != 200 {
		t.Fatalf("after a restart: %d", rec.Code)
	}
	// Revoked, it stops at once.
	if err := RevokeKey(ctx, st, k.ID, now); err != nil {
		t.Fatal(err)
	}
	if rec := call(key, "/mcp"); rec.Code != 401 || !strings.Contains(rec.Header().Get("WWW-Authenticate"), `error="invalid_token"`) {
		t.Fatalf("a revoked key: %d %q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
	// A key nobody uses in its first day stops working; one used keeps going.
	unused, _, _ := CreateKey(ctx, st, "Esquecida", false, now)
	now = now.Add(keyUnusedFor + time.Minute)
	if rec := call(unused, "/mcp"); rec.Code != 401 {
		t.Fatalf("an unused key after a day: %d", rec.Code)
	}
	if rec := call(roKey, "/mcp"); rec.Code != 200 {
		t.Fatalf("a used key after a day: %d", rec.Code)
	}
	keys, _ := Keys(ctx, st, now)
	expired := map[string]bool{}
	for _, k := range keys {
		expired[k.Name] = k.Expired
	}
	if len(keys) != 2 || !expired["Esquecida"] || expired["Leitor"] {
		t.Fatalf("the list: %+v", keys)
	}
}

func call2(s *Server, key string) *httptest.ResponseRecorder {
	return serve(s, fromTunnel("POST", "/mcp", map[string]string{"Authorization": "Bearer " + key}, "{}"))
}

// Wrong keys slow down their own address only; a valid key from another
// address goes through.
func TestWrongKeysAreThrottledByAddress(t *testing.T) {
	s, st, _ := agentServer(t)
	key, _, _ := CreateKey(context.Background(), st, "x", false, time.Now())
	a := map[string]string{"Authorization": "Bearer " + KeyPrefix + "wrong", "X-Forwarded-For": "198.51.100.7"}
	for i := 0; i < 10; i++ {
		serve(s, fromTunnel("POST", "/mcp", a, ""))
	}
	if rec := serve(s, fromTunnel("POST", "/mcp", a, "")); rec.Code != 401 || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("a throttled address: %d %v", rec.Code, rec.Header())
	}
	b := map[string]string{"Authorization": "Bearer " + key, "X-Forwarded-For": "203.0.113.9"}
	if rec := serve(s, fromTunnel("POST", "/mcp", b, "")); rec.Code != 200 {
		t.Fatalf("a valid key from another address: %d", rec.Code)
	}
	// A valid key from the slowed-down address itself still goes through:
	// a stale key behind the same network does not lock the good one out.
	same := map[string]string{"Authorization": "Bearer " + key, "X-Forwarded-For": "198.51.100.7"}
	if rec := serve(s, fromTunnel("POST", "/mcp", same, "")); rec.Code != 200 {
		t.Fatalf("a valid key from the slowed-down address: %d", rec.Code)
	}
}

// pkce is a verifier and its S256 challenge.
func pkce(seed string) (verifier, challenge string) {
	verifier = strings.Repeat(seed, 64)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

// oauthClient walks the OAuth flow against s, as an MCP client does.
type oauthClient struct {
	t        *testing.T
	s        *Server
	id       string
	redirect string
}

func registerClient(t *testing.T, s *Server, redirect string) oauthClient {
	t.Helper()
	rec := serve(s, fromTunnel("POST", "/oauth/register", map[string]string{"Content-Type": "application/json"},
		`{"client_name":"Claude","redirect_uris":["`+redirect+`"],"token_endpoint_auth_method":"none","grant_types":["authorization_code","refresh_token"]}`))
	var reg struct {
		ClientID string `json:"client_id"`
	}
	jsonBody(t, rec, &reg)
	if rec.Code != 201 || reg.ClientID == "" {
		t.Fatalf("registration: %d %s", rec.Code, rec.Body)
	}
	return oauthClient{t: t, s: s, id: reg.ClientID, redirect: redirect}
}

func (c oauthClient) authorizeQuery(challenge, redirect string) url.Values {
	return url.Values{"response_type": {"code"}, "client_id": {c.id}, "redirect_uri": {redirect}, "state": {"st4te"},
		"code_challenge": {challenge}, "code_challenge_method": {"S256"}, "resource": {origin + "/mcp"}}
}

// approve posts the consent form and returns the redirect's query.
func (c oauthClient) approve(q url.Values, password, access string) (*httptest.ResponseRecorder, url.Values) {
	form := url.Values{}
	for k, v := range q {
		form[k] = v
	}
	form.Set("password", password)
	form.Set("access", access)
	form.Set("decision", "allow")
	rec := serve(c.s, fromTunnel("POST", "/oauth/authorize", map[string]string{"Origin": origin}, form.Encode()))
	loc, _ := url.Parse(rec.Header().Get("Location"))
	return rec, loc.Query()
}

func (c oauthClient) token(form url.Values) (*httptest.ResponseRecorder, map[string]any) {
	rec := serve(c.s, fromTunnel("POST", "/oauth/token", nil, form.Encode()))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestTheOAuthFlow(t *testing.T) {
	s, st, path := agentServer(t)
	c := registerClient(t, s, "https://claude.ai/api/mcp/auth_callback")
	verifier, challenge := pkce("a")
	q := c.authorizeQuery(challenge, c.redirect)

	// The consent page: the client's name and where it goes back to.
	page := serve(s, fromTunnel("GET", "/oauth/authorize?"+q.Encode(), nil, ""))
	if page.Code != 200 || !strings.Contains(page.Body.String(), "Claude") || !strings.Contains(page.Body.String(), "claude.ai") ||
		!strings.Contains(page.Body.String(), `type="password"`) {
		t.Fatalf("the consent page: %d", page.Code)
	}
	// A consent from another site, or a wrong password, gives no code.
	form := url.Values{}
	for k, v := range q {
		form[k] = v
	}
	form.Set("password", testPassword)
	form.Set("decision", "allow")
	if rec := serve(s, fromTunnel("POST", "/oauth/authorize", map[string]string{"Origin": "https://evil.example"}, form.Encode())); rec.Code != 403 {
		t.Fatalf("a consent from another site: %d", rec.Code)
	}
	if rec, back := c.approve(q, "wrong password!", "full"); rec.Code != 401 || back.Get("code") != "" {
		t.Fatalf("a wrong password: %d %v", rec.Code, back)
	}
	// Refused by the person.
	form.Set("decision", "deny")
	rec := serve(s, fromTunnel("POST", "/oauth/authorize", map[string]string{"Origin": origin}, form.Encode()))
	if loc, _ := url.Parse(rec.Header().Get("Location")); loc.Query().Get("error") != "access_denied" || loc.Query().Get("iss") != origin {
		t.Fatalf("a refusal: %q", rec.Header().Get("Location"))
	}

	rec, back := c.approve(q, testPassword, "read")
	if rec.Code != 303 || back.Get("code") == "" || back.Get("state") != "st4te" || back.Get("iss") != origin ||
		!strings.HasPrefix(rec.Header().Get("Location"), c.redirect+"?") {
		t.Fatalf("approval: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	code := back.Get("code")
	exchange := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {c.id},
		"redirect_uri": {c.redirect}, "code_verifier": {verifier}, "resource": {origin + "/mcp"}}
	// A wrong verifier spends nothing for the attacker but the code.
	bad := url.Values{}
	for k, v := range exchange {
		bad[k] = v
	}
	_, wrongVerifier := pkce("b")
	bad.Set("code_verifier", wrongVerifier)
	if rec, out := c.token(bad); rec.Code != 400 || out["error"] != "invalid_grant" {
		t.Fatalf("a wrong verifier: %d %v", rec.Code, out)
	}
	// That code is gone; a new consent gives a new one.
	_, back = c.approve(q, testPassword, "read")
	exchange.Set("code", back.Get("code"))
	rec, tok := c.token(exchange)
	if rec.Code != 200 || tok["token_type"] != "Bearer" || tok["expires_in"] != float64(3600) || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("the exchange: %d %v", rec.Code, tok)
	}
	access, refresh := tok["access_token"].(string), tok["refresh_token"].(string)
	mcp := func(token string) *httptest.ResponseRecorder {
		return serve(s, fromTunnel("POST", "/mcp", map[string]string{"Authorization": "Bearer " + token}, "{}"))
	}
	if rec := mcp(access); rec.Code != 200 || rec.Body.String() != "oauth/Claude/read" {
		t.Fatalf("the access token: %d %s", rec.Code, rec.Body)
	}
	// The code used again ends the connection it made.
	if rec, _ := c.token(exchange); rec.Code != 400 {
		t.Fatalf("a code used twice: %d", rec.Code)
	}
	if rec := mcp(access); rec.Code != 401 {
		t.Fatalf("the connection survived a reused code: %d", rec.Code)
	}

	// A fresh connection, refreshed: the refresh token rotates, and the old
	// one used again revokes the connection.
	_, back = c.approve(q, testPassword, "full")
	exchange.Set("code", back.Get("code"))
	_, tok = c.token(exchange)
	access, refresh = tok["access_token"].(string), tok["refresh_token"].(string)
	refreshForm := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {c.id}, "resource": {origin + "/mcp"}}
	rec, tok2 := c.token(refreshForm)
	if rec.Code != 200 || tok2["refresh_token"] == refresh || tok2["access_token"] == access {
		t.Fatalf("a refresh: %d %v", rec.Code, tok2)
	}
	if rec := mcp(tok2["access_token"].(string)); rec.Code != 200 {
		t.Fatalf("the refreshed token: %d", rec.Code)
	}
	if rec, out := c.token(refreshForm); rec.Code != 400 || out["error"] != "invalid_grant" {
		t.Fatalf("a refresh token used twice: %d %v", rec.Code, out)
	}
	for _, tk := range []string{access, tok2["access_token"].(string)} {
		if rec := mcp(tk); rec.Code != 401 {
			t.Fatalf("the connection survived a reused refresh token: %d", rec.Code)
		}
	}
	newRefresh := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tok2["refresh_token"].(string)}, "client_id": {c.id}}
	if rec, _ := c.token(newRefresh); rec.Code != 400 {
		t.Fatalf("the newest refresh token of a revoked connection: %d", rec.Code)
	}

	// A new panel password ends every login.
	_, back = c.approve(q, testPassword, "full")
	exchange.Set("code", back.Get("code"))
	_, tok = c.token(exchange)
	if rec := mcp(tok["access_token"].(string)); rec.Code != 200 {
		t.Fatalf("before the new password: %d", rec.Code)
	}
	if err := os.WriteFile(path, []byte(hashOf(t, "a brand new password", MinCost)), 0o600); err != nil {
		t.Fatal(err)
	}
	if rec := mcp(tok["access_token"].(string)); rec.Code != 401 {
		t.Fatalf("a login survived a new password: %d", rec.Code)
	}
	if rec, _ := c.token(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tok["refresh_token"].(string)}, "client_id": {c.id}}); rec.Code != 400 {
		t.Fatalf("a refresh after a new password: %d", rec.Code)
	}
	if conns, _ := st.Connections(context.Background()); len(conns) != 0 {
		t.Fatalf("live connections after a new password: %d", len(conns))
	}
}

func TestAuthorizationRequestsAreChecked(t *testing.T) {
	s, _, _ := agentServer(t)
	c := registerClient(t, s, "https://claude.ai/api/mcp/auth_callback")
	_, challenge := pkce("a")
	get := func(q url.Values) *httptest.ResponseRecorder {
		return serve(s, fromTunnel("GET", "/oauth/authorize?"+q.Encode(), nil, ""))
	}
	// A redirect not registered is shown here, never followed.
	q := c.authorizeQuery(challenge, "https://evil.example/cb")
	if rec := get(q); rec.Code != 400 || rec.Header().Get("Location") != "" {
		t.Errorf("an unknown redirect: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	q = c.authorizeQuery(challenge, c.redirect)
	q.Set("client_id", "wamcp_client_unknown")
	if rec := get(q); rec.Code != 400 || rec.Header().Get("Location") != "" {
		t.Errorf("an unknown client: %d", rec.Code)
	}
	// The rest is shown here too: anyone may register a client, so nothing
	// goes to its address before the person decides.
	for name, change := range map[string][2]string{
		"no PKCE":          {"code_challenge_method", "plain"},
		"another resource": {"resource", "https://other.example/mcp"},
		"another response": {"response_type", "token"},
	} {
		q := c.authorizeQuery(challenge, c.redirect)
		q.Set(change[0], change[1])
		if rec := get(q); rec.Code != 400 || rec.Header().Get("Location") != "" {
			t.Errorf("%s: %d %q", name, rec.Code, rec.Header().Get("Location"))
		}
	}
	// Registration: https redirects, or http on loopback only.
	for _, r := range []string{"http://evil.example/cb", "javascript:alert(1)", "https://x.example/cb#frag"} {
		rec := serve(s, fromTunnel("POST", "/oauth/register", nil, `{"client_name":"x","redirect_uris":["`+r+`"]}`))
		if rec.Code != 400 {
			t.Errorf("registering %s: %d", r, rec.Code)
		}
	}
}

// A program on the person's computer comes back to any port of a loopback
// address it registered (RFC 8252), and only there.
func TestLoopbackRedirects(t *testing.T) {
	reg := []string{"http://localhost/callback", "http://127.0.0.1/callback", "https://app.example/cb"}
	for raw, want := range map[string]bool{
		"http://localhost:3118/callback": true, "http://127.0.0.1:50000/callback": true, "http://localhost/callback": true,
		"http://localhost:3118/other": false, "http://[::1]:3118/callback": false, "https://localhost:3118/callback": false,
		"https://app.example/cb": true, "https://app.example:444/cb": false, "https://app.example/cb?x=1": false,
	} {
		if got := redirectAllowed(reg, raw); got != want {
			t.Errorf("%s: %v, want %v", raw, got, want)
		}
	}
}

// A client known by its metadata document: fetched, checked, cached.
func TestClientMetadataDocuments(t *testing.T) {
	s, _, _ := agentServer(t)
	docs := 0
	doc := `{"client_id":"https://client.example/oauth/client.json","client_name":"ChatGPT","redirect_uris":["https://chatgpt.example/cb"],"token_endpoint_auth_method":"none"}`
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		docs++
		switch r.URL.Path {
		case "/oauth/client.json":
			_, _ = w.Write([]byte(doc))
		case "/oauth/liar.json":
			_, _ = w.Write([]byte(strings.Replace(doc, "client.json", "other.json", 1)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	// The test's document server is on loopback, which the real client
	// refuses: this one dials it for the example name.
	addr := srv.Listener.Addr().String()
	s.documents.client = &http.Client{Timeout: time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		}}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	c := oauthClient{t: t, s: s, id: "https://client.example/oauth/client.json", redirect: "https://chatgpt.example/cb"}
	verifier, challenge := pkce("c")
	page := serve(s, fromTunnel("GET", "/oauth/authorize?"+c.authorizeQuery(challenge, c.redirect).Encode(), nil, ""))
	if page.Code != 200 || !strings.Contains(page.Body.String(), "ChatGPT") || !strings.Contains(page.Body.String(), "client.example") {
		t.Fatalf("the consent page: %d %s", page.Code, page.Body)
	}
	_, back := c.approve(c.authorizeQuery(challenge, c.redirect), testPassword, "full")
	_, tok := c.token(url.Values{"grant_type": {"authorization_code"}, "code": {back.Get("code")}, "client_id": {c.id},
		"redirect_uri": {c.redirect}, "code_verifier": {verifier}})
	if tok["access_token"] == nil {
		t.Fatalf("no token: %v", tok)
	}
	if docs != 1 {
		t.Errorf("the document was fetched %d times, want once (cached)", docs)
	}
	// A document whose client_id is not its own address, one missing, and
	// one not https or on an internal address are refused, before any
	// redirect.
	for _, id := range []string{"https://client.example/oauth/liar.json", "https://client.example/oauth/missing.json",
		"https://127.0.0.1/client.json", "https://10.0.0.1/client.json", "https://client.example/"} {
		q := c.authorizeQuery(challenge, c.redirect)
		q.Set("client_id", id)
		if rec := serve(s, fromTunnel("GET", "/oauth/authorize?"+q.Encode(), nil, "")); rec.Code != 400 || rec.Header().Get("Location") != "" {
			t.Errorf("%s: %d %q", id, rec.Code, rec.Header().Get("Location"))
		}
	}
}

// The real fetch never reaches this computer, even by name.
func TestDocumentsAreNotFetchedFromInside(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the document server on loopback was reached")
	}))
	defer srv.Close()
	d := newDocuments()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	if _, err := d.get(context.Background(), "https://localhost:"+port+"/client.json", "198.51.100.7"); err == nil {
		t.Fatal("a document on loopback was accepted")
	}
}

func TestRegistrationIsLimited(t *testing.T) {
	s, _, _ := agentServer(t)
	codes := map[int]int{}
	for i := 0; i < registerPerHour+5; i++ {
		rec := serve(s, fromTunnel("POST", "/oauth/register", nil, `{"client_name":"x","redirect_uris":["https://x.example/cb"]}`))
		codes[rec.Code]++
	}
	if codes[201] != registerPerHour || codes[429] != 5 {
		t.Fatalf("registrations: %v", codes)
	}
	// Another address still registers.
	rec := serve(s, fromTunnel("POST", "/oauth/register", map[string]string{"X-Forwarded-For": "203.0.113.9"}, `{"client_name":"x","redirect_uris":["https://x.example/cb"]}`))
	if rec.Code != 201 {
		t.Fatalf("another address: %d", rec.Code)
	}
}

// The panel's session is never an agent's credential.
func TestTheSessionDoesNotReachTheMCP(t *testing.T) {
	s, _, _ := agentServer(t)
	c := sessionCookie(t, signIn(s, "127.0.0.1:40000", testPassword, nil))
	rec := serve(s, fromTunnel("POST", "/mcp", map[string]string{"Cookie": CookieName + "=" + c.Value}, "{}"))
	if rec.Code != 401 {
		t.Fatalf("/mcp with the panel session: %d %s", rec.Code, rec.Body)
	}
	if rec := serve(s, fromTunnel("GET", "/status", map[string]string{"Cookie": CookieName + "=" + c.Value}, "")); rec.Body.String() != "panel//full" {
		t.Fatalf("the panel with its session: %s", rec.Body)
	}
}

// The documents the real clients publish, as fetched on 2026-11-17, go
// through the checks: Claude (claude.ai, the apps), Claude Code and
// ChatGPT, whose document names private_key_jwt and supports none.
func TestTheRealClientsDocuments(t *testing.T) {
	s, _, _ := agentServer(t)
	byHost := map[string]string{}
	for file, id := range map[string]string{
		"claude.json":      "https://claude.ai/oauth/mcp-oauth-client-metadata",
		"claude-code.json": "https://claude.ai/oauth/claude-code-client-metadata",
		"chatgpt.json":     "https://chatgpt.com/oauth/client.json",
	} {
		raw, err := os.ReadFile(filepath.Join("testdata", "cimd", file))
		if err != nil {
			t.Fatal(err)
		}
		byHost[id] = string(raw)
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(byHost["https://"+r.Host+r.URL.Path]))
	}))
	defer srv.Close()
	addr := srv.Listener.Addr().String()
	s.documents.client = &http.Client{Timeout: time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		}}}
	for id, redirect := range map[string]string{
		"https://claude.ai/oauth/mcp-oauth-client-metadata":   "https://claude.ai/api/mcp/auth_callback",
		"https://claude.ai/oauth/claude-code-client-metadata": "http://localhost:51234/callback",
		"https://chatgpt.com/oauth/client.json":               "https://chatgpt.com/connector_platform_oauth_redirect",
	} {
		c := oauthClient{t: t, s: s, id: id, redirect: redirect}
		verifier, challenge := pkce("d")
		q := c.authorizeQuery(challenge, redirect)
		host, _ := url.Parse(id)
		if page := serve(s, fromTunnel("GET", "/oauth/authorize?"+q.Encode(), nil, "")); page.Code != 200 || !strings.Contains(page.Body.String(), host.Host) {
			t.Errorf("%s: the consent page answered %d: %.300s", id, page.Code, page.Body)
			continue
		}
		_, back := c.approve(q, testPassword, "full")
		_, tok := c.token(url.Values{"grant_type": {"authorization_code"}, "code": {back.Get("code")}, "client_id": {id},
			"redirect_uri": {redirect}, "code_verifier": {verifier}, "resource": {origin + "/mcp"}})
		if tok["access_token"] == nil {
			t.Errorf("%s: no token: %v", id, tok)
		}
	}
}

// What each OAuth route answers anyone, with no credential: the discovery
// documents and the consent page are open, and the rest refuses a request
// that does not make sense, but nothing reaches the panel or the MCP.
func TestTheOAuthRoutesWithoutACredential(t *testing.T) {
	s, _, _ := agentServer(t)
	want := map[string]int{
		"GET " + ProtectedResourcePath:          200,
		"GET " + ProtectedResourcePath + "/mcp": 200,
		"GET " + AuthServerPath:                 200,
		"POST " + RegisterPath:                  400, // an empty body is no client
		"GET " + AuthorizePath:                  400, // no client named: shown, never redirected
		"POST " + AuthorizePath:                 403, // not from the consent page
		"POST " + TokenPath:                     400, // no grant
	}
	if len(want) != len(OAuthRoutes) {
		t.Fatalf("%d routes checked, %d exist", len(want), len(OAuthRoutes))
	}
	for _, route := range OAuthRoutes {
		method, path, _ := strings.Cut(route, " ")
		rec := serve(s, fromTunnel(method, path, nil, ""))
		if rec.Code != want[route] || rec.Header().Get("Location") != "" || len(rec.Result().Cookies()) != 0 {
			t.Errorf("%s: %d %q, want %d", route, rec.Code, rec.Header().Get("Location"), want[route])
		}
	}
}
