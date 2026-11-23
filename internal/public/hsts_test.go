package public

import "testing"

// Over an https address the browser is told to keep to https; an http one,
// where that would lock the panel out, says nothing.
func TestHSTSOnlyOverHTTPS(t *testing.T) {
	s, _ := testServer(t, "https://wa.example.com", "")
	rec := serve(s, request("GET", LoginPath, "127.0.0.1:40000", map[string]string{"X-Forwarded-Proto": "https"}, ""))
	if got := rec.Header().Get("Strict-Transport-Security"); got != "max-age=86400" {
		t.Fatalf("over https: %q", got)
	}
	s, _ = testServer(t, "http://192.0.2.10:47822", "")
	rec = serve(s, request("GET", LoginPath, "192.0.2.50:1000", nil, ""))
	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("over http: %q", got)
	}
}

// An http address on this computer's own name is a test, not a warning.
func TestWhichAddressesAreUnencrypted(t *testing.T) {
	for origin, want := range map[string]bool{
		"https://wa.example.com": false, "http://wa.example.com": true, "http://192.0.2.10:47822": true,
		"http://localhost:47890": false, "http://127.0.0.1:47822": false, "http://[::1]:47822": false,
	} {
		if got := Unencrypted(origin); got != want {
			t.Errorf("%s: %v, want %v", origin, got, want)
		}
	}
}
