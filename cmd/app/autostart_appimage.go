package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// xdgAutostart writes or removes the XDG autostart entry that opens an
// AppImage, in the tray only, when the user logs in.
func xdgAutostart(on bool, appImage string) error {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(home, ".config")
	}
	path := filepath.Join(dir, "autostart", bundleID+".desktop")
	if !on {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	exec := `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", "$", `\$`).Replace(appImage) + `" --hidden`
	entry := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=WhatsApp MCP\nExec=%s\nIcon=whatsapp-mcp\nTerminal=false\nX-GNOME-Autostart-enabled=true\n", exec)
	return os.WriteFile(path, []byte(entry), 0o644)
}
