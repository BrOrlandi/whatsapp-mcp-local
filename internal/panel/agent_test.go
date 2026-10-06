package panel

import (
	"net/http/httptest"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/wacli"
)

// A program on this computer may act through the API; a browser page from
// elsewhere still may not, and a form cannot pass for a program.
func TestFromAgent(t *testing.T) {
	cases := []struct {
		name    string
		token   string
		headers map[string]string
		want    bool
	}{
		{"curl with JSON", "", map[string]string{"Content-Type": "application/json"}, true},
		{"curl with JSON and a charset", "", map[string]string{"Content-Type": "application/json; charset=utf-8"}, true},
		{"a form from an old browser", "", map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, false},
		{"no body type", "", nil, false},
		{"a page from another site", "", map[string]string{"Content-Type": "application/json", "Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"}, false},
		{"token required, none sent", "s3cret", map[string]string{"Content-Type": "application/json"}, false},
		{"token required and sent", "s3cret", map[string]string{"Content-Type": "application/json", "Authorization": "Bearer s3cret"}, true},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "http://127.0.0.1:47821/api/pair", nil)
		for k, v := range c.headers {
			r.Header.Set(k, v)
		}
		if got := (&Panel{Token: c.token}).fromAgent(r); got != c.want {
			t.Errorf("%s: fromAgent = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestNextStep(t *testing.T) {
	paired := wacli.Account{Authenticated: true}
	for _, c := range []struct {
		name string
		s    snapshot
		want string
	}{
		{"fresh install", snapshot{Sync: wacli.Status{State: "not_paired"}, Pairing: wacli.Pairing{State: "idle"}}, "pair"},
		{"pairing starting", snapshot{Sync: wacli.Status{State: "not_paired"}, Pairing: wacli.Pairing{State: "starting"}}, "wait"},
		{"QR on screen", snapshot{Sync: wacli.Status{State: "not_paired"}, Pairing: wacli.Pairing{State: "qr"}}, "scan"},
		{"code to type", snapshot{Sync: wacli.Status{State: "not_paired"}, Pairing: wacli.Pairing{State: "code"}}, "scan"},
		{"history arriving", snapshot{Account: paired, Sync: wacli.Status{State: "starting"}, Pairing: wacli.Pairing{State: "syncing"}}, "syncing"},
		{"opening", snapshot{Account: paired, Sync: wacli.Status{State: "starting"}, Pairing: wacli.Pairing{State: "idle"}}, "wait"},
		{"sending not ready yet", snapshot{Account: paired, Sync: wacli.Status{State: "connected"}, Pairing: wacli.Pairing{State: "idle"}}, "wait"},
		{"ready", snapshot{Account: paired, Sync: wacli.Status{State: "connected", Delegate: true}, Pairing: wacli.Pairing{State: "idle"}}, "ready"},
		{"logged out by the phone", snapshot{Sync: wacli.Status{State: "logged_out"}, Pairing: wacli.Pairing{State: "idle"}}, "pair"},
		{"reconnecting", snapshot{Account: paired, Sync: wacli.Status{State: "reconnecting"}, Pairing: wacli.Pairing{State: "idle"}}, "wait"},
		{"stopped", snapshot{Account: paired, Sync: wacli.Status{State: "stopped"}, Pairing: wacli.Pairing{State: "idle"}}, "error"},
	} {
		if got := nextStep(c.s); got != c.want {
			t.Errorf("%s: next = %q, want %q", c.name, got, c.want)
		}
	}
}
