//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

const appPath = "/Applications/WhatsApp MCP.app"

// testBridge runs the stdio program Claude Desktop starts, as Claude Desktop
// does: JSON-RPC lines in, answers out, forwarded to the app.
func testBridge(t *testing.T, e env) {
	in := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"integration","version":"1"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"whatsapp_status","arguments":{}}}
`
	bridge := appPath + "/Contents/MacOS/whatsapp-mcp-bridge"
	if out, err := remote(t, e, `"`+bridge+`" --version`, nil); err != nil || (e.version != "" && strings.TrimSpace(out) != e.version) {
		t.Errorf("bridge --version = %q %v", out, err)
	}
	// The bridge reads until stdin closes; a pause keeps it open until the
	// answers are back.
	out, err := remote(t, e, `(cat; sleep 8) | "`+bridge+`"`, []byte(in))
	if err != nil {
		t.Fatalf("bridge: %v\n%s", err, out)
	}
	answers := map[float64]map[string]any{}
	for _, line := range strings.Split(out, "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) == nil {
			if id, ok := m["id"].(float64); ok {
				answers[id] = m
			}
		}
	}
	if str(answers[1], "result.serverInfo.name") == "" {
		t.Errorf("bridge initialize = %s", clipAny(answers[1]))
	}
	if n := len(list(answers[2], "result.tools")); n != len(expectedTools) {
		t.Errorf("bridge tools/list has %d tools, want %d", n, len(expectedTools))
	}
	if get(answers[3], "result.isError") == true || len(list(answers[3], "result.content")) == 0 {
		t.Errorf("bridge tools/call = %s", clipAny(answers[3]))
	}
}

// testRestart quits the app and opens it again, as starting the computer
// does: it must come back connected and post the online event.
func testRestart(t *testing.T, a *app, r *receiver) {
	a.hook(t, r, "/online", obj{"events": []string{"online"}})
	before := time.Now()
	script := `pkill -TERM -x "WhatsApp MCP"; for i in $(seq 1 20); do pgrep -x "WhatsApp MCP" >/dev/null || break; sleep 1; done
pgrep -x wacli >/dev/null && sleep 3
open -a "` + appPath + `"`
	if out, err := remote(t, a.e, script, nil); err != nil {
		t.Fatalf("restarting the app: %v %s", err, out)
	}
	eventually(t, 2*time.Minute, "the app back and ready", func() bool {
		code, body, err := a.try(http.MethodGet, "/api/status", nil)
		var st any
		return err == nil && code == 200 && json.Unmarshal(body, &st) == nil && str(st, "next") == "ready"
	})
	d, ok := r.wait(t, "/online", 4*time.Minute, "the online event after the restart", func(d delivery) bool {
		return d.event == "online"
	})
	if !ok {
		return
	}
	if str(d.body, "online.reason") != "started" {
		t.Errorf("online reason = %q, want started", str(d.body, "online.reason"))
	}
	if since := str(d.body, "online.offline_since"); since == "" {
		t.Error("online event without offline_since, though the app was connected before")
	} else if ts, err := time.Parse(time.RFC3339, since); err != nil || ts.After(time.Now()) || before.Sub(ts) > 10*time.Minute {
		t.Errorf("offline_since = %s, not shortly before the restart", since)
	}
	// One sync left running: the old one went with the old app. The tools
	// start short-lived ones too, so allow them a moment to end.
	eventually(t, 30*time.Second, "a single wacli sync after the restart", func() bool {
		out, _ := remote(t, a.e, `pgrep -f "wacli.* sync" | wc -l`, nil)
		return strings.TrimSpace(out) == "1"
	})
}
