// Package platform holds what differs between macOS, Windows and Linux: where
// the app keeps its files, how it starts and stops the programs it runs, and
// how it opens things in the system.
package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// AppName is the product name, as folders and menus show it.
const AppName = "WhatsApp MCP"

// DataDir is where the app keeps everything it owns: wacli's store, its own
// state, the transcription model and binaries, and downloaded media.
//
//	macOS:   ~/Library/Application Support/WhatsApp MCP
//	Windows: %LOCALAPPDATA%\WhatsApp MCP
//	Linux:   $XDG_DATA_HOME/whatsapp-mcp (~/.local/share/whatsapp-mcp)
//
// WHATSAPP_MCP_DATA overrides it, as it does for the command line.
func DataDir() (string, error) {
	if v := os.Getenv("WHATSAPP_MCP_DATA"); v != "" {
		return v, nil
	}
	return dataDir()
}

// LogDir is where the app writes its log.
func LogDir() (string, error) {
	if v := os.Getenv("WHATSAPP_MCP_DATA"); v != "" {
		return filepath.Join(v, "logs"), nil
	}
	return logDir()
}

// Executable is this program's resolved path.
func Executable() string {
	bin, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(bin); err == nil {
		return resolved
	}
	return bin
}

// ExeName adds the platform's executable suffix to a program name.
func ExeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// Bundled finds a program shipped next to this one: inside the .app on macOS
// (Contents/Resources/bin), in bin\ beside the .exe on Windows, and beside
// the binary on Linux. It returns "" when the program is not there.
func Bundled(name string) string {
	exe := Executable()
	if exe == "" {
		return ""
	}
	dir := filepath.Dir(exe)
	name = ExeName(name)
	for _, p := range []string{
		filepath.Join(dir, "..", "Resources", "bin", name),
		filepath.Join(dir, "bin", name),
		filepath.Join(dir, name),
	} {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return filepath.Clean(p)
		}
	}
	return ""
}

// LookPath finds a program on PATH and in the places a package manager puts
// it, because a program started at login gets a PATH without them.
func LookPath(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
		return p
	}
	home, _ := os.UserHomeDir()
	for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin", "/home/linuxbrew/.linuxbrew/bin", filepath.Join(home, ".local", "bin")} {
		p := filepath.Join(dir, ExeName(name))
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}
