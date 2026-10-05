// Package legacy finds the installation the command line made before the app
// existed (a login service, wacli's store in ~/.wacli, the gateway's data in
// ~/.whatsapp-mcp-v2) and moves it into the app, so a paired WhatsApp keeps
// working without a new QR code.
//
// The data is moved, never copied: two processes with the same WhatsApp
// session would fight over the linked device.
package legacy

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/index"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/platform"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/wacli"
)

// Label is the command line's login service, on macOS and Linux.
const Label = "com.brorlandi.whatsapp-mcp-v2"

const unitName = "whatsapp-mcp-v2.service"

// Install is an earlier installation found on this computer.
type Install struct {
	// Service is the login service's file, or "" when there is none.
	Service string
	// Binary is the program the service ran, which Claude Desktop may still
	// be configured to start.
	Binary   string
	StoreDir string
	DataDir  string
	Port     int
	// Name and Phone are the account the store is paired with.
	Name  string
	Phone string
}

// Find looks for an earlier installation whose WhatsApp is still paired. It
// returns nil when there is none, or nothing worth moving.
func Find(ctx context.Context, wacliBin string) *Install {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	in := &Install{
		StoreDir: filepath.Join(home, ".wacli"),
		DataDir:  filepath.Join(home, ".whatsapp-mcp-v2"),
		Binary:   filepath.Join(home, ".local", "bin", "whatsapp-mcp-v2"),
		Port:     0,
	}
	if runtime.GOOS == "linux" {
		if _, err := os.Stat(in.StoreDir); err != nil {
			in.StoreDir = filepath.Join(home, ".local", "state", "wacli")
			if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
				in.StoreDir = filepath.Join(xdg, "wacli")
			}
		}
	}
	readService(in, home)
	if _, err := os.Stat(filepath.Join(in.StoreDir, "session.db")); err != nil {
		return nil
	}
	cli := &wacli.CLI{Bin: wacliBin, StoreDir: in.StoreDir}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var status wacli.Account
	if err := cli.Decode(cctx, &status, "--read-only", "auth", "status"); err != nil || !status.Authenticated {
		return nil
	}
	in.Phone = status.Phone
	if idx, err := index.Open(filepath.Join(in.StoreDir, "wacli.db")); err == nil {
		in.Name = idx.OwnName(cctx, status.JID)
		idx.Close()
	}
	return in
}

// readService fills in what the login service says: where its store and data
// are, which port it served on, and which program it ran.
func readService(in *Install, home string) {
	switch runtime.GOOS {
	case "darwin":
		path := filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
		raw, err := os.ReadFile(path)
		if err != nil {
			return
		}
		in.Service = path
		args, env := parsePlist(raw)
		if len(args) > 0 {
			in.Binary = args[0]
		}
		apply(in, env)
	case "linux":
		path := filepath.Join(home, ".config", "systemd", "user", unitName)
		raw, err := os.ReadFile(path)
		if err != nil {
			return
		}
		in.Service = path
		env := map[string]string{}
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if v, ok := strings.CutPrefix(line, "Environment="); ok {
				if uq, err := strconv.Unquote(v); err == nil {
					v = uq
				}
				if k, val, ok := strings.Cut(v, "="); ok {
					env[k] = val
				}
			}
			if v, ok := strings.CutPrefix(line, "ExecStart="); ok {
				if first := strings.Fields(v); len(first) > 0 {
					if uq, err := strconv.Unquote(first[0]); err == nil {
						in.Binary = uq
					} else {
						in.Binary = first[0]
					}
				}
			}
		}
		apply(in, env)
	}
}

func apply(in *Install, env map[string]string) {
	if v := env["WACLI_STORE_DIR"]; v != "" {
		in.StoreDir = v
	}
	if v := env["WHATSAPP_MCP_DATA"]; v != "" {
		in.DataDir = v
	}
	if v := env["WHATSAPP_MCP_PORT"]; v != "" {
		in.Port, _ = strconv.Atoi(v)
	}
}

