package wacli

import (
	"os"
	"runtime"
	"strings"
)

// env is the environment every wacli run gets: its store, no colours, and
// the name this computer shows in the phone's list of linked devices.
func (c *CLI) env() []string {
	return append(os.Environ(), "WACLI_STORE_DIR="+c.StoreDir, "NO_COLOR=1", "WACLI_DEVICE_LABEL="+DeviceLabel())
}

// DeviceLabel is the linked device's name, as wacli would build it but
// under the app's own name: "WhatsApp MCP - macOS (Brunos-MacBook)". The
// phone keeps the name given when the number was paired, so it only shows
// on numbers paired from now on.
func DeviceLabel() string {
	host, _ := os.Hostname()
	return deviceLabel(osName(runtime.GOOS, os.ReadFile), strings.TrimSpace(host))
}

func deviceLabel(osName, host string) string {
	label := "WhatsApp MCP"
	if osName != "" {
		label += " - " + osName
	}
	if host != "" {
		if osName != "" {
			label += " (" + host + ")"
		} else {
			label += " - " + host
		}
	}
	return label
}

func osName(goos string, readFile func(string) ([]byte, error)) string {
	switch goos {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		if data, err := readFile("/etc/os-release"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if key, value, ok := strings.Cut(line, "="); ok && key == "PRETTY_NAME" {
					if value = strings.Trim(strings.TrimSpace(value), `"`); value != "" {
						return value
					}
				}
			}
		}
		return "Linux"
	default:
		return goos
	}
}
