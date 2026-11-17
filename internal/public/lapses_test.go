package public

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// Registrations nobody signed in with never keep the table full: the
// oldest of them makes room. Only clients someone signed in with count
// toward the cap.
func TestRegistrationsCannotKeepTheCapFull(t *testing.T) {
	s, st, _ := agentServer(t)
	ctx := context.Background()
	old := time.Now().Add(-30 * time.Minute)
	var ids []string
	for i := 0; i < maxClients; i++ {
		c := registeredClient("Filler", []string{"https://filler.example/cb"}, old.Add(time.Duration(i)*time.Second))
		c.From = fmt.Sprintf("198.51.100.%d", i%250)
		if err := st.AddOAuthClient(ctx, c); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID)
		if i == 0 {
			defer func(id string) {
				if _, err := st.OAuthClient(ctx, id); err == nil {
					t.Error("the oldest unused registration was kept")
				}
			}(c.ID)
		}
	}
	c := registerClient(t, s, "https://claude.ai/api/mcp/auth_callback")
	if n, _ := st.OAuthClientCount(ctx); n > maxClients || c.id == "" {
		t.Fatalf("%d clients after a registration on a full table", n)
	}
	// Full of clients someone signed in with: now the cap holds.
	for _, id := range append(ids, c.id) {
		_ = st.TouchOAuthClient(ctx, id, time.Now())
	}
	rec := serve(s, fromTunnel("POST", "/oauth/register", map[string]string{"Content-Type": "application/json"},
		`{"client_name":"Late","redirect_uris":["https://late.example/cb"],"token_endpoint_auth_method":"none"}`))
	if rec.Code != 503 {
		t.Fatalf("a registration past the cap of used clients: %d", rec.Code)
	}
}

// A client's approval to come back without the password lapses when its
// connection is revoked in Conexões, when the password changes, and when
// the client is pruned.
func TestApprovalsLapse(t *testing.T) {
	s, st, path := agentServer(t)
	ctx := context.Background()
	c := registerClient(t, s, "https://claude.ai/api/mcp/auth_callback")
	verifier, challenge := pkce("l")
	q := c.authorizeQuery(challenge, c.redirect)
	session := func(password string) string {
		return CookieName + "=" + sessionCookie(t, signIn(s, "127.0.0.1:40000", password, nil)).Value
	}
	post := func(cookie, password string) *httptest.ResponseRecorder {
		form := url.Values{"decision": {"allow"}, "access": {"full"}}
		for k, v := range q {
			form[k] = v
		}
		if password != "" {
			form.Set("password", password)
		}
		return serve(s, fromTunnel("POST", "/oauth/authorize", map[string]string{"Origin": origin, "Cookie": cookie}, form.Encode()))
	}
	gaveCode := func(rec *httptest.ResponseRecorder) bool {
		return strings.Contains(rec.Header().Get("Location"), "code=")
	}
	cookie := session(testPassword)
	rec := post(cookie, testPassword)
	if !gaveCode(rec) || !gaveCode(post(cookie, "")) {
		t.Fatal("an approval, then the session alone")
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	if r, out := c.token(url.Values{"grant_type": {"authorization_code"}, "code": {loc.Query().Get("code")}, "client_id": {c.id},
		"redirect_uri": {c.redirect}, "code_verifier": {verifier}, "resource": {origin + "/mcp"}}); r.Code != 200 {
		t.Fatalf("the exchange: %d %v", r.Code, out)
	}
	conns, _ := st.Connections(ctx)
	if err := st.RevokeConnection(ctx, conns[0].ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if gaveCode(post(cookie, "")) {
		t.Fatal("a client whose connection was revoked came back with the session alone")
	}

	// Approved again; then a new password, and a session with it.
	if !gaveCode(post(cookie, testPassword)) {
		t.Fatal("approving again")
	}
	const newPassword = "another long password for the panel"
	if err := os.WriteFile(path, []byte(hashOf(t, newPassword, MinCost)), 0o600); err != nil {
		t.Fatal(err)
	}
	if gaveCode(post(session(newPassword), "")) {
		t.Fatal("an approval given with the old password let the client back in with the session alone")
	}

	// Pruning a client drops its approvals.
	unused := registeredClient("Unused", []string{"https://unused.example/cb"}, time.Now().Add(-2*time.Hour))
	_ = st.AddOAuthClient(ctx, unused)
	_ = st.Approve(ctx, unused.ID, "unused.example", "fp", time.Now())
	if _, err := st.PruneOAuthClients(ctx, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if ok, _ := st.Approved(ctx, unused.ID, "unused.example", "fp"); ok {
		t.Fatal("a pruned client's approval was kept")
	}
}
