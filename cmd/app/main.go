// Command app is WhatsApp MCP as a desktop app: one window with the control
// panel, an icon in the tray, and the gateway running inside the app's own
// process for as long as it is open.
package main

import (
	"fmt"
	"os"
	"slices"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/bridge"
)

var version = "dev"

func main() {
	// Inside an AppImage the bridge has no fixed path of its own, so Claude
	// Desktop starts the AppImage itself with this argument. Linux gives a
	// windowed program its stdio as any other.
	if len(os.Args) > 1 && os.Args[1] == "bridge" {
		os.Exit(bridge.Main())
	}
	a, err := newApp(slices.Contains(os.Args[1:], "--hidden"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "WhatsApp MCP:", err)
		os.Exit(1)
	}
	if err := a.run(); err != nil {
		a.logger.Error("the app stopped", "error", err)
		os.Exit(1)
	}
}
