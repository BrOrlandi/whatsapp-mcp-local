// Package service installs the daemon as a per-user background service, so it
// starts at login and restarts if it dies: launchd on macOS, systemd --user on
// Linux. A daemon that is only up while a terminal is open is a daemon whose
// index has holes.
package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const label = "com.brorlandi.whatsapp-mcp-v2"

type Spec struct {
	Binary string            // absolute path of this program
	Args   []string          // arguments after the binary, e.g. serve
	Env    map[string]string // environment the daemon needs, e.g. WACLI_BIN
	LogDir string
}

func Install(spec Spec) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return installLaunchd(spec)
	case "linux":
		return installSystemd(spec)
	}
	return "", fmt.Errorf("installing a service is supported on macOS and Linux; on %s run `whatsapp-mcp-v2 serve` yourself", runtime.GOOS)
}

func Uninstall() error {
	switch runtime.GOOS {
	case "darwin":
		plist := launchdPath()
		_ = exec.Command("launchctl", "bootout", "gui/"+strconv.Itoa(os.Getuid()), plist).Run()
		if err := os.Remove(plist); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	case "linux":
		_ = exec.Command("systemctl", "--user", "disable", "--now", "whatsapp-mcp-v2.service").Run()
		if err := os.Remove(systemdPath()); err != nil && !os.IsNotExist(err) {
			return err
		}
		return exec.Command("systemctl", "--user", "daemon-reload").Run()
	}
	return fmt.Errorf("no service support on %s", runtime.GOOS)
}

func launchdPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist")
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

func installLaunchd(spec Spec) (string, error) {
	var args strings.Builder
	for _, a := range append([]string{spec.Binary}, spec.Args...) {
		fmt.Fprintf(&args, "\t\t<string>%s</string>\n", xmlEscape(a))
	}
	var env strings.Builder
	for k, v := range spec.Env {
		fmt.Fprintf(&env, "\t\t<key>%s</key>\n\t\t<string>%s</string>\n", xmlEscape(k), xmlEscape(v))
	}
	log := filepath.Join(spec.LogDir, "whatsapp-mcp-v2.log")
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
%s	</array>
	<key>EnvironmentVariables</key>
	<dict>
%s	</dict>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ThrottleInterval</key>
	<integer>10</integer>
	<key>ProcessType</key>
	<string>Background</string>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, label, args.String(), env.String(), xmlEscape(log), xmlEscape(log))

	path := launchdPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(spec.LogDir, 0o755); err != nil {
		return "", err
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	_ = exec.Command("launchctl", "bootout", domain, path).Run()
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return "", err
	}
	if out, err := exec.Command("launchctl", "bootstrap", domain, path).CombinedOutput(); err != nil {
		return "", fmt.Errorf("launchctl bootstrap: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return path, nil
}

func systemdPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", "whatsapp-mcp-v2.service")
}

func installSystemd(spec Spec) (string, error) {
	quoted := make([]string, 0, len(spec.Args)+1)
	for _, a := range append([]string{spec.Binary}, spec.Args...) {
		quoted = append(quoted, strconv.Quote(a))
	}
	var env strings.Builder
	for k, v := range spec.Env {
		fmt.Fprintf(&env, "Environment=%s\n", strconv.Quote(k+"="+v))
	}
	unit := fmt.Sprintf(`[Unit]
Description=WhatsApp MCP (wacli) on localhost

[Service]
ExecStart=%s
%sRestart=always
RestartSec=5

[Install]
WantedBy=default.target
`, strings.Join(quoted, " "), env.String())
	path := systemdPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(unit), 0o644); err != nil {
		return "", err
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", "--now", "whatsapp-mcp-v2.service"}} {
		if out, err := exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput(); err != nil {
			return "", fmt.Errorf("systemctl --user %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	return path, nil
}

// Stop stops the installed service without removing it: the daemon and its
// sync end, as they would with the computer off, until Start.
func Stop() error {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("launchctl", "bootout", "gui/"+strconv.Itoa(os.Getuid()), launchdPath()).CombinedOutput()
		if err != nil && !strings.Contains(string(out), "No such process") {
			return fmt.Errorf("launchctl bootout: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	case "linux":
		return exec.Command("systemctl", "--user", "stop", "whatsapp-mcp-v2.service").Run()
	}
	return fmt.Errorf("no service support on %s", runtime.GOOS)
}

// Start starts the installed service again.
func Start() error {
	switch runtime.GOOS {
	case "darwin":
		if _, err := os.Stat(launchdPath()); err != nil {
			return fmt.Errorf("the service is not installed; run whatsapp-mcp-v2 service install")
		}
		out, err := exec.Command("launchctl", "bootstrap", "gui/"+strconv.Itoa(os.Getuid()), launchdPath()).CombinedOutput()
		if err != nil && !strings.Contains(string(out), "already") {
			return fmt.Errorf("launchctl bootstrap: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	case "linux":
		return exec.Command("systemctl", "--user", "start", "whatsapp-mcp-v2.service").Run()
	}
	return fmt.Errorf("no service support on %s", runtime.GOOS)
}
