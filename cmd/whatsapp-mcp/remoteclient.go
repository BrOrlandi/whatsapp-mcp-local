package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/bridge"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/remote"
)

// The command line as a client of another daemon's remote access, for a
// program without a screen (an agent in a container, say) to use that
// daemon's MCP: `remote pair` pairs it once with a code from the server,
// and `bridge --remote` serves the MCP on stdio through the session, as the
// desktop app does on its port. The device's key is a file only its owner
// reads, next to what the client knows about the server.

const (
	remoteClientFile = "remote-client.json"
	remoteClientKey  = "remote-client.key"
)

type remoteClientState struct {
	remote.Device
	LastStamp int64 `json:"last_stamp,omitempty"`
}

func remoteClientPaths(cfg config) (string, string) {
	return filepath.Join(cfg.DataDir, remoteClientFile), filepath.Join(cfg.DataDir, remoteClientKey)
}

// writePrivate writes a file only its owner reads, replacing it whole.
func writePrivate(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// remotePairCmd pairs this command line with a server: remote pair
// https://server code [--name N]. The code's scope (tools, for an agent)
// is chosen when it is made, on the server.
func remotePairCmd(cfg config, rest []string, name string) error {
	if len(rest) != 2 {
		return fail(exitUsage, "usage: whatsapp-mcp remote pair https://server <code> [--name N]")
	}
	if name == "" {
		name, _ = os.Hostname()
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	device, key, err := remote.Pair(ctx, rest[0], rest[1], name)
	if err != nil {
		return fail(exitFailed, "%s", i18n.Tf("o pareamento não deu certo: %v", err))
	}
	file, keyFile := remoteClientPaths(cfg)
	raw, _ := json.MarshalIndent(remoteClientState{Device: device}, "", "  ")
	if err := writePrivate(keyFile, key); err != nil {
		return err
	}
	if err := writePrivate(file, raw); err != nil {
		return err
	}
	if jsonOutput {
		printJSON(map[string]string{"server": device.Server, "device_id": device.ID, "scope": device.Scope,
			"server_fingerprint": remote.Fingerprint(device.ServerKey)})
		return nil
	}
	say("%s", i18n.Tf("Pareado com %s (escopo %s, aparelho %s). O MCP do servidor fica em: whatsapp-mcp bridge --remote", device.Server, device.Scope, device.ID))
	return nil
}

// bridgeRemote serves the server's MCP on stdio through remote access.
func bridgeRemote(cfg config) error {
	file, keyFile := remoteClientPaths(cfg)
	raw, err := os.ReadFile(file)
	if err != nil {
		return fail(exitNotReady, "%s", i18n.T("esta linha de comando não está pareada com um servidor: whatsapp-mcp remote pair https://servidor <código>"))
	}
	var st remoteClientState
	if err := json.Unmarshal(raw, &st); err != nil {
		return err
	}
	key, err := os.ReadFile(keyFile)
	if err != nil {
		return fail(exitNotReady, "%s", i18n.T("a chave deste aparelho não está aqui: pareie de novo (whatsapp-mcp remote pair)"))
	}
	client, err := remote.NewClient(st.Device, key)
	if err != nil {
		return err
	}
	// The handshake stamp only grows, across restarts too.
	client.LastStamp = st.LastStamp
	client.OnStamp = func(stamp int64) {
		st.LastStamp = stamp
		if b, err := json.MarshalIndent(st, "", "  "); err == nil {
			_ = writePrivate(file, b)
		}
	}
	// A key rotation would need somewhere to keep the new key first: the
	// command line does not rotate (OnNextKey refuses).
	client.OnNextKey = func([]byte) error { return errors.New("the command line does not rotate its key") }

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go client.Run(ctx)
	deadline := time.Now().Add(30 * time.Second)
	for client.Status().State != remote.StateConnected && time.Now().Before(deadline) && ctx.Err() == nil {
		if client.Status().State == remote.StateRefused {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Even unconnected, the bridge runs: each call says what is wrong, and
	// the client keeps trying in the background.
	status := client.Status()
	if status.State != remote.StateConnected {
		fmt.Fprintf(os.Stderr, "whatsapp-mcp: remote %s: %s\n", status.State, status.LastError)
	}
	return bridge.Run(ctx, bridge.Options{
		URL:        func() string { return "http://remote/mcp" },
		Transport:  client,
		NotRunning: fmt.Sprintf("the WhatsApp MCP server at %s cannot be reached right now (state: %s)", st.Server, client.Status().State),
	}, os.Stdin, os.Stdout)
}
