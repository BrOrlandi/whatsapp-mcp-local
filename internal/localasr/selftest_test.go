package localasr

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/sidecar"
)

// windowsTest turns on the engine test that only Windows runs, with fake
// programs whose -h exits with $FAKE_H_EXIT (0 by default), and whose
// transcription fails the same way when FAKE_H_EXIT is not 0: an engine
// that cannot start fails at everything.
func windowsTest(t *testing.T, exit string) {
	t.Helper()
	fakeTools(t)
	old := systemTools()
	whisper := filepath.Join(t.TempDir(), "whisper-cli")
	body, err := os.ReadFile(old.Whisper)
	if err != nil {
		t.Fatal(err)
	}
	gate := "[ -n \"$FAKE_H_COUNT\" ] && [ \"$1\" = -h ] && printf x >> \"$FAKE_H_COUNT\"\ncode=${FAKE_H_EXIT:-0}\nif [ -n \"$FAKE_H_SLEEP\" ]; then exec sleep \"$FAKE_H_SLEEP\"; fi\n[ \"$code\" != 0 ] && exit \"$code\"\n[ \"$1\" = -h ] && { echo usage; exit 0; }\n"
	script := strings.Replace(string(body), "#!/bin/sh\n", "#!/bin/sh\n"+gate, 1)
	if err := os.WriteFile(whisper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	prev := systemTools
	systemTools = func() sidecar.Tools { return sidecar.Tools{Whisper: whisper, FFmpeg: old.FFmpeg, Accel: "cpu"} }
	t.Cleanup(func() { systemTools = prev })
	on := selfTestOn
	selfTestOn = true
	t.Cleanup(func() { selfTestOn = on })
	t.Setenv("FAKE_H_EXIT", exit)
}

// modelServer stands in for Hugging Face and counts the requests.
func modelServer(t *testing.T) *atomic.Int32 {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	old := modelURL
	modelURL = srv.URL + "/" + ModelName
	t.Cleanup(func() { modelURL = old })
	return &hits
}

// On a Windows without the Visual C++ runtime the engine is installed but
// does not start: it is not ready, it says why, and nothing offers it.
func TestEngineThatDoesNotStartIsUnavailable(t *testing.T) {
	windowsTest(t, "1")
	e := engine(t, true)
	st := e.Status()
	if st.Ready || st.Unavailable != UnavailableEngine || !st.Supported || len(st.Missing) != 0 {
		t.Fatalf("status %+v, want supported, not ready, unavailable", st)
	}
	if e.Available() || e.Backend() != "none" {
		t.Errorf("available %v, backend %q", e.Available(), e.Backend())
	}
	_, err := e.Transcribe(context.Background(), audioFile(t), "", "")
	if !errors.Is(err, ErrUnavailable) || strings.TrimSpace(strings.TrimPrefix(err.Error(), ErrUnavailable.Error()+":")) == "" {
		t.Errorf("Transcribe: %v", err)
	}
}

// The engine package is tested before the 574 MB model is fetched.
func TestInstallStopsBeforeTheModelWhenTheEngineDoesNotStart(t *testing.T) {
	windowsTest(t, "1")
	hits := modelServer(t)
	e := engine(t, false)
	err := e.installAll(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("install: %v, want ErrUnavailable", err)
	}
	if hits.Load() != 0 {
		t.Errorf("the model was requested %d times", hits.Load())
	}
}

// Where the engine starts, nothing changes: it is ready, and the install
// goes on to the model.
func TestEngineThatStartsStaysReady(t *testing.T) {
	windowsTest(t, "0")
	e := engine(t, true)
	if st := e.Status(); !st.Ready || st.Unavailable != "" {
		t.Fatalf("status %+v, want ready", st)
	}
	text, err := e.Transcribe(context.Background(), audioFile(t), "pt", "")
	if err != nil || !strings.Contains(text, "heard [pt]") {
		t.Errorf("Transcribe: %q, %v", text, err)
	}
	hits := modelServer(t)
	if err := engine(t, false).installAll(context.Background()); err == nil || hits.Load() != 1 {
		t.Errorf("install without the model: %v, %d requests (want the model asked for once)", err, hits.Load())
	}
}

// A failed test is kept, so the status does not run the engine every time,
// until the settings open (Recheck): installing the runtime fixes it.
func TestRecheckSeesTheRuntimeInstalled(t *testing.T) {
	windowsTest(t, "1")
	e := engine(t, true)
	if e.Status().Ready {
		t.Fatal("ready with an engine that does not start")
	}
	t.Setenv("FAKE_H_EXIT", "0")
	if e.Status().Ready {
		t.Error("the failed test was not kept")
	}
	e.Recheck()
	if st := e.Status(); !st.Ready {
		t.Errorf("after Recheck: %+v", st)
	}
}

// A test that takes too long says nothing: the engine keeps counting as
// ready, so a slow first start never turns a working install off.
func TestSlowEngineTestKeepsItReady(t *testing.T) {
	windowsTest(t, "0")
	t.Setenv("FAKE_H_SLEEP", "5")
	old := selfTestTimeout
	selfTestTimeout = 100 * time.Millisecond
	t.Cleanup(func() { selfTestTimeout = old })
	start := time.Now()
	if st := engine(t, true).Status(); !st.Ready {
		t.Errorf("status %+v, want ready", st)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("the test took %s", d)
	}
}

// A test that took too long is believed for a while, so a slow computer
// does not wait for it on every status; a passing one survives Recheck.
func TestEngineTestIsNotRunEveryTime(t *testing.T) {
	windowsTest(t, "0")
	count := filepath.Join(t.TempDir(), "runs")
	t.Setenv("FAKE_H_COUNT", count)
	e := engine(t, true)
	for range 3 {
		e.Status()
	}
	e.Recheck()
	e.Status()
	if b, _ := os.ReadFile(count); len(b) != 1 {
		t.Errorf("a passing engine was run %d times, want 1", len(b))
	}
}

// A test that took too long survives Recheck for its minute: opening the
// settings on a slow computer does not run the engine again, and does not
// hold every status for the test's seconds.
func TestRecheckKeepsASlowTest(t *testing.T) {
	windowsTest(t, "0")
	t.Setenv("FAKE_H_SLEEP", "5")
	count := filepath.Join(t.TempDir(), "runs")
	t.Setenv("FAKE_H_COUNT", count)
	old := selfTestTimeout
	selfTestTimeout = time.Second
	t.Cleanup(func() { selfTestTimeout = old })
	e := engine(t, true)
	e.Status()
	e.Recheck()
	e.Status()
	if b, _ := os.ReadFile(count); len(b) != 1 {
		t.Errorf("a slow engine was run %d times across Recheck, want 1", len(b))
	}
}

// The transcription server says the engine cannot run, instead of ready.
func TestServerSaysTheEngineIsUnavailable(t *testing.T) {
	windowsTest(t, "1")
	srv := httptest.NewServer(engine(t, true).Handler(token, "test"))
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/health", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var h map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&h)
	resp.Body.Close()
	if h["ready"] != false || h["unavailable"] != UnavailableEngine {
		t.Errorf("health %v", h)
	}
	// And a client pointed at it says why.
	client := engine(t, false)
	if err := client.Configure(Change{Mode: ModeRemote, URL: srv.URL, Token: token}); err != nil {
		t.Fatal(err)
	}
	if rs := client.RemoteStatus(context.Background(), true); rs.Ready || rs.Reason != "unavailable" {
		t.Errorf("remote status %+v", rs)
	}
}

// Windows gives the NTSTATUS as the exit code: 0xC0000135 is a missing
// library, however the int holds it.
func TestDLLNotFound(t *testing.T) {
	for _, code := range []int{-1073741515, 3221225781} {
		if !isDLLNotFound(code) {
			t.Errorf("%d is 0xC0000135", code)
		}
	}
	for _, code := range []int{0, 1, -1, 255} {
		if isDLLNotFound(code) {
			t.Errorf("%d is not 0xC0000135", code)
		}
	}
	if err := unavailableError(UnavailableWindowsRuntime); !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "Visual C++") {
		t.Errorf("%v", err)
	}
}
