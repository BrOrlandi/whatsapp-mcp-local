package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/public"
)

// An agent on another computer reads the tools as they are for it, and an
// export comes as a link on the public address, never as a server path.
func TestToolsForAnAgentOutside(t *testing.T) {
	d := startServerMode(t, true)
	seedStore(t, d.cfg.StoreDir)
	h := d.PublicHandler()
	key, _, _ := public.CreateKey(context.Background(), d.primary.st, "Outside", false, time.Now())
	list := toolsCall(t, h, key, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	desc := map[string]string{}
	for _, tool := range list["result"].(map[string]any)["tools"].([]any) {
		m := tool.(map[string]any)
		desc[m["name"].(string)] = m["description"].(string)
	}
	for _, name := range []string{"send_media_message", "download_media", "export_messages"} {
		if strings.Contains(desc[name], "this machine") || strings.Contains(desc[name], "localhost") || !strings.Contains(desc[name], "server") {
			t.Errorf("%s, for an agent outside: %s", name, desc[name])
		}
	}
	// Programs on the server keep the local text.
	local := serveAgent(d.HTTPHandler(), func() *http.Request {
		r := outsideRequest("POST", "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, map[string]string{"X-Forwarded-For": "", "X-Forwarded-Proto": ""})
		r.Host = "127.0.0.1"
		return r
	}())
	if !strings.Contains(local.Body.String(), "from a file on this machine") {
		t.Error("the local tool list changed")
	}

	out := toolsCall(t, h, key, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"export_messages","arguments":{}}}`)
	text := out["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	var res map[string]any
	_ = json.Unmarshal([]byte(text), &res)
	link, _ := res["url"].(string)
	if _, hasPath := res["path"]; hasPath || !strings.HasPrefix(link, publicURL+"/media/") {
		t.Fatalf("the export for an agent outside: %s", text)
	}
	u, _ := url.Parse(link)
	if rec := serveAgent(h, outsideRequest("GET", u.Path, "", nil)); rec.Code != 200 {
		t.Fatalf("downloading the export: %d", rec.Code)
	}
}
