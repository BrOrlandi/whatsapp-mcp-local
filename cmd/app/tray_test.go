package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// On Linux, Wails' Menu.Update calls GTK from the goroutine that asks, and
// GTK used off its thread brings the app down at random. Every update of the
// tray's menu goes through tray.refresh, which sends it to the main thread
// there; this keeps a new call site from calling Update directly.
func TestTrayMenuUpdatesGoThroughRefresh(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		inRefresh := false
		for i, line := range strings.Split(string(src), "\n") {
			if strings.HasPrefix(line, "func ") {
				inRefresh = strings.HasPrefix(line, "func (t *tray) refresh()")
			}
			if strings.Contains(line, ".Update()") || strings.Contains(line, ".Update)") {
				if !inRefresh {
					t.Errorf("%s:%d updates a menu outside tray.refresh: %s", f, i+1, strings.TrimSpace(line))
				}
			}
		}
	}
}
