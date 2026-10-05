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

// File is config.json's name inside the data folder.
const File = "config.json"

type Config struct {
	Port int `json:"port"`
	// CloseToTray keeps the app running in the tray when its window closes.
	CloseToTray bool `json:"close_to_tray"`
	// Autostart opens the app, in the tray only, when the user logs in.
	Autostart bool `json:"autostart"`
	// Seen records one-time notices already shown, such as "close" for the
	// one explaining that closing the window keeps the app in the tray.
	Seen []string `json:"seen,omitempty"`
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
	if c.Port < 1 || c.Port > 65535 {
		c.Port = DefaultPort
	}
	return c, nil
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
