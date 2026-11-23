package localasr

import (
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/appconfig"
)

// fakeOpenAI is a server in OpenAI's format: it lists models, takes a
// transcription and keeps what it was sent, so a test sees the request.
type fakeOpenAI struct {
	*httptest.Server
	mu     sync.Mutex
	got    map[string]string // the multipart fields; "file" is its name
	auth   string
	path   string
	ctype  string
	models []string
	// status and body, when set, answer every transcription.
	status int
	body   string
}

func newFakeOpenAI(t *testing.T, key string, models ...string) *fakeOpenAI {
	t.Helper()
	f := &fakeOpenAI{models: models}
	mux := http.NewServeMux()
	check := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer "+key {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":{"message":"Incorrect API key provided: sk-abc***xyz","type":"invalid_request_error","param":null,"code":"invalid_api_key"}}`)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		if !check(w, r) {
			return
		}
		data := []map[string]string{}
		for _, m := range f.models {
			data = append(data, map[string]string{"id": m, "object": "model"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data})
	})
	mux.HandleFunc("POST /v1/audio/transcriptions", func(w http.ResponseWriter, r *http.Request) {
		if !check(w, r) {
			return
		}
		got := map[string]string{}
		mr, err := r.MultipartReader()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var ctype string
		for {
			p, err := mr.NextPart()
			if err != nil {
				break
			}
			b, _ := io.ReadAll(p)
			if p.FormName() == "file" {
				got["file"], ctype = p.FileName(), p.Header.Get("Content-Type")
				got["file_bytes"] = string(b)
				continue
			}
			got[p.FormName()] = string(b)
		}
		f.mu.Lock()
		f.got, f.auth, f.path, f.ctype = got, r.Header.Get("Authorization"), r.URL.Path, ctype
		status, body := f.status, f.body
		f.mu.Unlock()
		if status != 0 {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, body)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"text": " olá, " + got["language"] + " "})
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeOpenAI) request() (map[string]string, string, string, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.got, f.auth, f.path, f.ctype
}

// OpenAI is the same client as a remote server, at OpenAI's address, with
// the key and the model; the variables set it.
func TestOpenAITranscribes(t *testing.T) {
	fakeTools(t)
	key := "sk-test-key-of-some-length"
	f := newFakeOpenAI(t, key, "whisper-1", "gpt-4o-transcribe")
	old := OpenAIURL
	OpenAIURL = f.URL + "/v1"
	defer func() { OpenAIURL = old }()
	t.Setenv(EnvMode, "openai")
	t.Setenv("OPENAI_API_KEY", key)
	t.Setenv(EnvModel, "gpt-4o-transcribe")

	e := engine(t, false)
	if s := e.Settings(); s.Mode != ModeOpenAI || s.Token != key || s.Model != "gpt-4o-transcribe" || !s.FromEnv {
		t.Fatalf("settings = %+v", s)
	}
	if e.Backend() != "openai" || !e.Available() {
		t.Fatalf("backend = %s", e.Backend())
	}
	r, err := e.TranscribeAny(context.Background(), audioFile(t), "pt", "Conversa com Ana")
	if err != nil {
		t.Fatal(err)
	}
	if r.Text != "olá, pt" || r.Backend != "openai" || r.Model != "OpenAI gpt-4o-transcribe" {
		t.Errorf("result = %+v", r)
	}
	got, auth, path, ctype := f.request()
	if path != "/v1/audio/transcriptions" || auth != "Bearer "+key {
		t.Errorf("path %s, auth %q", path, auth)
	}
	want := map[string]string{"model": "gpt-4o-transcribe", "language": "pt", "prompt": "Conversa com Ana", "response_format": "json", "file": "note.ogg", "file_bytes": "OggS fake voice note"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if ctype != "audio/ogg" {
		t.Errorf("the file went as %q", ctype)
	}
	st := e.RemoteStatus(context.Background(), true)
	if !st.Reachable || !st.Ready || st.Mode != ModeOpenAI || st.Where() != "OpenAI (gpt-4o-transcribe)" {
		t.Errorf("status = %+v", st)
	}

	// A model the key cannot use shows in the status.
	t.Setenv(EnvModel, "gpt-4o-mini-transcribe")
	if st := e.RemoteStatus(context.Background(), true); st.Ready || !strings.Contains(st.Error, "gpt-4o-mini-transcribe") {
		t.Errorf("status = %+v", st)
	}

	// Without a language, none is sent: OpenAI detects it. A file with a
	// name OpenAI does not take goes as .ogg.
	t.Setenv(EnvModel, "")
	odd := filepath.Join(t.TempDir(), "voice.opus")
	if err := os.WriteFile(odd, []byte("OggS"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := e.TranscribeAny(context.Background(), odd, "auto", ""); err != nil {
		t.Fatal(err)
	}
	got, _, _, _ = f.request()
	if _, ok := got["language"]; ok {
		t.Errorf("language sent: %q", got["language"])
	}
	if _, ok := got["prompt"]; ok {
		t.Errorf("an empty prompt was sent")
	}
	if got["file"] != "audio.ogg" || got["model"] != DefaultModel {
		t.Errorf("file %q, model %q", got["file"], got["model"])
	}
}

// A refused key gets a fixed message, without what OpenAI said about it; an
// error in OpenAI's format is passed on; either way this computer stands in.
func TestOpenAIErrors(t *testing.T) {
	fakeTools(t)
	f := newFakeOpenAI(t, "sk-right-key-0000000000")
	old := OpenAIURL
	OpenAIURL = f.URL + "/v1"
	defer func() { OpenAIURL = old }()
	t.Setenv(EnvMode, "openai")
	t.Setenv(EnvOpenAIKey, "sk-wrong-key-000000000")

	e := engine(t, false)
	_, err := e.TranscribeAny(context.Background(), audioFile(t), "", "")
	if err == nil || !strings.Contains(err.Error(), "OpenAI refused the API key") || strings.Contains(err.Error(), "sk-") {
		t.Errorf("err = %v", err)
	}
	if st := e.RemoteStatus(context.Background(), true); st.Reachable || strings.Contains(st.Error, "sk-") {
		t.Errorf("status = %+v", st)
	}

	t.Setenv(EnvOpenAIKey, "sk-right-key-0000000000")
	f.status, f.body = http.StatusBadRequest, `{"error":{"message":"Audio file might be corrupted or unsupported","type":"invalid_request_error","param":"file","code":"invalid_value"}}`
	withLocal := engine(t, true)
	r, err := withLocal.TranscribeAny(context.Background(), audioFile(t), "en", "ctx")
	if err != nil || r.Backend != ModeLocal || r.Text != "heard [en] [ctx]" || !strings.Contains(r.Fallback, "Audio file might be corrupted") {
		t.Errorf("result = %+v, err = %v", r, err)
	}

	// No key at all: not available, and the status says why.
	t.Setenv(EnvOpenAIKey, "")
	if e.Available() || e.Backend() != "none" {
		t.Errorf("available %v, backend %s", e.Available(), e.Backend())
	}
	if st := e.RemoteStatus(context.Background(), true); !st.Configured || !strings.Contains(st.Error, "no OpenAI API key") {
		t.Errorf("status = %+v", st)
	}
}

// A remote server is any server in OpenAI's format: an address without a
// path is its root (the API under /v1), one with a path is the API itself.
// One that does not list models still counts as up.
func TestRemoteIsAnyOpenAICompatibleServer(t *testing.T) {
	fakeTools(t)
	f := newFakeOpenAI(t, "tok")
	e := engine(t, false)
	for _, addr := range []string{f.URL, f.URL + "/v1"} {
		if err := e.Configure(Change{Mode: ModeRemote, URL: addr, Token: "tok", Model: "Systran/faster-whisper-large-v3"}); err != nil {
			t.Fatal(err)
		}
		r, err := e.TranscribeAny(context.Background(), audioFile(t), "es", "")
		if err != nil || r.Text != "olá, es" || r.Backend != ModeRemote {
			t.Fatalf("%s: result = %+v, err = %v", addr, r, err)
		}
		got, _, path, _ := f.request()
		if path != "/v1/audio/transcriptions" || got["model"] != "Systran/faster-whisper-large-v3" {
			t.Errorf("%s: path %s, model %q", addr, path, got["model"])
		}
		if !strings.Contains(r.Model, "Systran/faster-whisper-large-v3 (transcription server ") {
			t.Errorf("model label = %q", r.Model)
		}
	}

	// whisper.cpp's server has no model list, but has the transcription
	// route: asked without a file, it answers with an error, so it is up.
	whisperCpp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/audio/transcriptions" {
			_, _ = io.WriteString(w, `{"error":"no 'file' field in the request"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer whisperCpp.Close()
	if err := e.Configure(Change{Mode: ModeRemote, URL: whisperCpp.URL}); err != nil {
		t.Fatal(err)
	}
	if st := e.RemoteStatus(context.Background(), true); !st.Reachable || !st.Ready {
		t.Errorf("whisper.cpp: status = %+v", st)
	}
	// Any other program at that address answers 404 everywhere: not up, and
	// the status says it is not a transcription server.
	bare := httptest.NewServer(http.NotFoundHandler())
	defer bare.Close()
	if err := e.Configure(Change{Mode: ModeRemote, URL: bare.URL}); err != nil {
		t.Fatal(err)
	}
	if st := e.RemoteStatus(context.Background(), true); st.Reachable || st.Ready || st.Reason != "not_compatible" {
		t.Errorf("another program: status = %+v", st)
	}
}

