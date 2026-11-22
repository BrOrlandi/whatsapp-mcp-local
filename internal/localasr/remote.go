package localasr

// Voice notes are transcribed in one of three ways (the mode):
//
//   - local: whisper.cpp on this computer;
//   - remote: any server that speaks OpenAI's transcription API, at an
//     address of the user's: `whatsapp-mcp transcription serve` on a Mac
//     with an Apple chip (serve.go), whisper.cpp's server, speaches…;
//   - openai: OpenAI's API itself, with the user's key.
//
// Remote and OpenAI are the same client; only the address, the key and the
// model change. When either fails (off, asleep, slow, refused), the voice
// note is transcribed on this computer, if transcription is installed here,
// so a sleeping Mac costs time and never the transcript.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/appconfig"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
)

// The modes.
const (
	ModeLocal  = "local"
	ModeRemote = "remote"
	ModeOpenAI = "openai"
)

// The variables that set the transcription; they win over the setting kept
// in config.json, as the server mode's variables do. Setting the mode or the
// address is what makes them count.
const (
	// EnvMode is local, remote or openai; when only the address is set, it is
	// remote.
	EnvMode = "WHATSAPP_MCP_TRANSCRIPTION_MODE"
	// EnvRemoteURL is the remote server's address.
	EnvRemoteURL = "WHATSAPP_MCP_TRANSCRIPTION_URL"
	// EnvRemoteToken is the remote server's token (and, for `transcription
	// serve`, the token it asks for).
	EnvRemoteToken = "WHATSAPP_MCP_TRANSCRIPTION_TOKEN"
	// EnvModel is the model asked of the remote server or of OpenAI.
	EnvModel = "WHATSAPP_MCP_TRANSCRIPTION_MODEL"
	// EnvOpenAIKey is the OpenAI API key; OPENAI_API_KEY stands in for it
	// when the mode is openai.
	EnvOpenAIKey = "WHATSAPP_MCP_TRANSCRIPTION_OPENAI_KEY"
)

// OpenAIURL is OpenAI's API.
var OpenAIURL = "https://api.openai.com/v1"

// DefaultModel is the model asked for when none is set: OpenAI's Whisper,
// and a name every compatible server takes.
const DefaultModel = "whisper-1"

// OpenAIModels are OpenAI's transcription models, for the settings page.
var OpenAIModels = []string{"whisper-1", "gpt-4o-transcribe", "gpt-4o-mini-transcribe"}

// openAIMaxAudio is the largest file OpenAI takes.
const openAIMaxAudio = 25 << 20

var (
	// dialTimeout is how long a server has to accept the connection: a
	// computer that is asleep or off is given up on quickly.
	dialTimeout = 3 * time.Second
	// probeTimeout bounds the check of whether the server is up.
	probeTimeout = 6 * time.Second
	// remoteTimeout bounds one transcription on the server, which, on a GPU,
	// takes seconds; past it the voice note is transcribed here instead.
	remoteTimeout = 10 * time.Minute
	// probeFor is how long a check of the server is reused, so the status
	// tools do not wait on the network each time.
	probeFor = 15 * time.Second
)

// Settings is how voice notes are transcribed.
type Settings struct {
	// Mode is local, remote or openai.
	Mode string
	// URL is the remote server's address (OpenAI's for openai).
	URL string
	// Token is the remote server's token, or the OpenAI key.
	Token string
	// Model is the model asked for, in remote and openai.
	Model string
	// FromEnv says the variables set it, so the panel cannot change it.
	FromEnv bool
}

// Offsite says voice notes go to another computer or to OpenAI.
func (s Settings) Offsite() bool { return s.Mode == ModeRemote || s.Mode == ModeOpenAI }

// usable says the offsite mode has what it needs: an address, or a key.
func (s Settings) usable() error {
	switch s.Mode {
	case ModeRemote:
		if s.URL == "" {
			return errors.New("no transcription server address is set")
		}
		if _, err := NormalizeURL(s.URL); err != nil {
			return fmt.Errorf("the transcription server address %q is not valid", s.URL)
		}
	case ModeOpenAI:
		if s.Token == "" {
			return errors.New("no OpenAI API key is set")
		}
	}
	return nil
}

