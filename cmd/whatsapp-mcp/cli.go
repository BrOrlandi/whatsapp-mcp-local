package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
)

// Exit codes, for scripts and agents that run the command line. docs/servidor.md
// lists them.
const (
	exitOK         = 0 // done; for status, WhatsApp is ready
	exitFailed     = 1 // something went wrong; the message says what
	exitUsage      = 2 // the command was given wrong
	exitNotRunning = 3 // the daemon does not answer on its port
	exitNotReady   = 4 // WhatsApp is not connected, or the pairing ended without it
	exitTimeout    = 5 // --timeout ran out
	exitConflict   = 6 // not possible here: a setting fixed by the environment, a busy port, Docker
	exitCancelled  = 130
)

// exitError is an error with the exit code it ends the program with.
type exitError struct {
	code int
	err  error
}

func (e exitError) Error() string { return e.err.Error() }
func (e exitError) Unwrap() error { return e.err }

func fail(code int, format string, args ...any) error {
	return exitError{code, fmt.Errorf(format, args...)}
}

func exitCode(err error) int {
	var e exitError
	if errors.As(err, &e) {
		return e.code
	}
	return exitFailed
}

// jsonOutput is set by a command run with --json: its result, and its
// error, go to stdout as JSON instead of text.
var jsonOutput bool

// stdout is where commands write; tests replace it.
var stdout io.Writer = os.Stdout

func printJSON(v any) {
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	if !jsonOutput {
		enc.SetIndent("", "  ")
	}
	_ = enc.Encode(v)
}

func say(format string, args ...any) {
	if !jsonOutput {
		fmt.Fprintf(stdout, format+"\n", args...)
	}
}

// flags is a command's flag set. Its flags may come before or after the
// positional arguments: `webhooks add URL --events message` works as well as
// the other way round.
type flags struct {
	*flag.FlagSet
	json *bool
}

func newFlags(name string) *flags {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return &flags{FlagSet: fs, json: fs.Bool("json", false, "")}
}

func (f *flags) parse(args []string) ([]string, error) {
	var rest []string
	for {
		if err := f.Parse(args); err != nil {
			return nil, fail(exitUsage, "%s: %v", f.Name(), err)
		}
		args = f.Args()
		if len(args) == 0 {
			break
		}
		if args[0] == "--" {
			rest = append(rest, args[1:]...)
			break
		}
		rest, args = append(rest, args[0]), args[1:]
	}
	jsonOutput = *f.json
	return rest, nil
}

// api talks to the running daemon over its local API (docs/api.md).
type api struct {
	base   string
	token  string
	client *http.Client
}

func newAPI(cfg config) api {
	return api{base: cfg.baseURL(), token: cfg.Token, client: &http.Client{Timeout: 2 * time.Minute}}
}

// call sends a request and decodes the JSON answer into out (when not nil).
func (a api) call(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if method != http.MethodGet {
		if body == nil {
			body = struct{}{}
		}
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, reader)
	if err != nil {
		return err
	}
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fail(exitNotRunning, "%s", i18n.Tf("o WhatsApp MCP não responde em %s: inicie o serviço com whatsapp-mcp service install (ou rode whatsapp-mcp serve)", a.base))
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		msg := strings.TrimSpace(string(raw))
		if json.Unmarshal(raw, &e) == nil && e.Error != "" {
			msg = e.Error
		}
		switch resp.StatusCode {
		case http.StatusForbidden, http.StatusUnauthorized:
			return fail(exitFailed, "%s", i18n.T("o WhatsApp MCP recusou o pedido: confira o token (whatsapp-mcp token)"))
		case http.StatusConflict:
			return fail(exitConflict, "%s", msg)
		case http.StatusNotFound:
			return fail(exitFailed, "%s", i18n.T("esta versão do WhatsApp MCP não conhece este pedido: reinicie o serviço para usar a versão instalada (whatsapp-mcp service restart)"))
		}
		return fail(exitFailed, "%s", msg)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// status is GET /api/status, the fields the command line reads.
type status struct {
	App      string `json:"app"`
	Version  string `json:"version"`
	Desktop  bool   `json:"desktop"`
	MCPURL   string `json:"mcp_url"`
	WhatsApp struct {
		State     string `json:"state"`
		Paired    bool   `json:"paired"`
		Connected bool   `json:"connected"`
		Phone     string `json:"phone"`
		Name      string `json:"name"`
		Label     string `json:"label"`
	} `json:"whatsapp"`
	Pairing struct {
		State     string `json:"state"`
		QR        string `json:"qr"`
		QRPNG     string `json:"qr_png"`
		QRVersion int    `json:"qr_version"`
		Code      string `json:"code"`
		Error     string `json:"error"`
	} `json:"pairing"`
	Health struct {
		Status  string `json:"status"`
		Summary string `json:"summary"`
		Checks  []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
			Fix    string `json:"fix"`
		} `json:"checks"`
	} `json:"health"`
	Next string `json:"next"`
}

func (a api) status(ctx context.Context) (status, json.RawMessage, error) {
	var raw json.RawMessage
	if err := a.call(ctx, http.MethodGet, "/api/status", nil, &raw); err != nil {
		return status{}, nil, err
	}
	var st status
	err := json.Unmarshal(raw, &st)
	return st, raw, err
}

// inContainer is set by the Docker image, where there is no login service:
// the container is the service.
func inContainer() bool { return os.Getenv("WHATSAPP_MCP_CONTAINER") != "" }

// headless reports whether there is no screen to open a browser on: a
// server, or a session over SSH.
func headless() bool {
	if os.Getenv("SSH_CONNECTION") != "" || inContainer() {
		return true
	}
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return runtime.GOOS == "linux"
	}
	return false
}
