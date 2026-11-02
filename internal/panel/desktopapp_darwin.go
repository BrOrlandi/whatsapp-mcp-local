package panel

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

const desktopControl = true

const desktopBundle = "com.anthropic.claudefordesktop"

func desktopRunning() bool {
	out, err := exec.Command("osascript", "-e", `application id "`+desktopBundle+`" is running`).Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// quitDesktop asks Claude Desktop to quit, as its menu would.
func quitDesktop(ctx context.Context) error {
	if err := exec.CommandContext(ctx, "osascript", "-e", `tell application id "`+desktopBundle+`" to quit`).Run(); err != nil {
		return err
	}
	return waitDesktopClosed(ctx, 20*time.Second)
}

func desktopLaunchTarget() string { return desktopBundle }

func openDesktop(bundle string) error {
	return exec.Command("open", "-b", bundle).Run()
}
