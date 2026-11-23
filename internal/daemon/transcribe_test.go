package daemon

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// A voice note sent to a transcription server that takes longer than the
// call waits: the call answers "still going", a second call joins the same
// work instead of starting over, and once it ends the transcript is kept.
func TestTranscribeAudioOutlivesTheCall(t *testing.T) {
	base := ""
	if runtime.GOOS != "windows" {
		base = "/tmp"
	}
	root, err := os.MkdirTemp(base, "daemon-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	store := filepath.Join(root, "s")
	_ = os.MkdirAll(store, 0o700)
	_ = os.WriteFile(filepath.Join(store, "AUTHED"), nil, 0o600)
	seedStore(t, store)
	db, err := sql.Open("sqlite", "file:"+filepath.Join(store, "wacli.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(fmt.Sprintf(`INSERT INTO messages (chat_jid, msg_id, sender_jid, ts, from_me, text, media_type, mime_type) VALUES
		('%s', 'A1', '%s', %d, 0, '', 'audio', 'audio/ogg; codecs=opus')`, testMae, testMae, time.Now().Unix()-30))
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	// The voice note is already downloaded, as download_media leaves it.
	media := filepath.Join(root, "data", "media", testMae)
	_ = os.MkdirAll(media, 0o700)
	_ = os.WriteFile(filepath.Join(media, "A1.ogg"), []byte("OggS voice note"), 0o600)

	release := make(chan struct{})
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer a-token-of-sixteen-or-more" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": []any{map[string]any{"id": "whisper-1"}}, "ready": true, "accel": "metal", "version": "test"})
		case "/v1/audio/transcriptions":
			requests.Add(1)
			// On a remote server the conversation stays here by default
			// (1.9.6): no prompt goes with the audio.
			if _, _, err := r.FormFile("file"); err != nil || r.FormValue("model") == "" || r.FormValue("language") != "pt" || r.FormValue("prompt") != "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			<-release
			_ = json.NewEncoder(w).Encode(map[string]any{"text": "Vou sim, levo a sobremesa."})
		}
	}))
	defer srv.Close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	t.Setenv("WHATSAPP_MCP_TRANSCRIPTION_URL", srv.URL)
	t.Setenv("WHATSAPP_MCP_TRANSCRIPTION_TOKEN", "a-token-of-sixteen-or-more")
	t.Setenv("WHATSAPP_MCP_TRANSCRIPTION_WAIT", "1")

	port := freePort(t)
	d, err := Start(Config{Port: port, WacliBin: fakeWacli, StoreDir: store, DataDir: filepath.Join(root, "data")})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Stop()
	waitSync(t, d, "connected")

	args := map[string]any{"message_id": "A1", "language": "pt"}
	res, isErr := callTool(t, port, "transcribe_audio", args)
	if isErr || res["status"] != "in_progress" || res["joined"] != false || res["retry_after_seconds"] == nil || res["backend"] != "remote" {
		t.Fatalf("first call: %v", res)
	}
	res, isErr = callTool(t, port, "transcribe_audio", args)
	if isErr || res["status"] != "in_progress" || res["joined"] != true {
		t.Fatalf("second call: %v", res)
	}
	status, _ := callTool(t, port, "whatsapp_status", nil)
	tr, _ := status["transcription"].(map[string]any)
	if tr["backend"] != "remote" || tr["in_progress"] != float64(1) || tr["context"] != "none" {
		t.Errorf("status: %v", tr)
	}

	// A third call joins the work, which ends while it waits.
	type answer struct {
		res   map[string]any
		isErr bool
	}
	third := make(chan answer, 1)
	go func() {
		r, e := callTool(t, port, "transcribe_audio", args)
		third <- answer{r, e}
	}()
	time.Sleep(300 * time.Millisecond)
	close(release)
	a := <-third
	res, isErr = a.res, a.isErr
	if res["joined"] != true {
		t.Fatalf("the third call did not join and finish: %v", res)
	}
	got, _ := res["transcript"].(map[string]any)
	if isErr || got["text"] != "Vou sim, levo a sobremesa." || got["source"] != "remote" || res["context_level"] != "none" {
		t.Fatalf("after: %v", res)
	}
	// The answer that ends the work (joined or not) carries the transcript
	// as kept: its date and language, not empty values.
	if when, _ := time.Parse(time.RFC3339, fmt.Sprint(got["created_at"])); when.Year() < 2000 || got["language"] != "pt" {
		t.Errorf("the transcript in the answer: created_at %v, language %v", got["created_at"], got["language"])
	}
	if n := requests.Load(); n != 1 {
		t.Errorf("the server was asked %d times, want once", n)
	}
	// From then on it is kept.
	res, _ = callTool(t, port, "transcribe_audio", args)
	if res["cached"] != true {
		t.Errorf("not kept: %v", res)
	}
}
