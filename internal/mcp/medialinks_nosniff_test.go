package mcp

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A media link answers with the type WhatsApp gave and forbids sniffing.
func TestMediaLinksForbidSniffing(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a.bin")
	_ = os.WriteFile(file, []byte("<html><script>x</script>"), 0o600)
	l := newMediaLinks()
	token := l.Add(file, "application/octet-stream", time.Minute)
	rec := httptest.NewRecorder()
	l.ServeHTTP(rec, httptest.NewRequest("GET", "/media/"+token, nil))
	if rec.Code != 200 || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
}
