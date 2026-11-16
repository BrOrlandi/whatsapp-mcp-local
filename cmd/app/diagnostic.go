package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/panel"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/platform"
)

var _ panel.DiagnosticSaver = (*App)(nil)

// SaveDiagnostic saves the diagnostic file in the Downloads folder and shows
// it in the file manager: the window cannot download it as a browser would.
func (a *App) SaveDiagnostic(name string, body []byte) (string, error) {
	dir := downloadsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, filepath.Base(name))
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return "", err
	}
	a.logger.Info("diagnostic file saved", "bytes", len(body))
	reveal(path)
	return path, nil
}

func downloadsDir() string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "linux" {
		if out, err := exec.Command("xdg-user-dir", "DOWNLOAD").Output(); err == nil {
			if dir := strings.TrimSpace(string(out)); dir != "" && dir != home {
				return dir
			}
		}
	}
	dir := filepath.Join(home, "Downloads")
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return dir
	}
	return home
}

// reveal shows a file selected in Finder or Explorer; elsewhere it opens its
// folder.
func reveal(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-R", path)
	case "windows":
		cmd = exec.Command("explorer", "/select,"+path)
	default:
		_ = platform.Open(filepath.Dir(path))
		return
	}
	platform.Background(cmd)
	_ = cmd.Start()
}
