package localasr

// A transcription server is this same engine on another computer, one with a
// GPU (an Apple Silicon Mac, say), reached over the network: `whatsapp-mcp
// transcription serve` runs it there (serve.go), and here the app sends it the
// voice note with the conversation's context and language. When it does not
// answer, the voice note is transcribed on this computer, if transcription is
// installed here, so a sleeping Mac costs time and never the transcript.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/appconfig"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
)

// The variables that point the app at a transcription server; they win over
// the setting kept in config.json, as the server mode's variables do.
const (
	EnvRemoteURL   = "WHATSAPP_MCP_TRANSCRIPTION_URL"
	EnvRemoteToken = "WHATSAPP_MCP_TRANSCRIPTION_TOKEN"
)

// RemoteLabel says a transcription server wrote a transcript.
const RemoteLabel = "whisper.cpp large-v3-turbo (transcription server)"

var (
	// dialTimeout is how long a transcription server has to accept the
	// connection: a computer that is asleep or off is given up on quickly.
	dialTimeout = 3 * time.Second
	// probeTimeout bounds the check of whether the server is up.
	probeTimeout = 4 * time.Second
	// remoteTimeout bounds one transcription on the server, which, on a GPU,
	// takes seconds; past it the voice note is transcribed here instead.
	remoteTimeout = 10 * time.Minute
	// probeFor is how long a check of the server is reused, so the status
	// tools do not wait on the network each time.
	probeFor = 15 * time.Second
)

// Remote is where voice notes go to be transcribed when not here.
type Remote struct {
	URL   string
	Token string
	// FromEnv says the variables set it, so the panel cannot change it.
	FromEnv bool
}

// RemoteStatus is what the panel and the status tools show about the
// transcription server; it never carries the token.
type RemoteStatus struct {
	Configured bool      `json:"configured"`
	URL        string    `json:"url,omitempty"`
	FromEnv    bool      `json:"from_env,omitempty"`
	Reachable  bool      `json:"reachable"`
	Ready      bool      `json:"ready"`
	Accel      string    `json:"accel,omitempty"`
	Version    string    `json:"version,omitempty"`
	Error      string    `json:"error,omitempty"`
	CheckedAt  time.Time `json:"checked_at,omitzero"`
}

// Result is a transcript and what made it.
type Result struct {
	Text string
	// Model names the engine, for the kept transcript.
	Model string
	// Backend is "remote" or "local".
	Backend string
	// Fallback is why the transcription server was not used, when one is set
	// up and this computer transcribed instead.
	Fallback string
}

// NormalizeURL checks a transcription server's address and writes it the
// one way: http(s)://host:port, without a trailing slash. A bare host:port is
// taken as http.
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

// Remote is the transcription server in use: the variables', else the one
// kept in config.json. It is read each time, so a change made by the command
// line reaches a running app.
func (e *Engine) Remote() Remote {
	if u := strings.TrimSpace(os.Getenv(EnvRemoteURL)); u != "" {
		n, err := NormalizeURL(u)
		if err != nil {
			n = u // kept as given, so the status shows what is wrong
		}
		return Remote{URL: n, Token: strings.TrimSpace(os.Getenv(EnvRemoteToken)), FromEnv: true}
	}
	c, err := appconfig.Load(e.data)
	if err != nil || c.Transcription == nil {
		return Remote{}
	}
	return Remote{URL: c.Transcription.RemoteURL, Token: c.Transcription.RemoteToken}
}

// ErrRemoteFromEnv means the variables set the transcription server.
var ErrRemoteFromEnv = errors.New("the transcription server is set by " + EnvRemoteURL)

// SetRemote keeps the transcription server in config.json; an empty address
// stops using one. An empty token keeps the one already kept for the same
// address, so the panel never has to show it.
func (e *Engine) SetRemote(rawURL, token string) error {
	if e.Remote().FromEnv {
		return ErrRemoteFromEnv
	}
	u, err := NormalizeURL(rawURL)
	if err != nil {
		return err
	}
	c, err := appconfig.Load(e.data)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	token = strings.TrimSpace(token)
	if u == "" {
		c.Transcription = nil
	} else {
		if token == "" && c.Transcription != nil && c.Transcription.RemoteURL == u {
			token = c.Transcription.RemoteToken
		}
		c.Transcription = &appconfig.Transcription{RemoteURL: u, RemoteToken: token}
	}
	e.probeMu.Lock()
	e.probe = RemoteStatus{}
	e.probeMu.Unlock()
	return appconfig.Save(e.data, c)
}

// Available says a voice note can be transcribed: by a transcription server
// or on this computer.
func (e *Engine) Available() bool {
	return e.Remote().URL != "" || e.Status().Ready
}

