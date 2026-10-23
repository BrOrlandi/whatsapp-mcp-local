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

// Options says where the daemon is and what to tell the client when it is
// not there.
type Options struct {
	// URL is the daemon's MCP address. It is asked again when the daemon does
	// not answer, so a port changed in the app is picked up without
	// restarting the client.
	URL   func() string
	Token string
	// NotRunning is the error a client gets when nothing answers, written
	// for the model to relay: how to start the daemon.
	NotRunning string
}

// Run copies newline-delimited JSON-RPC from in to the daemon, and the
// answers back to out. Requests run concurrently, so a long tool call does not
// hold up a ping.
func Run(ctx context.Context, opts Options, in io.Reader, out io.Writer) error {
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
			if resp := forward(ctx, httpClient, opts, hint, line); resp != nil {
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

func forward(ctx context.Context, client *http.Client, opts Options, hint string, line []byte) []byte {
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

	post := func(url string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(line))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-MCP-Client", hint)
		req.Header.Set("Accept", "application/json, text/event-stream")
		if opts.Token != "" {
			req.Header.Set("Authorization", "Bearer "+opts.Token)
		}
		return client.Do(req)
	}
	url := opts.URL()
	resp, err := post(url)
	if err != nil && ctx.Err() == nil {
		// The port may have moved since the last request: look again.
		if again := opts.URL(); again != url {
			url = again
			resp, err = post(url)
		}
	}
	if err != nil {
		return fail("%s (nothing answers at %s: %v)", opts.NotRunning, url, err)
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
