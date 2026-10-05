// Command app is WhatsApp MCP as a desktop app: one window with the control
// panel, an icon in the tray, and the gateway running inside the app's own
// process for as long as it is open.
package main

import (
	"fmt"
	"os"
	"slices"
)

var version = "dev"

func main() {
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
