//go:build integration

package integration

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// delivery is one POST the app made to the receiver.
type delivery struct {
	path   string
	event  string
	body   any
	signed bool // the signature matched the secret of that path
}

// receiver stands in for a user's script: it takes the app's webhook
// deliveries, through the reverse tunnel from the test Mac.
type receiver struct {
	mu      sync.Mutex
	secrets map[string]string
	got     []delivery
}

func startReceiver(t *testing.T, e env) *receiver {
	r := &receiver{secrets: map[string]string{}}
	if e.hookAddr == "" || e.hookURL == "" {
		return r
	}
	ln, err := net.Listen("tcp", e.hookAddr)
	if err != nil {
		t.Fatalf("webhook receiver: %v", err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		var body any
		_ = json.Unmarshal(raw, &body)
		r.mu.Lock()
		secret := r.secrets[req.URL.Path]
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(raw)
		signed := secret != "" && hmac.Equal([]byte(req.Header.Get("X-WhatsApp-MCP-Signature")), []byte("sha256="+hex.EncodeToString(mac.Sum(nil))))
		r.got = append(r.got, delivery{path: req.URL.Path, event: req.Header.Get("X-WhatsApp-MCP-Event"), body: body, signed: signed})
		r.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return r
}

// wait returns the first delivery to path that match accepts. A delivery
// that never comes is reported and the test goes on, so one missing event
// does not hide what the steps after it would find.
func (r *receiver) wait(t *testing.T, path string, within time.Duration, what string, match func(delivery) bool) (delivery, bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		r.mu.Lock()
		for _, d := range r.got {
			if d.path == path && match(d) {
				r.mu.Unlock()
				if !d.signed {
					t.Errorf("%s: the delivery's signature does not match the secret", what)
				}
				return d, true
			}
		}
		r.mu.Unlock()
		if time.Now().After(deadline) {
			t.Errorf("timed out after %s waiting for %s", within, what)
			return delivery{}, false
		}
		time.Sleep(time.Second)
	}
}

// hook creates a webhook delivering to path and removes it when the test ends.
func (a *app) hook(t *testing.T, r *receiver, path string, settings obj) string {
	t.Helper()
	if a.e.hookURL == "" {
		t.Skip("no webhook tunnel (WMCP_HOOK_URL)")
	}
	settings["url"] = a.e.hookURL + path
	res := a.json(http.MethodPost, "/api/webhooks", settings, 200)
	id, secret := str(res, "webhook.id"), str(res, "secret")
	if id == "" || secret == "" {
		t.Fatalf("create webhook = %s", clipAny(res))
	}
	r.mu.Lock()
	r.secrets[path] = secret
	r.mu.Unlock()
	t.Cleanup(func() { a.try(http.MethodDelete, "/api/webhooks/"+id, nil) })
	return id
}

func testWebhooksAPI(t *testing.T, a *app, r *receiver) {
	list0 := a.json(http.MethodGet, "/api/webhooks", nil, 200)
	if get(list0, "available") != true {
		t.Fatalf("webhooks not available: %s", clipAny(list0))
	}
	for _, ev := range []string{"message", "reaction", "receipt", "online"} {
		if !strings.Contains(mustJSON(get(list0, "events")), `"`+ev+`"`) {
			t.Errorf("event %s not offered", ev)
		}
	}

	id := a.hook(t, r, "/api", obj{"events": []string{"message"}})
	one := a.json(http.MethodGet, "/api/webhooks/"+id, nil, 200)
	if str(one, "url") == "" && str(one, "webhook.url") == "" {
		t.Errorf("get webhook = %s", clipAny(one))
	}
	a.json(http.MethodPatch, "/api/webhooks/"+id, obj{"events": []string{"message", "reaction"}, "include_own": true}, 200)
	patched := a.json(http.MethodGet, "/api/webhooks/"+id, nil, 200)
	if !strings.Contains(mustJSON(patched), `"reaction"`) {
		t.Errorf("patch did not take: %s", clipAny(patched))
	}

	test := a.json(http.MethodPost, "/api/webhooks/"+id+"/test", obj{}, 200)
	d, ok := r.wait(t, "/api", 30*time.Second, "the test delivery", func(d delivery) bool { return get(d.body, "test") == true })
	if ok && d.event == "" {
		t.Errorf("test delivery without X-WhatsApp-MCP-Event; test answered %s", clipAny(test))
	}

	if code, out := a.do(http.MethodPost, "/api/webhooks", obj{"url": "ftp://nope"}); code < 400 {
		t.Errorf("an ftp url was accepted: %d %s", code, clip(out))
	}
	if code, _ := a.do(http.MethodDelete, "/api/webhooks/"+id, nil); code >= 300 {
		t.Errorf("delete webhook = %d", code)
	}
	// The API answers what the person can fix with 409, not found included.
	if code, _ := a.do(http.MethodGet, "/api/webhooks/"+id, nil); code < 400 || code >= 500 {
		t.Errorf("deleted webhook answers %d, want a 4xx", code)
	}
}
