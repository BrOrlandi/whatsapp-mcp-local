package updater

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeApp makes a signed .app whose program prints version.
func fakeApp(t *testing.T, dir, version string) string {
	t.Helper()
	app := filepath.Join(dir, "WhatsApp MCP.app")
	macos := filepath.Join(app, "Contents", "MacOS")
	if err := os.MkdirAll(macos, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "main.go")
	_ = os.WriteFile(src, []byte(`package main
import "fmt"
func main() { fmt.Print("`+version+`") }`), 0o644)
	if out, err := exec.Command("go", "build", "-o", filepath.Join(macos, "WhatsApp MCP"), src).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	_ = os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>CFBundleExecutable</key><string>WhatsApp MCP</string><key>CFBundleIdentifier</key><string>test.wamcp</string></dict></plist>`), 0o644)
	if out, err := exec.Command("codesign", "--force", "--sign", "-", app).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return app
}

func TestInstallDMGSwapsTheBundle(t *testing.T) {
	if _, err := exec.LookPath("hdiutil"); err != nil {
		t.Skip("needs hdiutil")
	}
	installed := fakeApp(t, t.TempDir(), "1.0.0")
	stage := t.TempDir()
	fakeApp(t, stage, "1.1.0")
	os.Remove(filepath.Join(stage, "main.go"))
	dmg := filepath.Join(t.TempDir(), "WhatsApp-MCP.dmg")
	if out, err := exec.Command("hdiutil", "create", "-quiet", "-volname", "WhatsApp MCP", "-srcfolder", stage, "-format", "UDZO", dmg).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}

	var script string
	var env map[string]string
	old := relaunch
	relaunch = func(s string, e map[string]string) error { script, env = s, e; return nil }
	defer func() { relaunch = old }()

	if err := installDMG(context.Background(), dmg, installed); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(filepath.Join(installed, "Contents", "MacOS", "WhatsApp MCP")).Output()
	if err != nil || string(out) != "1.1.0" {
		t.Fatalf("the bundle in place runs %q (%v), want the new version", out, err)
	}
	if _, err := os.Stat(installed + ".old"); err != nil {
		t.Fatal("the old bundle must wait beside the new one until the app quits")
	}
	if !strings.Contains(script, "open") || env["TARGET"] != installed || env["OLD"] != installed+".old" {
		t.Fatalf("relaunch: %q %v", script, env)
	}
}

func TestInstallDMGRefusesABrokenSignature(t *testing.T) {
	if _, err := exec.LookPath("hdiutil"); err != nil {
		t.Skip("needs hdiutil")
	}
	installed := fakeApp(t, t.TempDir(), "1.0.0")
	stage := t.TempDir()
	app := fakeApp(t, stage, "1.1.0")
	os.Remove(filepath.Join(stage, "main.go"))
	// Changed after signing: the seal no longer matches.
	_ = os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte("<plist/>"), 0o644)
	dmg := filepath.Join(t.TempDir(), "WhatsApp-MCP.dmg")
	if out, err := exec.Command("hdiutil", "create", "-quiet", "-volname", "WhatsApp MCP", "-srcfolder", stage, "-format", "UDZO", dmg).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	old := relaunch
	relaunch = func(string, map[string]string) error { t.Fatal("must not relaunch"); return nil }
	defer func() { relaunch = old }()
	if err := installDMG(context.Background(), dmg, installed); err == nil {
		t.Fatal("a bundle with a broken signature must be refused")
	}
	if out, _ := exec.Command(filepath.Join(installed, "Contents", "MacOS", "WhatsApp MCP")).Output(); string(out) != "1.0.0" {
		t.Fatal("a refused update must leave the installed app alone")
	}
}
