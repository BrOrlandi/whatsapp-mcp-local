//go:build integration

// Package integration tests the real app, installed on the test Mac and
// paired with a WhatsApp account: the panel, the local API, every MCP tool,
// the webhooks, the bridge Claude Desktop runs, and a restart. It is not part
// of go test ./... nor of CI; scripts/integration.sh builds the app, installs
// it on the test Mac, opens the tunnels and runs it (docs/testes-integracao.md).
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// env is what scripts/integration.sh passes in.
type env struct {
	base     string // the app's address through the tunnel, http://127.0.0.1:port
	token    string // WHATSAPP_MCP_TOKEN, when the app asks for one
	hookAddr string // where this process listens for the webhooks
	hookURL  string // the same listener as the app on the test Mac reaches it
	testMac  string // user@host for ssh
	version  string // the version the app must report
	group    string // optional group the account administers, for the group tools
}

func loadEnv(t *testing.T) env {
	e := env{
		base:     os.Getenv("WMCP_URL"),
		token:    os.Getenv("WMCP_TOKEN"),
		hookAddr: os.Getenv("WMCP_HOOK_LISTEN"),
		hookURL:  os.Getenv("WMCP_HOOK_URL"),
		testMac:  os.Getenv("TEST_MAC"),
		version:  os.Getenv("WMCP_VERSION"),
		group:    os.Getenv("WMCP_GROUP"),
	}
	if e.base == "" || e.testMac == "" {
		t.Skip("run through scripts/integration.sh (WMCP_URL and TEST_MAC are not set)")
	}
	return e
}

// obj is a decoded JSON object, read by dotted paths.
type obj = map[string]any

func get(v any, path string) any {
	for _, k := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

func str(v any, path string) string {
	s, _ := get(v, path).(string)
	return s
}

func list(v any, path string) []any {
	if path == "" {
		l, _ := v.([]any)
		return l
	}
	l, _ := get(v, path).([]any)
	return l
}

// firstList returns the first array found under any of the keys: tools name
// their lists after what they hold (chats, messages, groups…).
func firstList(v any, keys ...string) []any {
	for _, k := range keys {
		if l := list(v, k); l != nil {
			return l
		}
	}
	if l, ok := v.([]any); ok {
		return l
	}
	return nil
}

// app talks to the app over HTTP.
type app struct {
	t    *testing.T
	e    env
	http *http.Client
	rpc  atomic.Int64
}

func newApp(t *testing.T, e env) *app {
	return &app{t: t, e: e, http: &http.Client{Timeout: 3 * time.Minute}}
}

func (a *app) do(method, path string, body any) (int, []byte) {
	a.t.Helper()
	code, out, err := a.try(method, path, body)
	if err != nil {
		a.t.Fatalf("%s %s: %v", method, path, err)
	}
	return code, out
}

// try is do without failing when the app does not answer, as while it
// restarts.
func (a *app) try(method, path string, body any) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, a.e.base+path, rd)
	if err != nil {
		return 0, nil, err
	}
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	if a.e.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.e.token)
	}
	res, err := a.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, out, nil
}

// json calls the API and decodes the answer, failing on any status but want.
func (a *app) json(method, path string, body any, want int) any {
	a.t.Helper()
	code, out := a.do(method, path, body)
	if code != want {
		a.t.Fatalf("%s %s = %d, want %d: %s", method, path, code, want, clip(out))
	}
	var v any
	if err := json.Unmarshal(out, &v); err != nil {
		a.t.Fatalf("%s %s: not JSON: %s", method, path, clip(out))
	}
	return v
}

// mcp sends one JSON-RPC request to /mcp and returns its result.
func (a *app) mcp(method string, params any) (obj, error) {
	id := a.rpc.Add(1)
	body, _ := json.Marshal(obj{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	req, _ := http.NewRequest(http.MethodPost, a.e.base+"/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if a.e.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.e.token)
	}
	res, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("/mcp %s = %d: %s", method, res.StatusCode, clip(raw))
	}
	// A streamable HTTP server may answer as an event stream.
	if strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "data:") {
				raw = []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
	}
	var msg struct {
		Result obj `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, fmt.Errorf("/mcp %s: %v: %s", method, err, clip(raw))
	}
	if msg.Error != nil {
		return nil, fmt.Errorf("/mcp %s: %d %s", method, msg.Error.Code, msg.Error.Message)
	}
	return msg.Result, nil
}

// call runs a tool and returns its decoded result. A tool error fails the test
// unless allowErr; the error text is then returned in "error".
func (a *app) call(tool string, args obj) any {
	a.t.Helper()
	v, isErr := a.tryCall(tool, args)
	if isErr {
		a.t.Fatalf("%s(%v) failed: %s", tool, args, clipAny(v))
	}
	return v
}

func (a *app) tryCall(tool string, args obj) (any, bool) {
	a.t.Helper()
	if args == nil {
		args = obj{}
	}
	res, err := a.mcp("tools/call", obj{"name": tool, "arguments": args})
	if err != nil {
		a.t.Fatalf("%s: %v", tool, err)
	}
	isErr, _ := res["isError"].(bool)
	var out any
	for _, c := range list(res, "content") {
		if str(c, "type") == "text" {
			if err := json.Unmarshal([]byte(str(c, "text")), &out); err != nil {
				out = str(c, "text")
			}
			break
		}
	}
	if out == nil {
		out = res
	}
	return out, isErr
}

// eventually retries check until it holds or the time runs out.
func eventually(t *testing.T, within time.Duration, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		if check() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s", within, what)
		}
		time.Sleep(2 * time.Second)
	}
}

// remote runs a shell command on the test Mac.
func remote(t *testing.T, e env, script string, stdin []byte) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", e.testMac, script)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func clip(b []byte) string {
	if len(b) > 600 {
		return string(b[:600]) + "…"
	}
	return string(b)
}

func clipAny(v any) string {
	b, _ := json.Marshal(v)
	return clip(b)
}
