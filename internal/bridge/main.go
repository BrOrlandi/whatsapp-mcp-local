package bridge

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/appconfig"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/platform"
)

// Main is the app's bridge: what Claude Desktop starts, forwarding to the app
// on this computer at the port its config.json names, read again whenever
// the app does not answer. It returns the exit code.
func Main() int {
	dataDir, err := platform.DataDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "whatsapp-mcp-bridge:", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err = Run(ctx, Options{
		URL:   func() string { return "http://127.0.0.1:" + strconv.Itoa(appconfig.ResolvePort(dataDir)) + "/mcp" },
		Token: os.Getenv("WHATSAPP_MCP_TOKEN"),
		NotRunning: "WhatsApp MCP is not open on this computer. Ask the user to open the WhatsApp MCP app " +
			"(it keeps running in the menu bar or system tray once opened) and try again",
	}, os.Stdin, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "whatsapp-mcp-bridge:", err)
		return 1
	}
	return 0
}