// parsePlist reads ProgramArguments and EnvironmentVariables from a launchd
// property list.
func parsePlist(raw []byte) (args []string, env map[string]string) {
	env = map[string]string{}
	dec := xml.NewDecoder(strings.NewReader(string(raw)))
	for {
		tok, err := dec.Token()
		if err != nil {
			return args, env
		}
		if start, ok := tok.(xml.StartElement); ok && start.Name.Local == "dict" {
			root, _ := plistValue(dec, start).(map[string]any)
			for _, a := range asList(root["ProgramArguments"]) {
				if s, ok := a.(string); ok {
					args = append(args, s)
				}
			}
			if vars, ok := root["EnvironmentVariables"].(map[string]any); ok {
				for k, v := range vars {
					if s, ok := v.(string); ok {
						env[k] = s
					}
				}
			}
			return args, env
		}
	}
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

// plistValue decodes the element start opened: a dict, an array or a scalar.
func plistValue(dec *xml.Decoder, start xml.StartElement) any {
	switch start.Name.Local {
	case "dict":
		out := map[string]any{}
		key := ""
		for {
			tok, err := dec.Token()
			if err != nil {
				return out
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "key" {
					_ = dec.DecodeElement(&key, &t)
					continue
				}
				out[key] = plistValue(dec, t)
			case xml.EndElement:
				return out
			}
		}
	case "array":
		var out []any
		for {
			tok, err := dec.Token()
			if err != nil {
				return out
			}
			switch t := tok.(type) {
			case xml.StartElement:
				out = append(out, plistValue(dec, t))
			case xml.EndElement:
				return out
			}
		}
	case "true", "false":
		_ = dec.Skip()
		return start.Name.Local == "true"
	default:
		var v string
		_ = dec.DecodeElement(&v, &start)
		return v
	}
}

// ErrDataNotMoved means the store moved, so WhatsApp now runs in the app, but
// some of the gateway's own data (transcripts, model, media) stayed behind.
var ErrDataNotMoved = errors.New("the WhatsApp session moved, but not all of the other data did")

// Move stops the old login service, waits for its wacli to let go of the
// store, and moves the store and the gateway's data into the app's data
// folder. When the store cannot be moved, the service is started again and
// nothing is lost. An error wrapping ErrDataNotMoved means the store did
// move; any other error means it did not.
func Move(ctx context.Context, in *Install, storeDir, dataDir string) error {
	if err := stopService(in); err != nil {
		return fmt.Errorf("não foi possível parar o serviço antigo: %w", err)
	}
	if err := waitReleased(ctx, in.StoreDir); err != nil {
		startService(in)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(storeDir), 0o700); err != nil {
		startService(in)
		return err
	}
	if err := removeIfEmpty(storeDir); err != nil {
		startService(in)
		return fmt.Errorf("a pasta %s já tem um WhatsApp; nada foi movido", storeDir)
	}
	if err := move(in.StoreDir, storeDir); err != nil {
		startService(in)
		return fmt.Errorf("não foi possível mover %s: %w", in.StoreDir, err)
	}
	// The store has moved: from here on the old service cannot run, and what
	// fails is reported without undoing the move.
	var errs []error
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		errs = append(errs, err)
	}
	for _, name := range []string{"state.db", "state.db-wal", "state.db-shm", "models", "media"} {
		src := filepath.Join(in.DataDir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(dataDir, name)
		if err := removeIfEmpty(dst); err != nil {
			errs = append(errs, fmt.Errorf("%s já existe em %s", name, dataDir))
			continue
		}
		if err := move(src, dst); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	if in.Service != "" {
		if err := os.Remove(in.Service); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
		if runtime.GOOS == "linux" {
			_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
		}
	}
	if len(errs) > 0 {
		return errors.Join(append([]error{ErrDataNotMoved}, errs...)...)
	}
	return nil
}

func stopService(in *Install) error {
	if in.Service == "" {
		return nil
	}
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("launchctl", "bootout", "gui/"+strconv.Itoa(os.Getuid()), in.Service).CombinedOutput()
		if err != nil && !strings.Contains(string(out), "No such process") && !strings.Contains(string(out), "not find") {
			return fmt.Errorf("launchctl bootout: %v: %s", err, strings.TrimSpace(string(out)))
		}
	case "linux":
		if out, err := exec.Command("systemctl", "--user", "disable", "--now", unitName).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl: %v: %s", err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func startService(in *Install) {
	if in.Service == "" {
		return
	}
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("launchctl", "bootstrap", "gui/"+strconv.Itoa(os.Getuid()), in.Service).Run()
	case "linux":
		_ = exec.Command("systemctl", "--user", "enable", "--now", unitName).Run()
	}
}

// waitReleased waits until no live process holds the store: wacli writes its
// pid into the lock file, and the old daemon interrupts it as it stops.
func waitReleased(ctx context.Context, storeDir string) error {
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		pid := lockHolder(storeDir)
		if pid == 0 || !platform.Alive(pid) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return errors.New("o WhatsApp da instalação antiga não parou a tempo; feche o que estiver usando ~/.wacli e tente de novo")
}

func lockHolder(storeDir string) int {
	raw, err := os.ReadFile(filepath.Join(storeDir, "LOCK"))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "pid="); ok {
			pid, _ := strconv.Atoi(v)
			return pid
		}
	}
	return 0
}

// removeIfEmpty clears the way for a move: a missing path or an empty folder
// is fine, anything else is refused.
func removeIfEmpty(path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("exists")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return errors.New("not empty")
	}
	return os.Remove(path)
}

// move renames, and copies when the two places are on different disks. After
// a copy, the original is first renamed aside, which is atomic on its own
// disk, so the old service can never find it again even if deleting it then
// fails; when it cannot even be renamed, the copy is undone and the move
// fails, leaving exactly one copy of the session.
func move(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyTree(src, dst); err != nil {
		os.RemoveAll(dst)
		return err
	}
	aside := src + ".moved-to-app-" + time.Now().Format("20060102-150405")
	if err := os.Rename(src, aside); err != nil {
		os.RemoveAll(dst)
		return fmt.Errorf("copied, but the original could not be set aside: %w", err)
	}
	_ = os.RemoveAll(aside) // best effort: under its new name nothing uses it
	return nil
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		switch {
		case info.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		case info.Mode().IsRegular():
			in, err := os.Open(path)
			if err != nil {
				return err
			}
			defer in.Close()
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, in); err != nil {
				out.Close()
				return err
			}
			return out.Close()
		}
		return nil // sockets and the like are recreated by wacli
	})
}
