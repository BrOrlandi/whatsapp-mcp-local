// Command whatsapp-mcp-bridge is the stdio MCP server Claude Desktop starts.
// It forwards every request to the WhatsApp MCP app on this computer, finding
// its port in the app's config.json each time, so a port changed in the app
// needs nothing done in Claude Desktop.
//
// It is a program of its own, not the app with an argument, because a
// Windows program built to open windows has no reliable stdio.
package main

import (
	"fmt"
	"os"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/bridge"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println(version)
		return
	}
	os.Exit(bridge.Main())
}
