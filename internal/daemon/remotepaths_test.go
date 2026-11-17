package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/public"
)

// An agent outside never reads the server's folders: media_stats, the
// purge_media preview and whatsapp_status name them by a short word, while
// a program on the server gets them as before.
func TestNoServerPathReachesAnAgentOutside(t *testing.T) {
	d := startServerMode(t, true)
	seedStore(t, d.cfg.StoreDir)
	media := filepath.Join(d.cfg.DataDir, "media")
	_ = os.MkdirAll(filepath.Join(media, "5511900000001_s.whatsapp.net"), 0o700)
	_ = os.WriteFile(filepath.Join(media, "5511900000001_s.whatsapp.net", "P1.png"), []byte("png"), 0o600)
	h := d.PublicHandler()
	key, _, _ := public.CreateKey(context.Background(), d.primary.st, "Outside", false, time.Now())
	local := func(body string) string {
		r := outsideRequest("POST", "/mcp", body, map[string]string{"X-Forwarded-For": "", "X-Forwarded-Proto": ""})
		r.Host = "127.0.0.1"
		return serveAgent(d.HTTPHandler(), r).Body.String()
	}
	for _, call := range []string{
		`{"name":"media_stats","arguments":{}}`,
		`{"name":"purge_media","arguments":{}}`,
		`{"name":"whatsapp_status","arguments":{}}`,
	} {
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":` + call + `}`
		outside := serveAgent(h, outsideRequest("POST", "/mcp", body, map[string]string{"Authorization": "Bearer " + key})).Body.String()
		for _, dir := range []string{d.cfg.DataDir, d.cfg.StoreDir, filepath.Dir(d.cfg.DataDir)} {
			if strings.Contains(outside, dir) {
				t.Errorf("%s, for an agent outside, names %s: %s", call, dir, outside)
			}
		}
		if strings.Contains(call, "media") && !strings.Contains(local(body), media) {
			t.Errorf("%s, on the server itself, lost its path: %s", call, local(body))
		}
	}
	purge := serveAgent(h, outsideRequest("POST", "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"purge_media","arguments":{}}}`,
		map[string]string{"Authorization": "Bearer " + key})).Body.String()
	if !strings.Contains(purge, `media/5511900000001_s.whatsapp.net/P1.png`) {
		t.Errorf("the purge preview, for an agent outside, should name the file inside media: %s", purge)
	}
}
