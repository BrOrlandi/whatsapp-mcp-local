// Command whatsapp-mcp-bridge is the stdio MCP server Claude Desktop starts.
// It forwards every request to the WhatsApp MCP app on this computer, finding
// its port in the app's config.json each time, so a port changed in the app
// needs nothing done in Claude Desktop.
//
// It is a program of its own, not the app with an argument, because a
// Windows program built to open windows has no reliable stdio.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/appconfig"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/bridge"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/platform"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println(version)
		return
	}
	dataDir, err := platform.DataDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "whatsapp-mcp-bridge:", err)
		os.Exit(1)
	}
	url := func() string {
		return "http://127.0.0.1:" + strconv.Itoa(appconfig.ResolvePort(dataDir)) + "/mcp"
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err = bridge.Run(ctx, bridge.Options{
		URL:   url,
		Token: os.Getenv("WHATSAPP_MCP_TOKEN"),
		NotRunning: "WhatsApp MCP is not open on this computer. Ask the user to open the WhatsApp MCP app " +
			"(it keeps running in the menu bar or system tray once opened) and try again",
	}, os.Stdin, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "whatsapp-mcp-bridge:", err)
		os.Exit(1)
	}
}
