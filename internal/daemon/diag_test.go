package daemon

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The diagnostic file has what a support case needs (sync's warnings, the
// AI tools that connected and their calls) and none of the numbers.
func TestDiagnosticExportIsUsefulAndAnonymous(t *testing.T) {
	tmp := ""
	if runtime.GOOS != "windows" {
		tmp = "/tmp" // the store's socket path must stay short
	}
	root, err := os.MkdirTemp(tmp, "diag-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	store := filepath.Join(root, "s")
	if err := os.MkdirAll(store, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"AUTHED", "UNDECRYPTABLE"} {
		if err := os.WriteFile(filepath.Join(store, f), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	port := freePort(t)
	d, err := Start(Config{Port: port, RequirePort: true, WacliBin: fakeWacli, StoreDir: store,
		DataDir: filepath.Join(root, "data"), LogPath: filepath.Join(root, "logs", "whatsapp-mcp.log")})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Stop()
	waitSync(t, d, "connected")

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	post := func(body string) {
		req, _ := http.NewRequest(http.MethodPost, base+"/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "test-agent")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	post(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"claude-ai","version":"0.1.0"}}}`)
	post(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	post(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"send_text_message","arguments":{"chat_jid":"5511987654321@s.whatsapp.net","message":"segredo do Felipe"}}}`)

	var out string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, base+"/diagnostico/logs.txt?horas=24", nil)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		_, _ = b.ReadFrom(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Disposition"), "whatsapp-mcp-diagnostics-") {
			t.Fatalf("export = %d %q", resp.StatusCode, resp.Header.Get("Content-Disposition"))
		}
		out = b.String()
		if strings.Contains(out, "wacli_stderr") && strings.Contains(out, "tool_call") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	for _, want := range []string{"app started", "sync sync_started", "code=undecryptable_message", "group#", "user#", "msg#",
		"wacli_stderr", "mcp client_initialize", "client=claude-ai", "mcp notification", "method=notifications/initialized",
		"mcp tool_call", "tool=send_text_message", `"message":"<omitted>"`, "wacli command command=\"auth status\""} {
		if !strings.Contains(out, want) {
			t.Errorf("%q missing from the export:\n%s", want, out)
		}
	}
	for _, leak := range []string{"5511987654321", "120363025246125486", "3EB0C431D5A2F1E9B7", "segredo", root} {
		if strings.Contains(out, leak) {
			t.Errorf("%q leaked into the export:\n%s", leak, out)
		}
	}
}
