//go:build integration_server

// The server mode end to end, on this computer, with Docker and no WhatsApp
// account: the image up through compose.yml (the "proxy you already run"
// option, its public listener on this computer's loopback), then what a
// person and an agent do through the public address: sign in, make a key, the
// OAuth login, tool calls, and the data surviving a restart. wacli is the
// real one in the image, not paired: whatsapp_status answers all the same.
//
//	go test -tags integration_server -count=1 -v ./integration/
//
// It builds the image from this checkout unless WMCP_SERVER_IMAGE names one
// already built (repository:tag). Its own build tag keeps it out of the
// suite against the test Mac (-tags integration).
package integration

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

const srvOrigin = "https://wa.example.com"

type srvStack struct {
	t        *testing.T
	dir      string
	base     string // where the test reaches the public listener
	password string
	client   *http.Client
}

func srvRepo(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func srvRun(t *testing.T, dir string, stdin []byte, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

func srvFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// srvUp starts the stack in a folder of its own, as install-docker.sh leaves
// it for a proxy you already run.
func srvUp(t *testing.T) *srvStack {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is not installed")
	}
	repo := srvRepo(t)
	image := os.Getenv("WMCP_SERVER_IMAGE")
	if image == "" {
		image = strings.TrimSpace(srvRun(t, repo, nil, "scripts/image-name.sh")) + ":integration"
		t.Logf("building %s", image)
		srvRun(t, repo, nil, "docker", "build", "-q", "-f", "build/docker/Dockerfile", "--build-arg", "VERSION=integration", "-t", image, ".")
	}
	name, tag, _ := strings.Cut(image, ":")
	dir := t.TempDir()
	compose, err := os.ReadFile(filepath.Join(repo, "compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "compose.yml"), compose, 0o644)
	_ = os.WriteFile(filepath.Join(dir, "compose.override.yml"), []byte(`services:
  whatsapp-mcp:
    ports:
      - 127.0.0.1:${WHATSAPP_MCP_PUBLIC_PORT}:47822
`), 0o644)
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	password := hex.EncodeToString(b)
	hash := strings.TrimSpace(srvRun(t, dir, []byte(password+"\n"), "docker", "run", "--rm", "-i", image, "password", "hash"))
	port := srvFreePort(t)
	// A subnet of its own, so it does not meet another stack's.
	n := 200 + port%50
	project := "wamcp-it-" + strconv.Itoa(port)
	env := fmt.Sprintf("COMPOSE_PROJECT_NAME=%s\nCOMPOSE_PROFILES=\nWHATSAPP_MCP_IMAGE=%s\nWHATSAPP_MCP_VERSION=%s\n"+
		"WHATSAPP_MCP_PUBLIC_URL=%s\nWHATSAPP_MCP_PANEL_PASSWORD_HASH='%s'\nWHATSAPP_MCP_TRUSTED_PROXY=172.30.%d.1\n"+
		"WHATSAPP_MCP_SUBNET=172.30.%d.0/24\nWHATSAPP_MCP_GATEWAY=172.30.%d.1\nWHATSAPP_MCP_PUBLIC_PORT=%d\n",
		project, name, tag, srvOrigin, hash, n, n, n, port)
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &srvStack{t: t, dir: dir, base: "http://127.0.0.1:" + strconv.Itoa(port), password: password,
		client: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	t.Cleanup(func() {
		out, _ := exec.Command("docker", "compose", "--project-directory", dir, "down", "-v").CombinedOutput()
		t.Logf("down: %s", strings.TrimSpace(lastLine(string(out))))
	})
	s.up()
	return s
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

func (s *srvStack) up() {
	s.t.Helper()
	srvRun(s.t, s.dir, nil, "docker", "compose", "up", "-d")
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if strings.TrimSpace(srvRun(s.t, s.dir, nil, "docker", "compose", "ps", "--format", "{{.Health}}", "whatsapp-mcp")) == "healthy" {
			return
		}
		time.Sleep(time.Second)
	}
	s.t.Fatalf("the container never became healthy:\n%s", srvRun(s.t, s.dir, nil, "docker", "compose", "logs", "whatsapp-mcp"))
}

// do sends a request as the proxy in front would forward it.
func (s *srvStack) do(method, path string, body string, header map[string]string) (*http.Response, string) {
	s.t.Helper()
	req, _ := http.NewRequest(method, s.base+path, strings.NewReader(body))
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Host = "wa.example.com"
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func (s *srvStack) tool(credential, name string) map[string]any {
	s.t.Helper()
	resp, body := s.do("POST", "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+name+`","arguments":{}}}`,
		map[string]string{"Authorization": "Bearer " + credential, "Content-Type": "application/json"})
	var out struct {
		Result map[string]any `json:"result"`
	}
	if resp.StatusCode != 200 || json.Unmarshal([]byte(body), &out) != nil || out.Result == nil {
		s.t.Fatalf("%s: %d %.300s", name, resp.StatusCode, body)
	}
	return out.Result
}

func TestServerMode(t *testing.T) {
	s := srvUp(t)
	var cookie, key string

	t.Run("the panel asks for the password", func(t *testing.T) {
		resp, _ := s.do("GET", "/status", "", nil)
		if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/entrar") {
			t.Fatalf("%d %q", resp.StatusCode, resp.Header.Get("Location"))
		}
		form := url.Values{"password": {"wrong password!"}}.Encode()
		if resp, _ := s.do("POST", "/entrar", form, map[string]string{"Origin": srvOrigin, "Content-Type": "application/x-www-form-urlencoded"}); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("a wrong password: %d", resp.StatusCode)
		}
	})

	t.Run("sign-in", func(t *testing.T) {
		form := url.Values{"password": {s.password}, "next": {"/conexoes"}}.Encode()
		resp, _ := s.do("POST", "/entrar", form, map[string]string{"Origin": srvOrigin, "Content-Type": "application/x-www-form-urlencoded"})
		for _, c := range resp.Cookies() {
			if c.Name == "whatsapp_mcp_session" && c.Secure && c.HttpOnly {
				cookie = c.Name + "=" + c.Value
			}
		}
		if resp.StatusCode != http.StatusSeeOther || cookie == "" {
			t.Fatalf("%d, cookie %v", resp.StatusCode, cookie != "")
		}
	})

	t.Run("Configurações shows the version and the addresses", func(t *testing.T) {
		resp, body := s.do("GET", "/configuracoes", "", map[string]string{"Cookie": cookie})
		if resp.StatusCode != 200 {
			t.Fatalf("/configuracoes: %d", resp.StatusCode)
		}
		for _, want := range []string{`id="atualizacoes"`, "docker compose pull", `id="enderecos"`, srvOrigin, "http://whatsapp-mcp:47822"} {
			if !strings.Contains(body, want) {
				t.Errorf("Configurações lacks %q", want)
			}
		}
		for _, unwanted := range []string{"data-update-install", `id="dados"`, "/configuracoes/porta"} {
			if strings.Contains(body, unwanted) {
				t.Errorf("Configurações on a server shows %q", unwanted)
			}
		}
		if resp, body := s.do("GET", "/api/atualizacao", "", map[string]string{"Cookie": cookie}); resp.StatusCode != 200 || !strings.Contains(body, `"state"`) {
			t.Errorf("/api/atualizacao signed in: %d %s", resp.StatusCode, body)
		}
		if resp, _ := s.do("GET", "/api/atualizacao", "", nil); resp.StatusCode == 200 {
			t.Error("/api/atualizacao answers without a session")
		}
	})

	t.Run("a key from the Conexões page", func(t *testing.T) {
		resp, body := s.do("POST", "/conexoes/chaves", url.Values{"name": {"integration"}}.Encode(), map[string]string{
			"Origin": srvOrigin, "Sec-Fetch-Site": "same-origin", "Cookie": cookie, "Content-Type": "application/x-www-form-urlencoded"})
		key = regexp.MustCompile(`wamcp_key_[A-Za-z0-9_-]{43}`).FindString(body)
		if resp.StatusCode != 200 || key == "" {
			t.Fatalf("%d, key %v", resp.StatusCode, key != "")
		}
	})

	t.Run("a tool call with the key", func(t *testing.T) {
		if resp, _ := s.do("POST", "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, map[string]string{"Content-Type": "application/json"}); resp.StatusCode != http.StatusUnauthorized ||
			!strings.Contains(resp.Header.Get("WWW-Authenticate"), "resource_metadata=") {
			t.Fatalf("/mcp without a credential: %d", resp.StatusCode)
		}
		res := s.tool(key, "whatsapp_status")
		text := res["content"].([]any)[0].(map[string]any)["text"].(string)
		if res["isError"] == true || !strings.Contains(text, srvOrigin+"/mcp") {
			t.Fatalf("whatsapp_status: %.300s", text)
		}
	})

	t.Run("the OAuth login and a tool call with its token", func(t *testing.T) {
		const redirect = "https://client.example.com/callback"
		resp, body := s.do("POST", "/oauth/register", `{"client_name":"Integration","redirect_uris":["`+redirect+`"]}`, map[string]string{"Content-Type": "application/json"})
		var reg struct {
			ClientID string `json:"client_id"`
		}
		if resp.StatusCode != http.StatusCreated || json.Unmarshal([]byte(body), &reg) != nil {
			t.Fatalf("registration: %d %s", resp.StatusCode, body)
		}
		verifier := strings.Repeat("v", 64)
		sum := sha256.Sum256([]byte(verifier))
		form := url.Values{"response_type": {"code"}, "client_id": {reg.ClientID}, "redirect_uri": {redirect}, "state": {"s"},
			"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"},
			"resource": {srvOrigin + "/mcp"}}
		// The consent's first step asks only for the password; the right
		// one signs the browser in and comes back to the same request.
		if resp, body := s.do("GET", "/oauth/authorize?"+form.Encode(), "", nil); resp.StatusCode != 200 ||
			!strings.Contains(body, `name="step" value="password"`) || strings.Contains(body, `name="access"`) {
			t.Fatalf("the consent's first step: %d %.300s", resp.StatusCode, body)
		}
		step := url.Values{"step": {"password"}, "password": {s.password}}
		for k, v := range form {
			step[k] = v
		}
		resp, _ = s.do("POST", "/oauth/authorize", step.Encode(), map[string]string{"Origin": srvOrigin, "Content-Type": "application/x-www-form-urlencoded"})
		var session string
		for _, c := range resp.Cookies() {
			if c.Name == "whatsapp_mcp_session" {
				session = c.Name + "=" + c.Value
			}
		}
		if resp.StatusCode != http.StatusSeeOther || session == "" || !strings.HasPrefix(resp.Header.Get("Location"), "/oauth/authorize?") {
			t.Fatalf("the consent's password: %d %q", resp.StatusCode, resp.Header.Get("Location"))
		}
		// The second step, with that session: the choices, and the code.
		if resp, body := s.do("GET", resp.Header.Get("Location"), "", map[string]string{"Cookie": session}); resp.StatusCode != 200 ||
			!strings.Contains(body, `name="access"`) || strings.Contains(body, `type="password"`) {
			t.Fatalf("the consent's second step: %d %.300s", resp.StatusCode, body)
		}
		form.Set("access", "read")
		form.Set("decision", "allow")
		resp, _ = s.do("POST", "/oauth/authorize", form.Encode(), map[string]string{"Origin": srvOrigin, "Content-Type": "application/x-www-form-urlencoded", "Cookie": session})
		loc, _ := url.Parse(resp.Header.Get("Location"))
		if loc == nil || loc.Query().Get("code") == "" || loc.Query().Get("iss") != srvOrigin {
			t.Fatalf("consent: %d %q", resp.StatusCode, resp.Header.Get("Location"))
		}
		ex := url.Values{"grant_type": {"authorization_code"}, "code": {loc.Query().Get("code")}, "client_id": {reg.ClientID},
			"redirect_uri": {redirect}, "code_verifier": {verifier}, "resource": {srvOrigin + "/mcp"}}
		resp, body = s.do("POST", "/oauth/token", ex.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
		var tok struct {
			AccessToken string `json:"access_token"`
		}
		if resp.StatusCode != 200 || json.Unmarshal([]byte(body), &tok) != nil || tok.AccessToken == "" {
			t.Fatalf("token: %d %s", resp.StatusCode, body)
		}
		if res := s.tool(tok.AccessToken, "whatsapp_status"); res["isError"] == true {
			t.Fatalf("whatsapp_status: %v", res)
		}
		// Read-only: a send is refused.
		res := s.tool(tok.AccessToken, "send_text_message")
		if b, _ := json.Marshal(res); res["isError"] != true || !strings.Contains(string(b), "not allowed for this connection") {
			t.Fatalf("a send with a read-only login: %s", b)
		}
	})

	t.Run("the data survives a restart", func(t *testing.T) {
		srvRun(t, s.dir, nil, "docker", "compose", "down")
		s.up()
		if res := s.tool(key, "whatsapp_status"); res["isError"] == true {
			t.Fatalf("the key after down and up: %v", res)
		}
	})
}
