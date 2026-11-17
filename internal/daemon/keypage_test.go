package daemon

import (
	"net/url"
	"testing"
)

// The key-created page lets its own forms post with their origin: under
// no-referrer a browser sends Origin: null, which the panel refuses.
func TestTheKeyPageKeepsItsOrigin(t *testing.T) {
	d := startServerMode(t, true)
	h := d.PublicHandler()
	cookie := signedIn(t, h)
	rec := serveAgent(h, outsideRequest("POST", "/conexoes/chaves", url.Values{"name": {"x"}}.Encode(),
		map[string]string{"Cookie": cookie, "Origin": publicURL, "Sec-Fetch-Site": "same-origin", "Content-Type": "application/x-www-form-urlencoded"}))
	if rec.Code != 200 || rec.Header().Get("Referrer-Policy") != "same-origin" {
		t.Fatalf("%d Referrer-Policy %q", rec.Code, rec.Header().Get("Referrer-Policy"))
	}
}
