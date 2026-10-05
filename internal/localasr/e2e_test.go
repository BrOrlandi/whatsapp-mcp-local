package localasr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/sidecar"
)

// TestInstallAndTranscribe installs transcription from a set of archives
// built with scripts/sidecars and transcribes a voice note. It downloads and
// runs real programs, so it only runs when asked:
//
//	WAMCP_E2E_PACKAGES=<dir with the archives and manifest.json>, or
//	                   "published" for the archives the app's manifest pins
//	WAMCP_E2E_AUDIO=<an Ogg/Opus voice note>  WAMCP_E2E_EXPECT=<a word it says>
//	WAMCP_E2E_MODEL=<an already downloaded model, to skip 574 MB>
func TestInstallAndTranscribe(t *testing.T) {
	dir := os.Getenv("WAMCP_E2E_PACKAGES")
	if dir == "" {
		t.Skip("set WAMCP_E2E_PACKAGES to run")
	}
	if dir != "published" {
		srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
		defer srv.Close()
		raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		manifest := filepath.Join(t.TempDir(), "manifest.json")
		if err := os.WriteFile(manifest, []byte(strings.ReplaceAll(string(raw), "BASE", srv.URL)), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("WHATSAPP_MCP_SIDECARS", manifest)
	}
	// Only what this installs may be used.
	old := systemTools
	systemTools = func() sidecar.Tools { return sidecar.Tools{} }
	defer func() { systemTools = old }()

	data := t.TempDir()
	if model := os.Getenv("WAMCP_E2E_MODEL"); model != "" {
		_ = os.MkdirAll(filepath.Join(data, "models"), 0o755)
		if err := os.Symlink(model, filepath.Join(data, "models", ModelName)); err != nil {
			t.Fatal(err)
		}
	}
	e := New(data)
	if st := e.Status(); !st.Supported || st.Ready {
		t.Fatalf("before install: %+v", st)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	if err := e.installAll(ctx); err != nil {
		t.Fatal(err)
	}
	st := e.Status()
	if !st.Ready || !strings.HasPrefix(st.Whisper, data) {
		t.Fatalf("after install: %+v", st)
	}
	start := time.Now()
	text, err := e.Transcribe(ctx, os.Getenv("WAMCP_E2E_AUDIO"), "pt", "Conversa com Bruno sobre a Birdie")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s transcribed in %s (%s): %s", filepath.Base(os.Getenv("WAMCP_E2E_AUDIO")), time.Since(start).Round(time.Millisecond), st.Accel, text)
	if want := os.Getenv("WAMCP_E2E_EXPECT"); want != "" && !strings.Contains(text, want) {
		t.Fatalf("transcript lacks %q", want)
	}

	// A program changed after install is refused.
	if err := os.WriteFile(st.FFmpeg, []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	if st := e.Status(); st.Ready || st.Install.State != "error" {
		t.Fatalf("a tampered program must not be used: %+v", st)
	}
}
