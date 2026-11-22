package localasr

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/appconfig"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/sidecar"
)

// fakeTools stands in for ffmpeg and whisper-cli: the "transcript" says the
// language and the prompt whisper was given, so a test sees them arrive.
func fakeTools(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake programs are shell scripts")
	}
	dir := t.TempDir()
	ffmpeg := filepath.Join(dir, "ffmpeg")
	whisper := filepath.Join(dir, "whisper-cli")
	write := func(path, body string) {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// The last argument is the WAV file to write.
	write(ffmpeg, `for a; do last=$a; done; : > "$last"`+"\n")
	write(whisper, `while [ $# -gt 0 ]; do case "$1" in -of) of=$2; shift;; -l) l=$2; shift;; --prompt) p=$2; shift;; esac; shift; done
if [ -n "$FAKE_WHISPER_SLEEP" ]; then sleep "$FAKE_WHISPER_SLEEP"; fi
printf 'heard [%s] [%s]\n[BLANK_AUDIO]\n' "$l" "$p" > "$of.txt"
`)
	old := systemTools
	systemTools = func() sidecar.Tools { return sidecar.Tools{Whisper: whisper, FFmpeg: ffmpeg, Accel: "cpu"} }
	t.Cleanup(func() { systemTools = old })
	for _, v := range []string{EnvMode, EnvRemoteURL, EnvRemoteToken, EnvModel, EnvOpenAIKey, "OPENAI_API_KEY"} {
		t.Setenv(v, "")
	}
}

// engine is an Engine in a temporary data folder, with the model in place
// (a sparse file) when withModel is set.
func engine(t *testing.T, withModel bool) *Engine {
	t.Helper()
	data := t.TempDir()
	if withModel {
		if err := os.MkdirAll(filepath.Join(data, "models"), 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(data, "models", ModelName))
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(modelSize); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	return New(data)
}

func audioFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "note.ogg")
	if err := os.WriteFile(p, []byte("OggS fake voice note"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const token = "a-token-of-sixteen-or-more"

// transcriptionServer runs a transcription server (Handler) on its own
// engine, with the model in place.
func transcriptionServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(engine(t, true).Handler(token, "test"))
	t.Cleanup(srv.Close)
	return srv
}

func TestRemoteTranscribesWithContextAndLanguage(t *testing.T) {
	fakeTools(t)
	srv := transcriptionServer(t)
	e := engine(t, false) // nothing installed here: only the server can do it
	if e.Available() || e.Backend() != "none" {
		t.Fatalf("before: available %v, backend %s", e.Available(), e.Backend())
	}
	if err := e.SetRemote(srv.URL+"/", token); err != nil {
		t.Fatal(err)
	}
	if !e.Available() || e.Backend() != "remote" {
		t.Fatalf("after: available %v, backend %s", e.Available(), e.Backend())
	}
	r, err := e.TranscribeAny(context.Background(), audioFile(t), "pt", "Conversa com Ana sobre a Birdie")
	if err != nil {
		t.Fatal(err)
	}
	if r.Backend != "remote" || !strings.HasPrefix(r.Model, "whisper.cpp large-v3-turbo (transcription server 127.0.0.1:") || r.Fallback != "" {
		t.Errorf("result = %+v", r)
	}
	if r.Text != "heard [pt] [Conversa com Ana sobre a Birdie]" {
		t.Errorf("text = %q: the language and the context must reach whisper on the server", r.Text)
	}
	st := e.RemoteStatus(context.Background(), true)
	if !st.Configured || !st.Reachable || !st.Ready || st.Accel != "cpu" || st.Version != "test" || st.Mode != ModeRemote || st.Model != DefaultModel {
		t.Errorf("status = %+v", st)
	}
}

func TestRemoteNeedsTheToken(t *testing.T) {
	fakeTools(t)
	srv := transcriptionServer(t)
	for _, auth := range []string{"", "Bearer wrong-token-of-sixteen", "Bearer " + token + "x"} {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/health", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%q: %s", auth, resp.Status)
		}
	}
	e := engine(t, false)
	if err := e.SetRemote(srv.URL, "wrong-token-of-sixteen"); err != nil {
		t.Fatal(err)
	}
	st := e.RemoteStatus(context.Background(), true)
	if st.Reachable || !strings.Contains(st.Error, "token") {
		t.Errorf("status = %+v", st)
	}
	// Without transcription here, the error says what failed.
	if _, err := e.TranscribeAny(context.Background(), audioFile(t), "", ""); err == nil || !strings.Contains(err.Error(), "token") {
		t.Errorf("err = %v", err)
	}
}

func TestFallsBackToThisComputer(t *testing.T) {
	fakeTools(t)
	// A port nobody listens on: the connection is refused at once.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := "http://" + ln.Addr().String()
	ln.Close()

	e := engine(t, true)
	if err := e.SetRemote(dead, token); err != nil {
		t.Fatal(err)
	}
	if e.Backend() != "remote+local" {
		t.Errorf("backend = %s", e.Backend())
	}
	start := time.Now()
	r, err := e.TranscribeAny(context.Background(), audioFile(t), "en", "ctx")
	if err != nil {
		t.Fatal(err)
	}
	if r.Backend != "local" || r.Model != Label || !strings.Contains(r.Fallback, "did not answer") || r.Text != "heard [en] [ctx]" {
		t.Errorf("result = %+v", r)
	}
	if d := time.Since(start); d > dialTimeout+2*time.Second {
		t.Errorf("the fallback took %s", d)
	}
	// The failure shows in the status until the next check.
	if st := e.RemoteStatus(context.Background(), false); st.Reachable || st.Error == "" {
		t.Errorf("status = %+v", st)
	}
}

