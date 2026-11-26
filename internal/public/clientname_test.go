package public

import (
	"context"
	"net/url"
	"testing"
)

// A registered app goes by the name it gives; one that gives none, or only
// says it speaks MCP, by the host it goes back to, or "Conector" for a
// program on the person's computer. Never "mcp".
func TestARegisteredAppsName(t *testing.T) {
	s, _, _ := agentServer(t)
	for _, tc := range []struct{ name, redirect, want string }{
		{"Orca", "http://127.0.0.1:33418/callback", "Orca"},
		{"mcp", "http://127.0.0.1:33418/callback", "Conector"},
		{"MCP Client", "http://localhost:9000/cb", "Conector"},
		{"", "https://app.example/cb", "app.example"},
		{"mcp", "https://app.example/cb", "app.example"},
		{"  ", "https://bücher.example/cb", "xn--bcher-kva.example"},
	} {
		rec := serve(s, fromTunnel("POST", "/oauth/register", map[string]string{"Content-Type": "application/json"},
			`{"client_name":"`+tc.name+`","redirect_uris":["`+tc.redirect+`"],"token_endpoint_auth_method":"none"}`))
		var reg struct {
			ClientName string `json:"client_name"`
		}
		jsonBody(t, rec, &reg)
		if reg.ClientName != tc.want {
			t.Errorf("client_name %q going back to %s: named %q, want %q", tc.name, tc.redirect, reg.ClientName, tc.want)
		}
	}
}

// The connection a sign-in makes carries the name the consent page showed.
func TestTheConnectionIsNamedAsTheConsentPage(t *testing.T) {
	s, st, _ := agentServer(t)
	rec := serve(s, fromTunnel("POST", "/oauth/register", map[string]string{"Content-Type": "application/json"},
		`{"client_name":"Orca","redirect_uris":["http://127.0.0.1:33418/callback"],"token_endpoint_auth_method":"none"}`))
	var reg struct {
		ClientID string `json:"client_id"`
	}
	jsonBody(t, rec, &reg)
	c := oauthClient{t: t, s: s, id: reg.ClientID, redirect: "http://127.0.0.1:33418/callback"}
	verifier, challenge := pkce("o")
	_, back := c.approve(c.authorizeQuery(challenge, c.redirect), testPassword, "full")
	if r, out := c.token(url.Values{"grant_type": {"authorization_code"}, "code": {back.Get("code")}, "client_id": {c.id},
		"redirect_uri": {c.redirect}, "code_verifier": {verifier}, "resource": {origin + "/mcp"}}); r.Code != 200 {
		t.Fatalf("token: %d %v", r.Code, out)
	}
	conns, err := st.Connections(context.Background())
	if err != nil || len(conns) != 1 || conns[0].ClientName != "Orca" {
		t.Fatalf("connections: %+v %v", conns, err)
	}
}
