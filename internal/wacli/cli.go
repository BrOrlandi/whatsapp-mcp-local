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

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/platform"
)

// CLI runs wacli commands against one store.
type CLI struct {
	Bin      string
	StoreDir string
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

// Run executes one command with --json and returns its data.
func (c *CLI) Run(ctx context.Context, args ...string) (json.RawMessage, error) {
	full := append([]string{"--json"}, args...)
	cmd := exec.CommandContext(ctx, c.Bin, full...)
	cmd.Env = append(os.Environ(), "WACLI_STORE_DIR="+c.StoreDir, "NO_COLOR=1")
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

// IsLockError reports whether wacli refused because another process holds the
// store lock.
func IsLockError(err error) bool {
	var e *Error
	return errors.As(err, &e) && strings.Contains(strings.ToLower(e.Message), "lock")
}
