// Package bridge turns the stdio MCP transport into requests to the running
// daemon. Claude Desktop, and Cowork through it, only start stdio servers; the
// bridge lets them use the one daemon that owns the WhatsApp session instead
// of each starting its own.
package bridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Run copies newline-delimited JSON-RPC from in to the daemon at url, and the
// answers back to out. Requests run concurrently, so a long tool call does not
// hold up a ping.
func Run(ctx context.Context, url, token string, in io.Reader, out io.Writer) error {
	httpClient := &http.Client{Timeout: 15 * time.Minute}
	var writeMu sync.Mutex
	write := func(line []byte) {
		writeMu.Lock()
		defer writeMu.Unlock()
		_, _ = out.Write(append(bytes.TrimSpace(line), '\n'))
	}

	// The client's name, learned from its initialize request, travels on every
	// request so the daemon can tell which app a call came from.
	var client atomic.Value
	client.Store("bridge:unknown")

	var wg sync.WaitGroup
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if name := initializeClient(line); name != "" {
			client.Store("bridge:" + name)
		}
		hint := client.Load().(string)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if resp := forward(ctx, httpClient, url, token, hint, line); resp != nil {
				write(resp)
			}
		}()
	}
	wg.Wait()
	return scanner.Err()
}

func initializeClient(line []byte) string {
	var msg struct {
		Method string `json:"method"`
		Params struct {
			ClientInfo struct {
				Name string `json:"name"`
			} `json:"clientInfo"`
		} `json:"params"`
	}
	if json.Unmarshal(line, &msg) != nil || msg.Method != "initialize" {
		return ""
	}
	return msg.Params.ClientInfo.Name
}

func forward(ctx context.Context, client *http.Client, url, token, hint string, line []byte) []byte {
	var msg struct {
		ID json.RawMessage `json:"id"`
	}
	_ = json.Unmarshal(line, &msg)
	fail := func(format string, args ...any) []byte {
		if len(msg.ID) == 0 {
			return nil // a notification has nobody to answer
		}
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": msg.ID,
			"error": map[string]any{"code": -32000, "message": fmt.Sprintf(format, args...)}})
		return body
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(line))
	if err != nil {
		return fail("%v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MCP-Client", hint)
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fail("the whatsapp-mcp-v2 daemon is not answering at %s (%v); start it with `whatsapp-mcp-v2 service install` or `whatsapp-mcp-v2 serve`", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusAccepted || len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return fail("the daemon answered %s: %s", resp.Status, bytes.TrimSpace(body))
	}
	return body
}
