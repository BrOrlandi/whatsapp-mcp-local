// Package appconfig is the app's config.json: what must be known before
// anything else starts, and what the stdio bridge reads to find the MCP. The
// daemon's own state (transcripts, clients, setup) stays in state.db.
package appconfig

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
)

// DefaultPort is uncommon on purpose: the MCP should not collide with the dev
// servers that live on 3000, 5173 or 8080.
const DefaultPort = 47821

// LanguageBefore is the language of a config.json written before the app
// had languages, which spoke only Brazilian Portuguese.
const LanguageBefore = "pt-BR"

// File is config.json's name inside the data folder.
const File = "config.json"

type Config struct {
	Port int `json:"port"`
	// CloseToTray keeps the app running in the tray when its window closes.
	CloseToTray bool `json:"close_to_tray"`
	// Autostart opens the app, in the tray only, when the user logs in.
	Autostart bool `json:"autostart"`
	// Language is the language the app shows (pt-BR, es, en); empty follows
	// the system's. It is always written, empty too, so a config.json
	// without it is one from before the app had languages (see Load).
	Language string `json:"language"`
	// Seen records one-time notices already shown, such as "close" for the
	// one explaining that closing the window keeps the app in the tray.
	Seen []string `json:"seen,omitempty"`
	// Remote, when set, makes the app the remote control of the daemon on a
	// server (internal/remote) instead of running WhatsApp here. The
	// device's private key is not here: it is in the system's keychain.
	Remote *Remote `json:"remote,omitempty"`
}

// Remote is the server the app uses and how it knows it.
type Remote struct {
	Server    string `json:"server"`
	ServerKey []byte `json:"server_key"`
	DeviceID  string `json:"device_id"`
	Scope     string `json:"scope"`
}

// Defaults is the configuration of a first run.
func Defaults() Config {
	return Config{Port: DefaultPort, CloseToTray: true, Autostart: true}
}

func Path(dataDir string) string { return filepath.Join(dataDir, File) }

// Load reads config.json, falling back to the defaults for a missing file
// and for any field it does not set.
func Load(dataDir string) (Config, error) {
	c := Defaults()
	raw, err := os.ReadFile(Path(dataDir))
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return Defaults(), err
	}
	// A config.json from before the app had languages: the person has been
	// using it in Portuguese, and an update does not change that, even on a
	// computer set to English. A new install follows the system's language.
	var keys map[string]json.RawMessage
	if json.Unmarshal(raw, &keys) == nil {
		if _, ok := keys["language"]; !ok {
			c.Language = LanguageBefore
		}
	}
	if c.Port < 1 || c.Port > 65535 {
		c.Port = DefaultPort
	}
	return c, nil
}

// Chosen is the language kept in a data folder: "language" in config.json,
// empty to follow the system's. A data folder in use from before the app had
// languages, with no config.json (the command line wrote one only when a
// setting changed; now MarkNew writes it on a folder's first run), stays in
// Portuguese, as the person has been using it.
func Chosen(dataDir string) string {
	if _, err := os.Stat(Path(dataDir)); err == nil {
		c, _ := Load(dataDir)
		return c.Language
	}
	if _, err := os.Stat(filepath.Join(dataDir, "state.db")); err == nil {
		return LanguageBefore
	}
	return ""
}

// MarkNew gives a data folder about to be used for the first time a
// config.json with the defaults, so later runs know it is from after the
// languages and keep following the system's (Chosen). A folder already in use
// is left alone.
func MarkNew(dataDir string) error {
	for _, name := range []string{File, "state.db"} {
		if _, err := os.Stat(filepath.Join(dataDir, name)); !errors.Is(err, os.ErrNotExist) {
			return nil
		}
	}
	return Save(dataDir, Defaults())
}

// Save writes config.json atomically: a temporary file renamed over the old
// one, so the bridge never reads half a file.
func Save(dataDir string, c Config) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dataDir, ".config-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(body, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), Path(dataDir)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// HasSeen reports whether a one-time notice was shown.
func (c Config) HasSeen(notice string) bool {
	for _, s := range c.Seen {
		if s == notice {
			return true
		}
	}
	return false
}

// PortFromEnv is the port WHATSAPP_MCP_PORT sets, which wins over
// config.json; ok is false when the variable is not set.
func PortFromEnv() (port int, ok bool, err error) {
	v := os.Getenv("WHATSAPP_MCP_PORT")
	if v == "" {
		return 0, false, nil
	}
	p, err := strconv.Atoi(v)
	if err != nil || p < 1 || p > 65535 {
		return 0, true, errors.New("WHATSAPP_MCP_PORT must be a port number, got " + strconv.Quote(v))
	}
	return p, true, nil
}

// ResolvePort is the port the MCP listens on: the environment first, then
// config.json, then the default.
func ResolvePort(dataDir string) int {
	if p, ok, err := PortFromEnv(); ok && err == nil {
		return p
	}
	if c, err := Load(dataDir); err == nil {
		return c.Port
	}
	return DefaultPort
}
