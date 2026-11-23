package panel

import (
	"path/filepath"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
)

// The line after saving the logs names the folder the file went to: without
// a Downloads folder the app saves it in the home folder, and says so.
func TestSavedNoteNamesTheRealFolder(t *testing.T) {
	defer i18n.Set(i18n.Source)
	i18n.Set(i18n.Source)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	name := "whatsapp-mcp-diagnostics-2026-11-22-1200.txt"
	if got, want := savedNote(filepath.Join(home, "Downloads", name)), "Salvo na pasta Downloads: "+name; got != want {
		t.Errorf("in Downloads: %q, want %q", got, want)
	}
	if got, want := savedNote(filepath.Join(home, name)), "Salvo na sua pasta pessoal: "+name; got != want {
		t.Errorf("in the home folder: %q, want %q", got, want)
	}
}
