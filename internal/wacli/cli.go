// Package wacli drives the wacli binary: one-shot commands through its JSON
// envelope, and the long-running `sync --follow` process that owns the
// WhatsApp session.
//
// wacli keeps its packages internal, so the CLI is the only supported surface.
// That is also the right boundary: wacli stays responsible for the protocol and
// its store, and this gateway only decides when to call it.
package wacli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/diag"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/platform"
)

// CLI runs wacli commands against one store.
type CLI struct {
	Bin      string
	StoreDir string
	// Diag, when set, gets a line for every command: which one, its flags
	// (never their values), how long it took and whether it failed.
	Diag *diag.Log
}

// envelope is the shape every `--json` command prints.
type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *string         `json:"error"`
}

// Error is a command wacli ran and refused. Message is wacli's own words,
// which are written for people and are worth passing on unchanged.
type Error struct {
	Args    []string
	Message string
}

func (e *Error) Error() string { return e.Message }

// Version is the version the wacli at bin reports ("0.26.0"), or "" when
// it does not answer.
func Version(ctx context.Context, bin string) string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--version")
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	platform.Background(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimPrefix(fields[len(fields)-1], "v")
}

// Run executes one command with --json and returns its data.
func (c *CLI) Run(ctx context.Context, args ...string) (data json.RawMessage, err error) {
	if c.Diag != nil {
		started := time.Now()
		defer func() {
			command, flags := commandShape(args)
			c.Diag.Event("wacli", "command", diag.Fields{"command": command, "flags": flags,
				"ms": time.Since(started).Milliseconds(), "error": err})
		}()
	}
	full := append([]string{"--json"}, args...)
	cmd := exec.CommandContext(ctx, c.Bin, full...)
	cmd.Env = c.env()
	platform.Background(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	var env envelope
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &env); err == nil {
		if env.Success {
			return env.Data, nil
		}
		if env.Error != nil && *env.Error != "" {
			return nil, &Error{Args: args, Message: *env.Error}
		}
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("wacli %s did not finish in time: %w", strings.Join(args, " "), ctx.Err())
	}
	detail := lastLine(stderr.String())
	if detail == "" && runErr != nil {
		detail = runErr.Error()
	}
	if detail == "" {
		detail = "no output"
	}
	return nil, &Error{Args: args, Message: detail}
}

// commandShape is a command's words ("chats mark-read") and its flag names,
// without the values, which can be numbers or text.
func commandShape(args []string) (string, []string) {
	var words, flags []string
	takesValue := false
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			name, _, hasValue := strings.Cut(a, "=")
			flags = append(flags, name)
			takesValue = !hasValue && !booleanFlags[name]
			continue
		}
		if takesValue {
			takesValue = false
			continue
		}
		if commandWord(a) {
			words = append(words, a)
		}
	}
	return strings.Join(words, " "), flags
}

// booleanFlags are the flags this app passes without a value.
var booleanFlags = map[string]bool{"--read-only": true, "--json": true, "--events": true, "--follow": true,
	"--force": true, "--help": true, "--all": true, "--refresh-contacts": true, "--refresh-groups": true}

// commandWord is a subcommand's name: lowercase letters and dashes, which
// a phone number, an address or a message id never is.
func commandWord(s string) bool {
	if s == "" || len(s) > 30 {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && r != '-' {
			return false
		}
	}
	return true
}

// Decode runs a command and unmarshals its data into v.
func (c *CLI) Decode(ctx context.Context, v any, args ...string) error {
	data, err := c.Run(ctx, args...)
	if err != nil {
		return err
	}
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("could not read wacli's answer to %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

// Supports reports whether a wacli command has a flag, from its help: the
// command line may run an older wacli than the app ships.
func (c *CLI) Supports(ctx context.Context, flag string, command ...string) bool {
	cmd := exec.CommandContext(ctx, c.Bin, append(command, "--help")...)
	cmd.Env = c.env()
	platform.Background(cmd)
	out, _ := cmd.CombinedOutput()
	return bytes.Contains(out, []byte(flag))
}

// IsLockError reports whether wacli refused because another process holds the
// store lock.
func IsLockError(err error) bool {
	var e *Error
	return errors.As(err, &e) && strings.Contains(strings.ToLower(e.Message), "lock")
}