// Backend says where voice notes go: "remote" (with this computer as the
// fallback when it is set up: "remote+local"), "local", or "none".
func (e *Engine) Backend() string {
	remote, local := e.Remote().URL != "", e.Status().Ready
	switch {
	case remote && local:
		return "remote+local"
	case remote:
		return "remote"
	case local:
		return "local"
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

// RemoteStatus checks the transcription server, reusing a check made in the
// last few seconds unless fresh is set.
func (e *Engine) RemoteStatus(ctx context.Context, fresh bool) RemoteStatus {
	r := e.Remote()
	if r.URL == "" {
		return RemoteStatus{}
	}
	e.probeMu.Lock()
	cached := e.probe
	e.probeMu.Unlock()
	if !fresh && cached.URL == r.URL && time.Since(cached.CheckedAt) < probeFor {
		return cached
	}
	st := RemoteStatus{Configured: true, URL: r.URL, FromEnv: r.FromEnv, CheckedAt: time.Now()}
	h, err := probe(ctx, r)
	if err != nil {
		st.Error = err.Error()
	} else {
		st.Reachable, st.Ready, st.Accel, st.Version = true, h.Ready, h.Accel, h.Version
		if !h.Ready {
			st.Error = "the transcription server has no transcription installed: run `whatsapp-mcp transcription install` there"
		}
	}
	e.probeMu.Lock()
	e.probe = st
	e.probeMu.Unlock()
	return st
}

// health is what GET /v1/health answers.
type health struct {
	Ready   bool   `json:"ready"`
	Accel   string `json:"accel"`
	Engine  string `json:"engine"`
	Version string `json:"version"`
}

func probe(ctx context.Context, r Remote) (health, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	var h health
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL+"/v1/health", nil)
	if err != nil {
		return h, err
	}
	authorize(req, r.Token)
	resp, err := client.Do(req)
	if err != nil {
		return h, unreachable(r.URL, err)
	}
	defer resp.Body.Close()
	if err := answerError(resp); err != nil {
		return h, err
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&h); err != nil {
		return h, fmt.Errorf("the transcription server at %s answered something else: %w", r.URL, err)
	}
	return h, nil
}

func authorize(req *http.Request, token string) {
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func unreachable(u string, err error) error {
	return fmt.Errorf("the transcription server at %s did not answer: %w", u, err)
}

// answerError turns a server's refusal into an error that says what to do.
func answerError(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	var body struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body)
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return errors.New("the transcription server refused the token: copy the one `whatsapp-mcp transcription serve` printed there")
	}
	if body.Error != "" {
		return fmt.Errorf("the transcription server answered %s: %s", resp.Status, body.Error)
	}
	return fmt.Errorf("the transcription server answered %s", resp.Status)
}

// transcribeRemote sends the voice note to the transcription server.
func transcribeRemote(ctx context.Context, r Remote, audio, language, prompt string) (string, error) {
	f, err := os.Open(audio)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("language", language)
	_ = w.WriteField("prompt", prompt)
	part, err := w.CreateFormFile("audio", filepath.Base(audio))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, io.LimitReader(f, maxAudio+1)); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	if body.Len() > maxAudio {
		return "", errors.New("the voice note is too large to send to the transcription server")
	}
	ctx, cancel := context.WithTimeout(ctx, remoteTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL+"/v1/transcribe", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	authorize(req, r.Token)
	resp, err := client.Do(req)
	if err != nil {
		return "", unreachable(r.URL, err)
	}
	defer resp.Body.Close()
	if err := answerError(resp); err != nil {
		return "", err
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return "", fmt.Errorf("the transcription server at %s answered something else: %w", r.URL, err)
	}
	return out.Text, nil
}

// TranscribeAny transcribes a voice note on the transcription server when
// one is set up, and on this computer when there is none or it fails: it is
// off, asleep, slow, or refused the token. Without transcription installed
// here, the server's failure is the error.
func (e *Engine) TranscribeAny(ctx context.Context, audio, language, prompt string) (Result, error) {
	r := e.Remote()
	if r.URL != "" {
		text, err := transcribeRemote(ctx, r, audio, language, prompt)
		if err == nil {
			return Result{Text: text, Model: RemoteLabel, Backend: "remote"}, nil
		}
		if ctx.Err() != nil {
			return Result{}, err
		}
		// What failed shows in the status tools until the next check.
		e.probeMu.Lock()
		e.probe = RemoteStatus{Configured: true, URL: r.URL, FromEnv: r.FromEnv, Error: err.Error(), CheckedAt: time.Now()}
		e.probeMu.Unlock()
		if !e.Status().Ready {
			return Result{}, fmt.Errorf("%w; transcription is not installed on this computer to fall back on", err)
		}
		text, lerr := e.Transcribe(ctx, audio, language, prompt)
		if lerr != nil {
			return Result{}, fmt.Errorf("%v; on this computer: %w", err, lerr)
		}
		return Result{Text: text, Model: Label, Backend: "local", Fallback: err.Error()}, nil
	}
	text, err := e.Transcribe(ctx, audio, language, prompt)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Model: Label, Backend: "local"}, nil
}