// base is the API's base address, the one that ends in /v1: an address
// without a path is a server's root, and its API is at /v1.
func (s Settings) base() string {
	if s.Mode == ModeOpenAI {
		return OpenAIURL
	}
	if u, err := url.Parse(s.URL); err == nil && (u.Path == "" || u.Path == "/") {
		return strings.TrimRight(s.URL, "/") + "/v1"
	}
	return strings.TrimRight(s.URL, "/")
}

// where names the other end, for messages.
func (s Settings) where() string {
	if s.Mode == ModeOpenAI {
		return "OpenAI"
	}
	return "the transcription server at " + s.URL
}

// RemoteStatus is what the panel and the status tools show about the remote
// server or OpenAI; it never carries the token or the key.
type RemoteStatus struct {
	Configured bool   `json:"configured"`
	Mode       string `json:"mode,omitempty"`
	URL        string `json:"url,omitempty"`
	Model      string `json:"model,omitempty"`
	FromEnv    bool   `json:"from_env,omitempty"`
	Reachable  bool   `json:"reachable"`
	Ready      bool   `json:"ready"`
	// Accel, Engine and Version come from `whatsapp-mcp transcription
	// serve`; other servers do not say.
	Accel     string    `json:"accel,omitempty"`
	Engine    string    `json:"engine,omitempty"`
	Version   string    `json:"version,omitempty"`
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checked_at,omitzero"`
}

// Where names the other end, for messages: OpenAI, or the server's address.
func (s RemoteStatus) Where() string {
	if s.Mode == ModeOpenAI {
		return "OpenAI (" + s.Model + ")"
	}
	return "the transcription server at " + s.URL
}

// Result is a transcript and what made it.
type Result struct {
	Text string
	// Model names the engine, for the kept transcript.
	Model string
	// Backend is "remote", "openai" or "local".
	Backend string
	// Fallback is why the remote server or OpenAI was not used, when one is
	// set up and this computer transcribed instead.
	Fallback string
}

// NormalizeURL checks a transcription server's address and writes it the
// one way: http(s)://host:port[/path], without a trailing slash. A bare
// host:port is taken as http.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New(i18n.T("o endereço do servidor de transcrição precisa ser como http://192.168.0.10:47823"))
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host+u.Path, "/"), nil
}

// Settings is the transcription in use: the variables', else the one kept
// in config.json. It is read each time, so a change made by the command line
// reaches a running app.
func (e *Engine) Settings() Settings {
	envMode := strings.ToLower(strings.TrimSpace(os.Getenv(EnvMode)))
	envURL := strings.TrimSpace(os.Getenv(EnvRemoteURL))
	if envMode != "" || envURL != "" {
		s := Settings{Mode: envMode, FromEnv: true, Model: strings.TrimSpace(os.Getenv(EnvModel))}
		if s.Mode == "" {
			s.Mode = ModeRemote
		}
		switch s.Mode {
		case ModeRemote:
			s.URL = envURL
			if n, err := NormalizeURL(envURL); err == nil {
				s.URL = n
			} // else kept as given, so the status shows what is wrong
			s.Token = strings.TrimSpace(os.Getenv(EnvRemoteToken))
		case ModeOpenAI:
			s.URL = OpenAIURL
			s.Token = strings.TrimSpace(os.Getenv(EnvOpenAIKey))
			if s.Token == "" {
				s.Token = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
			}
		}
		if s.Offsite() && s.Model == "" {
			s.Model = DefaultModel
		}
		return s
	}
	c, err := appconfig.Load(e.data)
	if err != nil || c.Transcription == nil {
		return Settings{Mode: ModeLocal}
	}
	return fromConfig(c.Transcription)
}

func fromConfig(t *appconfig.Transcription) Settings {
	mode := t.Mode
	if mode == "" { // kept by 1.9.0-beta: an address meant remote
		mode = ModeLocal
		if t.RemoteURL != "" {
			mode = ModeRemote
		}
	}
	s := Settings{Mode: mode}
	switch mode {
	case ModeRemote:
		s.URL, s.Token, s.Model = t.RemoteURL, t.RemoteToken, t.RemoteModel
	case ModeOpenAI:
		s.URL, s.Token, s.Model = OpenAIURL, t.OpenAIKey, t.OpenAIModel
	}
	if s.Offsite() && s.Model == "" {
		s.Model = DefaultModel
	}
	return s
}