// Each mode keeps its own settings, so switching back asks for nothing, and
// a config.json from 1.9.0-beta (an address, no mode) means remote.
func TestModesKeepTheirSettings(t *testing.T) {
	fakeTools(t)
	e := engine(t, false)
	if err := e.Configure(Change{Mode: ModeOpenAI}); err == nil {
		t.Error("openai without a key was accepted")
	}
	if err := e.Configure(Change{Mode: ModeRemote}); err == nil {
		t.Error("remote without an address was accepted")
	}
	if err := e.Configure(Change{Mode: "cloud"}); err == nil {
		t.Error("an unknown mode was accepted")
	}
	must := func(ch Change) {
		t.Helper()
		if err := e.Configure(ch); err != nil {
			t.Fatal(err)
		}
	}
	must(Change{Mode: ModeRemote, URL: "192.168.0.10:47823", Token: "remote-token", Model: "large"})
	must(Change{Mode: ModeOpenAI, Token: "sk-key", Model: "gpt-4o-mini-transcribe"})
	if s := e.Settings(); s.Mode != ModeOpenAI || s.Token != "sk-key" || s.Model != "gpt-4o-mini-transcribe" || s.URL != OpenAIURL {
		t.Errorf("openai = %+v", s)
	}
	must(Change{Mode: ModeRemote, URL: "http://192.168.0.10:47823"})
	if s := e.Settings(); s.Token != "remote-token" || s.Model != "large" {
		t.Errorf("back to remote = %+v", s)
	}
	must(Change{Mode: ModeLocal})
	must(Change{Mode: ModeOpenAI})
	if s := e.Settings(); s.Token != "sk-key" {
		t.Errorf("back to openai = %+v", s)
	}
	// Another server does not get the last one's token or model.
	must(Change{Mode: ModeRemote, URL: "http://10.0.0.3:8000"})
	if s := e.Settings(); s.Token != "" || s.Model != DefaultModel {
		t.Errorf("another server = %+v", s)
	}

	if err := appconfig.Save(e.data, appconfig.Config{Transcription: &appconfig.Transcription{RemoteURL: "http://10.0.0.9:47823", RemoteToken: "old"}}); err != nil {
		t.Fatal(err)
	}
	if s := e.Settings(); s.Mode != ModeRemote || s.URL != "http://10.0.0.9:47823" || s.Token != "old" {
		t.Errorf("1.9.0-beta config = %+v", s)
	}
	// config.json holds secrets: only its owner reads it.
	if fi, err := os.Stat(appconfig.Path(e.data)); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("config.json mode = %v, %v", fi.Mode().Perm(), err)
	}
}

