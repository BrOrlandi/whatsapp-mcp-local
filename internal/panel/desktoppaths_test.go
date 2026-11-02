package panel

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Claude 4's Claude-Data comes first, then the package's virtualised copy,
// then the classic file; every file that exists is written; with no file,
// the newest folder that exists decides.
func TestWindowsDesktopPaths(t *testing.T) {
	const file = "claude_desktop_config.json"
	dir := t.TempDir()
	data := filepath.Join(dir, "Local", "Claude-Data", "claude_x")
	store := filepath.Join(dir, "Packages", "Claude_x", "LocalCache", "Roaming", "Claude")
	classic := filepath.Join(dir, "Roaming", "Claude")
	mkdir := func(d string) {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	touch := func(d string) {
		mkdir(d)
		if err := os.WriteFile(filepath.Join(d, file), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	check := func(data, store string, want ...string) {
		t.Helper()
		if got := windowsDesktopPaths(file, data, store, classic); !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	check("", "", filepath.Join(classic, file))
	mkdir(store)
	check("", store, filepath.Join(store, file))
	touch(classic)
	check("", store, filepath.Join(classic, file))
	touch(store)
	check("", store, filepath.Join(store, file), filepath.Join(classic, file))
	mkdir(data)
	check(data, store, filepath.Join(data, file), filepath.Join(store, file), filepath.Join(classic, file))
	touch(data)
	check(data, store, filepath.Join(data, file), filepath.Join(store, file), filepath.Join(classic, file))
}