// ErrFromEnv means the variables set the transcription.
var ErrFromEnv = errors.New("the transcription is set by " + EnvMode + " or " + EnvRemoteURL)

// Change is a new transcription setting. An empty Token keeps the one
// already kept (for the same address, in remote), so the panel never has to
// show it; an empty Model keeps the one kept.
type Change struct {
	Mode  string
	URL   string
	Token string
	Model string
}

// Configure keeps a new transcription setting in config.json. What the other
// modes keep stays, so switching back does not ask for it again.
func (e *Engine) Configure(ch Change) error {
	if e.Settings().FromEnv {
		return ErrFromEnv
	}
	c, err := appconfig.Load(e.data)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	t := appconfig.Transcription{}
	if c.Transcription != nil {
		t = *c.Transcription
	}
	token, model := strings.TrimSpace(ch.Token), strings.TrimSpace(ch.Model)
	switch ch.Mode {
	case ModeLocal:
	case ModeRemote:
		u, err := NormalizeURL(ch.URL)
		if err != nil {
			return err
		}
		if u == "" {
			return errors.New(i18n.T("informe o endereço do servidor de transcrição"))
		}
		// A token and a model are for their own server.
		if moved := t.RemoteURL != u; moved {
			t.RemoteToken, t.RemoteModel = "", ""
		}
		t.RemoteURL = u
		if token != "" {
			t.RemoteToken = token
		}
		if model != "" {
			t.RemoteModel = model
		}
	case ModeOpenAI:
		if token != "" {
			t.OpenAIKey = token
		}
		if t.OpenAIKey == "" {
			return errors.New(i18n.T("informe a chave da API da OpenAI"))
		}
		if model != "" {
			t.OpenAIModel = model
		}
	default:
		return fmt.Errorf("the transcription mode must be local, remote or openai, not %q", ch.Mode)
	}
	t.Mode = ch.Mode
	if t == (appconfig.Transcription{Mode: ModeLocal}) {
		c.Transcription = nil
	} else {
		c.Transcription = &t
	}
	e.probeMu.Lock()
	e.probe = RemoteStatus{}
	e.probeMu.Unlock()
	return appconfig.Save(e.data, c)
}

// SetRemote points voice notes at a remote server; an empty address goes
// back to this computer.
func (e *Engine) SetRemote(rawURL, token string) error {
	if strings.TrimSpace(rawURL) == "" {
		return e.Configure(Change{Mode: ModeLocal})
	}
	return e.Configure(Change{Mode: ModeRemote, URL: rawURL, Token: token})
}

// offsite is the remote server or OpenAI in use, when there is one that can
// be tried.
func (e *Engine) offsite() (Settings, bool) {
	s := e.Settings()
	return s, s.Offsite() && s.usable() == nil
}

// Available says a voice note can be transcribed: offsite or on this
// computer.
func (e *Engine) Available() bool {
	_, ok := e.offsite()
	return ok || e.Status().Ready
}

// Backend says where voice notes go: "remote" or "openai" (with this
// computer as the fallback when it is set up: "remote+local",
// "openai+local"), "local", or "none".
func (e *Engine) Backend() string {
	s, off := e.offsite()
	local := e.Status().Ready
	switch {
	case off && local:
		return s.Mode + "+local"
	case off:
		return s.Mode
	case local:
		return ModeLocal
	}
	return "none"
}