// The transcription server speaks OpenAI's API: the formats, the errors, the
// model list.
func TestTheServerSpeaksOpenAI(t *testing.T) {
	fakeTools(t)
	srv := transcriptionServer(t)
	post := func(fields map[string]string, withFile bool) (*http.Response, string) {
		t.Helper()
		pr, pw := io.Pipe()
		w := multipart.NewWriter(pw)
		go func() {
			for k, v := range fields {
				_ = w.WriteField(k, v)
			}
			if withFile {
				part, _ := w.CreateFormFile("file", "note.ogg")
				_, _ = part.Write([]byte("OggS"))
			}
			_ = w.Close()
			_ = pw.Close()
		}()
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/audio/transcriptions", pr)
		req.Header.Set("Content-Type", w.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}
	type apiErr struct {
		Error struct {
			Message, Type string
			Code          *string
		}
	}
	decodeErr := func(body string) apiErr {
		var e apiErr
		if err := json.Unmarshal([]byte(body), &e); err != nil || e.Error.Message == "" || e.Error.Type == "" {
			t.Errorf("not an OpenAI error: %s", body)
		}
		return e
	}

	resp, body := post(map[string]string{"model": "whisper-1", "language": "pt", "prompt": "Ana"}, true)
	if resp.StatusCode != 200 || !strings.Contains(body, `"text":"heard [pt] [Ana]"`) {
		t.Errorf("json: %d %s", resp.StatusCode, body)
	}
	resp, body = post(map[string]string{"response_format": "text"}, true)
	if resp.StatusCode != 200 || body != "heard [auto] []\n" || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") {
		t.Errorf("text: %d %q", resp.StatusCode, body)
	}
	resp, body = post(map[string]string{"response_format": "verbose_json", "language": "en"}, true)
	var verbose struct {
		Text, Language string
		Duration       *float64
	}
	if resp.StatusCode != 200 || json.Unmarshal([]byte(body), &verbose) != nil || verbose.Text != "heard [en] []" || verbose.Language != "en" || verbose.Duration == nil {
		t.Errorf("verbose_json: %d %s", resp.StatusCode, body)
	}
	resp, body = post(map[string]string{"response_format": "srt"}, true)
	if resp.StatusCode != 400 || decodeErr(body).Error.Type != "invalid_request_error" {
		t.Errorf("srt: %d %s", resp.StatusCode, body)
	}
	resp, body = post(nil, false)
	if resp.StatusCode != 400 || !strings.Contains(decodeErr(body).Error.Message, "file") {
		t.Errorf("no file: %d %s", resp.StatusCode, body)
	}

	get := func(path, auth string) (int, string) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	code, body := get("/v1/models", token)
	if code != 200 || !strings.Contains(body, `"id":"whisper-1"`) || !strings.Contains(body, `"object":"list"`) || !strings.Contains(body, `"ready":true`) {
		t.Errorf("models: %d %s", code, body)
	}
	code, body = get("/v1/models", "")
	if e := decodeErr(body); code != 401 || e.Error.Code == nil || *e.Error.Code != "invalid_api_key" {
		t.Errorf("no token: %d %s", code, body)
	}
	code, body = get("/v1/transcribe", token)
	if code != 404 {
		t.Errorf("the old endpoint: %d %s", code, body)
	}
	decodeErr(body)

	// The app's own client, against it, reads the extras.
	e := engine(t, false)
	if err := e.SetRemote(srv.URL, token); err != nil {
		t.Fatal(err)
	}
	if st := e.RemoteStatus(context.Background(), true); st.Engine != serverEngine || st.Accel != "cpu" || st.Version != "test" {
		t.Errorf("status = %+v", st)
	}
}

// A dead address falls back in OpenAI mode as in remote mode.
func TestOpenAIFallsBackWhenUnreachable(t *testing.T) {
	fakeTools(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	old := OpenAIURL
	OpenAIURL = "http://" + ln.Addr().String() + "/v1"
	ln.Close()
	defer func() { OpenAIURL = old }()
	e := engine(t, true)
	if err := e.Configure(Change{Mode: ModeOpenAI, Token: "sk-x"}); err != nil {
		t.Fatal(err)
	}
	if e.Backend() != "openai+local" {
		t.Errorf("backend = %s", e.Backend())
	}
	r, err := e.TranscribeAny(context.Background(), audioFile(t), "", "")
	if err != nil || r.Backend != ModeLocal || !strings.Contains(r.Fallback, "OpenAI did not answer") {
		t.Errorf("result = %+v, err = %v", r, err)
	}
}