func TestFallsBackWhenTheServerFails(t *testing.T) {
	fakeTools(t)
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "whisper.cpp failed"})
	}))
	defer broken.Close()
	e := engine(t, true)
	if err := e.SetRemote(broken.URL, token); err != nil {
		t.Fatal(err)
	}
	r, err := e.TranscribeAny(context.Background(), audioFile(t), "", "")
	if err != nil || r.Backend != "local" || !strings.Contains(r.Fallback, "whisper.cpp failed") {
		t.Errorf("result = %+v, err = %v", r, err)
	}
}

func TestASlowServerIsGivenUpOn(t *testing.T) {
	fakeTools(t)
	hang := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-hang:
		case <-r.Context().Done():
		}
	}))
	defer slow.Close()
	defer close(hang)
	old := remoteTimeout
	remoteTimeout = 200 * time.Millisecond
	defer func() { remoteTimeout = old }()
	e := engine(t, true)
	if err := e.SetRemote(slow.URL, token); err != nil {
		t.Fatal(err)
	}
	r, err := e.TranscribeAny(context.Background(), audioFile(t), "", "")
	if err != nil || r.Backend != "local" {
		t.Errorf("result = %+v, err = %v", r, err)
	}
}

func TestTheVariablesWin(t *testing.T) {
	fakeTools(t)
	e := engine(t, false)
	if err := e.SetRemote("192.168.0.10:47823", token); err != nil {
		t.Fatal(err)
	}
	c, _ := appconfig.Load(e.data)
	if c.Transcription == nil || c.Transcription.RemoteURL != "http://192.168.0.10:47823" || c.Transcription.RemoteToken != token {
		t.Fatalf("kept: %+v", c.Transcription)
	}
	// An empty token keeps the one kept for the same address.
	if err := e.SetRemote("http://192.168.0.10:47823", ""); err != nil {
		t.Fatal(err)
	}
	if r := e.Settings(); r.Token != token {
		t.Errorf("token = %q", r.Token)
	}
	t.Setenv(EnvRemoteURL, "http://10.0.0.2:47823/")
	t.Setenv(EnvRemoteToken, "from-env")
	if r := e.Settings(); r.Mode != ModeRemote || r.URL != "http://10.0.0.2:47823" || r.Token != "from-env" || !r.FromEnv || r.Model != DefaultModel {
		t.Errorf("remote = %+v", r)
	}
	if err := e.SetRemote("", ""); !errors.Is(err, ErrFromEnv) {
		t.Errorf("err = %v", err)
	}
	t.Setenv(EnvRemoteURL, "")
	if err := e.SetRemote("", ""); err != nil {
		t.Fatal(err)
	}
	if r := e.Settings(); r.Mode != ModeLocal || r.URL != "" {
		t.Errorf("remote = %+v", r)
	}
}

func TestNormalizeURL(t *testing.T) {
	for in, want := range map[string]string{
		"":                            "",
		"192.168.0.10:47823":          "http://192.168.0.10:47823",
		" http://mac.local:47823/ ":   "http://mac.local:47823",
		"https://asr.example.com/x/":  "https://asr.example.com/x",
		"ftp://192.168.0.10":          "!",
		"http://user:pw@host:1":       "!",
		"http://host:1/?token=secret": "!",
	} {
		got, err := NormalizeURL(in)
		if want == "!" {
			if err == nil {
				t.Errorf("%q: accepted as %q", in, got)
			}
			continue
		}
		if err != nil || got != want {
			t.Errorf("%q: %q, %v", in, got, err)
		}
	}
}

func TestTheQueueRespectsTheCaller(t *testing.T) {
	fakeTools(t)
	e := engine(t, true)
	e.slot <- struct{}{} // another transcription holds the engine
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := e.Transcribe(ctx, audioFile(t), "", "")
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 2*time.Second {
		t.Fatalf("err = %v after %s", err, time.Since(start))
	}
	<-e.slot
	if text, err := e.Transcribe(context.Background(), audioFile(t), "", ""); err != nil || text != "heard [auto] []" {
		t.Fatalf("text = %q, err = %v", text, err)
	}
}

func TestTheServerRefusesWithoutTranscription(t *testing.T) {
	fakeTools(t)
	srv := httptest.NewServer(engine(t, false).Handler(token, "test"))
	defer srv.Close()
	e := engine(t, false)
	if err := e.SetRemote(srv.URL, token); err != nil {
		t.Fatal(err)
	}
	st := e.RemoteStatus(context.Background(), true)
	if !st.Reachable || st.Ready || !strings.Contains(st.Error, "transcription install") {
		t.Errorf("status = %+v", st)
	}
	if _, err := e.TranscribeAny(context.Background(), audioFile(t), "", ""); err == nil || !strings.Contains(err.Error(), "503") {
		t.Errorf("err = %v", err)
	}
}