func remoteClient() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = (&net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = 5 * time.Second
	// No overall timeout here: each call carries its own context.
	return &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

var client = remoteClient()

// RemoteStatus checks the remote server or OpenAI, reusing a check made in
// the last few seconds unless fresh is set. Configured is false in local.
func (e *Engine) RemoteStatus(ctx context.Context, fresh bool) RemoteStatus {
	s := e.Settings()
	if !s.Offsite() {
		return RemoteStatus{}
	}
	e.probeMu.Lock()
	cached := e.probe
	e.probeMu.Unlock()
	if !fresh && cached.Mode == s.Mode && cached.URL == s.URL && cached.Model == s.Model && time.Since(cached.CheckedAt) < probeFor {
		return cached
	}
	st := RemoteStatus{Configured: true, Mode: s.Mode, URL: s.URL, Model: s.Model, FromEnv: s.FromEnv, CheckedAt: time.Now()}
	if err := s.usable(); err != nil {
		st.Error = err.Error()
	} else if m, err := probe(ctx, s); err != nil {
		st.Error = err.Error()
	} else {
		st.Reachable, st.Ready, st.Accel, st.Engine, st.Version = true, m.ready(), m.Accel, m.Engine, m.Version
		switch {
		case !st.Ready:
			st.Error = "the transcription server has no transcription installed: run `whatsapp-mcp transcription install` there"
		case s.Mode == ModeOpenAI && len(m.Data) > 0 && !m.has(s.Model):
			st.Ready, st.Error = false, fmt.Sprintf("this OpenAI key cannot use the model %s", s.Model)
		}
	}
	e.probeMu.Lock()
	e.probe = st
	e.probeMu.Unlock()
	return st
}

// models is what GET /v1/models answers: OpenAI's list, which
// `whatsapp-mcp transcription serve` extends with whether its engine is
// installed, its acceleration and its version.
type models struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
	Ready   *bool  `json:"ready"`
	Accel   string `json:"accel"`
	Engine  string `json:"engine"`
	Version string `json:"version"`
}

// ready: a server that does not say is taken as ready.
func (m models) ready() bool { return m.Ready == nil || *m.Ready }

func (m models) has(id string) bool {
	return slices.ContainsFunc(m.Data, func(d struct {
		ID string `json:"id"`
	}) bool {
		return d.ID == id
	})
}

// probe asks the server for its models: an answer means it is up and the
// token is right. A server that does not list models (whisper.cpp's) answers
// 404, which still means it is up.
func probe(ctx context.Context, s Settings) (models, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	var m models
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base()+"/models", nil)
	if err != nil {
		return m, err
	}
	authorize(req, s.Token)
	resp, err := client.Do(req)
	if err != nil {
		return m, unreachable(s, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return m, nil
	}
	if err := answerError(s, resp); err != nil {
		return m, err
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&m); err != nil {
		return m, fmt.Errorf("%s answered something else: %w", s.where(), err)
	}
	return m, nil
}

func authorize(req *http.Request, token string) {
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func unreachable(s Settings, err error) error {
	return fmt.Errorf("%s did not answer: %w", s.where(), err)
}

// answerError turns a refusal into an error that says what to do. A refused
// token or key gets a fixed message, so nothing of it is repeated.
func answerError(s Settings, resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		if s.Mode == ModeOpenAI {
			return errors.New("OpenAI refused the API key: check it in the transcription settings")
		}
		return errors.New("the transcription server refused the token: use the one in transcription-server.token in that computer's data folder")
	}
	if msg := errorMessage(io.LimitReader(resp.Body, 64<<10)); msg != "" {
		return fmt.Errorf("%s answered %s: %s", s.where(), resp.Status, msg)
	}
	return fmt.Errorf("%s answered %s", s.where(), resp.Status)
}

// errorMessage reads an error body: OpenAI's {"error": {"message"}}, a
// plain {"error": "…"}, or FastAPI's {"detail": "…"}.
func errorMessage(r io.Reader) string {
	var body struct {
		Error  json.RawMessage `json:"error"`
		Detail any             `json:"detail"`
	}
	if json.NewDecoder(r).Decode(&body) != nil {
		return ""
	}
	var msg string
	var obj struct {
		Message string `json:"message"`
	}
	switch {
	case json.Unmarshal(body.Error, &obj) == nil && obj.Message != "":
		msg = obj.Message
	case json.Unmarshal(body.Error, &msg) == nil:
	case body.Detail != nil:
		msg = fmt.Sprint(body.Detail)
	}
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}
	return msg
}

// audioTypes are the file extensions OpenAI takes; a voice note with another
// name is sent as .ogg, which WhatsApp's voice notes are.
var audioTypes = []string{".flac", ".m4a", ".mp3", ".mp4", ".mpeg", ".mpga", ".oga", ".ogg", ".wav", ".webm"}

