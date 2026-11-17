package daemon

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/public"
)

// From another computer, send_media_message takes an https URL only, as its
// description says, drafts included; on the server itself http still works.
func TestAnAgentOutsideSendsMediaByHTTPSOnly(t *testing.T) {
	d := startServerMode(t, true)
	seedStore(t, d.cfg.StoreDir)
	h := d.PublicHandler()
	key, _, _ := public.CreateKey(context.Background(), d.primary.st, "Outside", false, time.Now())
	for _, dry := range []string{"true", "false"} {
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"send_media_message","arguments":{"to":"5511900000001@s.whatsapp.net","type":"image","url":"http://example.com/a.png","dry_run":` + dry + `}}}`
		outside := serveAgent(h, outsideRequest("POST", "/mcp", body, map[string]string{"Authorization": "Bearer " + key})).Body.String()
		if !strings.Contains(outside, "http is not accepted") {
			t.Errorf("an http URL from outside (dry_run %s): %s", dry, outside)
		}
	}
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"send_media_message","arguments":{"to":"5511900000001@s.whatsapp.net","type":"image","url":"http://example.com/a.png","dry_run":true}}}`
	r := outsideRequest("POST", "/mcp", body, map[string]string{"X-Forwarded-For": "", "X-Forwarded-Proto": ""})
	r.Host = "127.0.0.1"
	if local := serveAgent(d.HTTPHandler(), r).Body.String(); strings.Contains(local, "http is not accepted") || !strings.Contains(local, "dry_run") {
		t.Errorf("an http URL on the server itself: %s", local)
	}
}
