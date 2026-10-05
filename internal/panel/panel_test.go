package panel

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSameOrigin(t *testing.T) {
	cases := []struct {
		name    string
		method  string
		headers map[string]string
		want    bool
	}{
		{"page fetch from itself", "POST", map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://127.0.0.1:47821"}, true},
		{"form post from itself", "POST", map[string]string{"Origin": "http://127.0.0.1:47821"}, true},
		{"another site", "POST", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}, false},
		{"another localhost app", "POST", map[string]string{"Sec-Fetch-Site": "same-site", "Origin": "http://localhost:3000"}, false},
		{"a post that names no origin", "POST", nil, false},
		{"typed into the address bar", "GET", map[string]string{"Sec-Fetch-Site": "none"}, true},
		{"an api read from another site", "GET", map[string]string{"Sec-Fetch-Site": "cross-site"}, false},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, "http://127.0.0.1:47821/api/pair", nil)
		for k, v := range c.headers {
			r.Header.Set(k, v)
		}
		if got := sameOrigin(r); got != c.want {
			t.Errorf("%s: sameOrigin = %v, want %v", c.name, got, c.want)
		}
	}
}

// The app's window reaches the panel in memory: its webview sends neither
// Sec-Fetch-Site nor, on a fetch, Origin, and nothing else can take that path.
func TestInternalRequestsAreTheAppsOwn(t *testing.T) {
	var got bool
	h := Internal(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = sameOrigin(r) }))
	r := httptest.NewRequest("POST", "wails://localhost/api/pair", nil)
	r.Header.Set("Referer", "wails://localhost/")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if !got {
		t.Fatal("a request from the app's own window must be accepted")
	}
	if sameOrigin(r) {
		t.Fatal("the same request over the network, unmarked, must be refused")
	}
}

func TestFormatPhone(t *testing.T) {
	for in, want := range map[string]string{
		"5511987654321": "+55 (11) 98765-4321",
		"551132654321":  "+55 (11) 3265-4321",
		"4915112345678": "+4915112345678",
		"":              "",
	} {
		if got := formatPhone(in); got != want {
			t.Errorf("formatPhone(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTemplatesParse(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("templates do not parse: %v", r)
		}
	}()
	parseTemplates()
}

func TestParseMCPGet(t *testing.T) {
	out := "whatsapp:\n  Scope: User config (available in all your projects)\n  Status: ✔ Connected\n  Type: http\n  URL: https://example.test/mcp\n  Headers:\n    Authorization: Bearer x\n"
	if got := parseMCPGet(out); got != "https://example.test/mcp" {
		t.Fatalf("parseMCPGet = %q", got)
	}
	if got := parseMCPGet("whatsapp:\n  Type: stdio\n  Command: /bin/x\n"); got != "/bin/x" {
		t.Fatalf("parseMCPGet stdio = %q", got)
	}
}