// transcribeRemote sends the voice note to the remote server or OpenAI, as
// POST <base>/audio/transcriptions. engine is what the server says made it,
// when it says.
func transcribeRemote(ctx context.Context, s Settings, audio, language, prompt string) (text, engine string, err error) {
	limit := int64(maxAudio)
	if s.Mode == ModeOpenAI {
		limit = openAIMaxAudio
	}
	f, err := os.Open(audio)
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.Size() > limit {
		return "", "", fmt.Errorf("the voice note is too large to send to %s (%d MB)", s.where(), fi.Size()>>20)
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("model", s.Model)
	_ = w.WriteField("response_format", "json")
	if language != "" && language != "auto" {
		_ = w.WriteField("language", language)
	}
	if strings.TrimSpace(prompt) != "" {
		_ = w.WriteField("prompt", prompt)
	}
	name := filepath.Base(audio)
	ext := strings.ToLower(filepath.Ext(name))
	if !slices.Contains(audioTypes, ext) {
		name, ext = "audio.ogg", ".ogg"
	}
	ctype := mime.TypeByExtension(ext)
	if ext == ".ogg" || ext == ".oga" {
		ctype = "audio/ogg"
	}
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, name))
	h.Set("Content-Type", ctype)
	part, err := w.CreatePart(h)
	if err != nil {
		return "", "", err
	}
	if _, err := io.Copy(part, io.LimitReader(f, limit+1)); err != nil {
		return "", "", err
	}
	if err := w.Close(); err != nil {
		return "", "", err
	}
	ctx, cancel := context.WithTimeout(ctx, remoteTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.base()+"/audio/transcriptions", &body)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	authorize(req, s.Token)
	resp, err := client.Do(req)
	if err != nil {
		return "", "", unreachable(s, err)
	}
	defer resp.Body.Close()
	if err := answerError(s, resp); err != nil {
		return "", "", err
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", "", unreachable(s, err)
	}
	var out struct {
		Text   *string `json:"text"`
		Engine string  `json:"engine"`
	}
	if json.Unmarshal(raw, &out) != nil || out.Text == nil {
		// A server that answers text despite being asked for json.
		if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "text/plain") {
			return strings.TrimSpace(string(raw)), "", nil
		}
		return "", "", fmt.Errorf("%s answered something other than a transcript", s.where())
	}
	return strings.TrimSpace(*out.Text), out.Engine, nil
}

// label names what made a transcript, for the kept transcript.
func label(s Settings, engine string) string {
	if s.Mode == ModeOpenAI {
		return "OpenAI " + s.Model
	}
	if engine == "" {
		engine = s.Model
	}
	host := s.URL
	if u, err := url.Parse(s.URL); err == nil && u.Host != "" {
		host = u.Host
	}
	return engine + " (transcription server " + host + ")"
}

// TranscribeAny transcribes a voice note on the remote server or OpenAI when
// one is set up, and on this computer when there is none or it fails: it is
// off, asleep, slow, or refused the token. Without transcription installed
// here, the offsite failure is the error.
func (e *Engine) TranscribeAny(ctx context.Context, audio, language, prompt string) (Result, error) {
	s := e.Settings()
	if s.Offsite() {
		err := s.usable()
		if err == nil {
			var text, engine string
			text, engine, err = transcribeRemote(ctx, s, audio, language, prompt)
			if err == nil {
				return Result{Text: text, Model: label(s, engine), Backend: s.Mode}, nil
			}
			if ctx.Err() != nil {
				return Result{}, err
			}
		}
		// What failed shows in the status tools until the next check.
		e.probeMu.Lock()
		e.probe = RemoteStatus{Configured: true, Mode: s.Mode, URL: s.URL, Model: s.Model, FromEnv: s.FromEnv, Error: err.Error(), CheckedAt: time.Now()}
		e.probeMu.Unlock()
		if !e.Status().Ready {
			return Result{}, fmt.Errorf("%w; transcription is not installed on this computer to fall back on", err)
		}
		text, lerr := e.Transcribe(ctx, audio, language, prompt)
		if lerr != nil {
			return Result{}, fmt.Errorf("%v; on this computer: %w", err, lerr)
		}
		return Result{Text: text, Model: Label, Backend: ModeLocal, Fallback: err.Error()}, nil
	}
	text, err := e.Transcribe(ctx, audio, language, prompt)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Model: Label, Backend: ModeLocal}, nil
}
