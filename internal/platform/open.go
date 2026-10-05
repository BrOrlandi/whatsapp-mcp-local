package platform

import (
	"os/exec"
	"runtime"
	"strings"
)

// Open hands a URL or a folder to the system: the default browser, Finder,
// Explorer or the desktop's file manager.
func Open(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
		} else {
			cmd = exec.Command("explorer", target)
		}
	default:
		cmd = exec.Command("xdg-open", target)
	}
	Background(cmd)
	return cmd.Start()
}
