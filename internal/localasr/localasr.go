// Package localasr transcribes voice notes on this computer with whisper.cpp,
// the way Handy does: no audio leaves the machine and nothing is billed. The
// programs come from this project's release (internal/sidecar) and the model
// from Hugging Face, both checked by sha256, the first time it is turned on.
//
// Whisper rather than Parakeet because Whisper takes an initial prompt, and the
// prompt is where the conversation's context goes: the chat's name, the people
// in it, what was just written. Names and terms the speaker uses come out
// spelled the way the chat spells them instead of the way they sound.
package localasr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/platform"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/sidecar"
)

const (
	// ModelName is Whisper large-v3-turbo, quantised: about 574 MB, fast on
	// Apple Silicon through Metal, and good at Portuguese.
	ModelName = "ggml-large-v3-turbo-q5_0.bin"
	ModelURL  = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/" + ModelName
	modelSize = 574_041_195
	// modelSHA256 is the file Hugging Face serves under that name.
	modelSHA256 = "394221709cd5ad1f40c46e6031ca61bce88931e6e088c188294c6d5a55ffa7e2"
	// Label says which engine wrote a transcript.
	Label = "whisper.cpp large-v3-turbo (local)"
)

// modelURL is where the model comes from; tests point it elsewhere.
var modelURL = ModelURL

// RunTimeout is the most one transcription may take on this computer once it
// starts. A CPU without a GPU the engine uses takes about as long as the audio
// lasts, often more, so it is generous: what bounds the wait of an AI tool is
// the MCP's own limit (mcp.Server), not this.
var RunTimeout = time.Hour

type Engine struct {
	data string // the data folder: the programs go in bin/, the model in models/
	dir  string // where the model lives

	// envContextIgnored logs once that WHATSAPP_MCP_TRANSCRIPTION_CONTEXT
	// does nothing while voice notes are transcribed here.
	envContextIgnored sync.Once
	mu                sync.Mutex
	install           Install
	// slot lets one transcription run at a time (the GPU, or the CPU, is
	// shared); a caller waits for it only as long as its context allows.
	slot chan struct{}

	probeMu sync.Mutex
	probe   RemoteStatus // the last check of the remote server, see RemoteStatus

	testMu sync.Mutex
	tested selfTestResult // the last run of the engine, on Windows (selftest.go)
}

// Install is the state of setting the engine up, for the panel.
type Install struct {
	State      string    `json:"state"` // idle, running, done, error
	Step       string    `json:"step,omitempty"`
	Downloaded int64     `json:"downloaded"`
	Total      int64     `json:"total,omitempty"`
	Error      string    `json:"error,omitempty"`
	At         time.Time `json:"at,omitempty"`
}

// Status says whether local transcription can run here.
type Status struct {
	// Supported is set where there is something to install: a published
	// package for this system, or the programs already on the computer.
	Supported bool   `json:"supported"`
	Ready     bool   `json:"ready"`
	Whisper   string `json:"whisper,omitempty"`
	FFmpeg    string `json:"ffmpeg,omitempty"`
	Model     string `json:"model,omitempty"`
	// Accel is how whisper runs: metal (an Apple GPU), cuda (an NVIDIA GPU)
	// or cpu, where a voice note takes about as long as it lasts.
	Accel   string   `json:"accel,omitempty"`
	Missing []string `json:"missing,omitempty"`
	// Unavailable is set when the engine is installed but does not run on
	// this computer (UnavailableWindowsRuntime, UnavailableEngine); Ready is
	// then false and Supported stays true.
	Unavailable string  `json:"unavailable,omitempty"`
	Install     Install `json:"install"`
}

func New(dataDir string) *Engine {
	return &Engine{data: dataDir, dir: filepath.Join(dataDir, "models"), slot: make(chan struct{}, 1)}
}

// tools finds the programs: the ones this app installed, checked against
// their recorded sha256, or else ones already on the computer (Homebrew,
// a package manager), which the command line has always accepted.
func (e *Engine) tools() (sidecar.Tools, error) {
	if t, err := sidecar.Installed(e.data); err == nil {
		return t, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return sidecar.Tools{}, err
	}
	return systemTools(), nil
}

// systemTools finds the programs already on the computer. It is a variable
// so tests can leave them out.
var systemTools = func() sidecar.Tools {
	t := sidecar.Tools{Whisper: platform.LookPath("whisper-cli"), FFmpeg: platform.LookPath("ffmpeg"), Accel: "cpu"}
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		t.Accel = "metal" // Homebrew's whisper.cpp is built with Metal
	}
	return t
}

func (e *Engine) modelPath() string { return filepath.Join(e.dir, ModelName) }

func (e *Engine) Status() Status {
	t, toolsErr := e.tools()
	pkg, published := sidecar.ForThisSystem()
	s := Status{Whisper: t.Whisper, FFmpeg: t.FFmpeg, Accel: t.Accel}
	if s.Whisper == "" && published {
		s.Accel = pkg.Accel
	}
	s.Supported = published || (s.Whisper != "" && s.FFmpeg != "")
	if info, err := os.Stat(e.modelPath()); err == nil && info.Size() > modelSize/2 {
		s.Model = e.modelPath()
	}
	if s.Whisper == "" {
		s.Missing = append(s.Missing, "whisper.cpp")
	}
	if s.FFmpeg == "" {
		s.Missing = append(s.Missing, "ffmpeg")
	}
	if s.Model == "" {
		s.Missing = append(s.Missing, "modelo")
	}
	s.Unavailable = e.unavailable(s.Whisper)
	s.Ready = len(s.Missing) == 0 && s.Unavailable == ""
	e.mu.Lock()
	s.Install = e.install
	e.mu.Unlock()
	if s.Install.State == "" {
		s.Install.State = "idle"
	}
	if toolsErr != nil && s.Install.State != "running" {
		s.Install.State, s.Install.Error = "error", toolsErr.Error()
	}
	return s
}

func (e *Engine) setInstall(f func(*Install)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	f(&e.install)
	e.install.At = time.Now()
}

var ErrInstalling = errors.New("the installation is already running")

// StartInstall sets the engine up in the background: whisper-cli and ffmpeg
// from this project's release when missing, a test that they run (on
// Windows), then the model.
func (e *Engine) StartInstall() error {
	e.mu.Lock()
	if e.install.State == "running" {
		e.mu.Unlock()
		return ErrInstalling
	}
	e.install = Install{State: "running", At: time.Now()}
	e.mu.Unlock()
	go func() {
		err := e.installAll(context.Background())
		if err != nil { // the panel says it in its own words; the original is here
			log.Printf("transcription: the install failed: %v", err)
		}
		e.setInstall(func(i *Install) {
			if err != nil {
				i.State, i.Error = "error", err.Error()
			} else {
				i.State, i.Step = "done", ""
			}
		})
	}()
	return nil
}

// InstallNow sets the engine up and waits, printing progress: for the
// installer script.
func (e *Engine) InstallNow(ctx context.Context, out io.Writer) error {
	done := make(chan error, 1)
	go func() { done <- e.installAll(ctx) }()
	last := ""
	for {
		select {
		case err := <-done:
			fmt.Fprintln(out)
			return err
		case <-time.After(time.Second):
			st := e.Status().Install
			line := st.Step
			if st.Total > 0 {
				line = fmt.Sprintf("%s %d%%", st.Step, st.Downloaded*100/st.Total)
			}
			if line != last {
				fmt.Fprintf(out, "\r  %s   ", line)
				last = line
			}
		}
	}
}

func (e *Engine) installAll(ctx context.Context) error {
	s := e.Status()
	if s.Whisper == "" || s.FFmpeg == "" || s.Install.Error != "" {
		pkg, ok := sidecar.ForThisSystem()
		if !ok {
			return errors.New(i18n.Tf("ainda não há transcrição local publicada para %s/%s", runtime.GOOS, runtime.GOARCH))
		}
		e.setInstall(func(i *Install) {
			i.Step, i.Downloaded, i.Total = i18n.T("Baixando o programa de transcrição"), 0, pkg.Size
		})
		err := sidecar.Install(ctx, e.data, pkg, func(done, total int64) {
			e.setInstall(func(i *Install) { i.Downloaded, i.Total = done, total })
		})
		if err != nil {
			return err
		}
	}
	// The small package is tested before the large model is fetched: on a
	// computer where the engine cannot run, the model would be 574 MB for
	// nothing.
	if s = e.Status(); s.Unavailable != "" {
		return unavailableError(s.Unavailable)
	}
	if s.Model == "" {
		if err := e.download(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) download(ctx context.Context) error {
	if err := os.MkdirAll(e.dir, 0o755); err != nil {
		return err
	}
	e.setInstall(func(i *Install) { i.Step, i.Total = i18n.T("Baixando o modelo de transcrição"), modelSize })
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("não foi possível baixar o modelo"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New(i18n.Tf("o download do modelo respondeu %s", resp.Status))
	}
	if resp.ContentLength > 0 {
		e.setInstall(func(i *Install) { i.Total = resp.ContentLength })
	}
	tmp := e.modelPath() + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	h := sha256.New()
	buf := make([]byte, 1<<20)
	var n int64
	for {
		k, rerr := resp.Body.Read(buf)
		if k > 0 {
			if _, err := f.Write(buf[:k]); err != nil {
				f.Close()
				return err
			}
			h.Write(buf[:k])
			n += int64(k)
			got := n
			e.setInstall(func(i *Install) { i.Downloaded = got })
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return fmt.Errorf("%s: %w", i18n.T("o download do modelo parou"), rerr)
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != modelSHA256 {
		return errors.New(i18n.Tf("o modelo baixado não confere (%d bytes, sha256 %s); tente de novo", n, got))
	}
	return os.Rename(tmp, e.modelPath())
}

// Transcribe turns an audio file into text. prompt is the conversation's
// context, which Whisper uses to spell names and terms; language is an
// ISO-639-1 code or empty for automatic detection. Its errors reach the AI
// through transcribe_audio, so they do not follow the panel's language.
func (e *Engine) Transcribe(ctx context.Context, audio, language, prompt string) (string, error) {
	text, _, err := e.transcribe(ctx, audio, language, prompt)
	return text, err
}

// transcribe is Transcribe that also says how long the voice note is, in
// seconds, for the transcription server's verbose answer.
func (e *Engine) transcribe(ctx context.Context, audio, language, prompt string) (string, float64, error) {
	s := e.Status()
	if s.Unavailable != "" {
		return "", 0, unavailableError(s.Unavailable)
	}
	if !s.Ready {
		return "", 0, fmt.Errorf("a transcrição local não está instalada (falta: %s)", strings.Join(s.Missing, ", "))
	}
	select {
	case e.slot <- struct{}{}:
	case <-ctx.Done():
		return "", 0, fmt.Errorf("the transcription waited for its turn and gave up: %w", ctx.Err())
	}
	defer func() { <-e.slot }()
	// The run's own limit starts once it has the engine, not while it waits:
	// on a slow processor a long voice note takes many minutes.
	ctx, cancel := context.WithTimeout(ctx, RunTimeout)
	defer cancel()

	dir, err := os.MkdirTemp("", "wamcp-asr-")
	if err != nil {
		return "", 0, err
	}
	defer os.RemoveAll(dir)
	wav := filepath.Join(dir, "audio.wav")
	// WhatsApp voice notes are Ogg Opus; whisper.cpp wants 16 kHz mono PCM.
	conv := exec.CommandContext(ctx, s.FFmpeg, "-nostdin", "-loglevel", "error", "-i", audio, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wav)
	platform.Background(conv)
	if out, err := conv.CombinedOutput(); err != nil {
		if missingRuntime(err) {
			return "", 0, fmt.Errorf("%w: ffmpeg cannot start (%s)", ErrUnavailable, exitDetail(err))
		}
		return "", 0, fmt.Errorf("ffmpeg não conseguiu ler o áudio: %s", failureText(out, err, 2))
	}
	if language == "" {
		language = "auto"
	}
	base := filepath.Join(dir, "out")
	// On a GPU the threads only feed it; on the CPU they do the work.
	threads := max(2, runtime.NumCPU()/2)
	if s.Accel == "cpu" {
		threads = max(2, runtime.NumCPU()-1)
	}
	// No -nt: without timestamps whisper.cpp moves on a whole 30-second window
	// each time, and whatever the window held after the model stopped is lost
	// (a third of an 8-minute voice note). The .txt has no timestamps anyway.
	args := []string{"-m", s.Model, "-f", wav, "-l", language, "-otxt", "-of", base, "-np",
		"-t", fmt.Sprint(threads)}
	if p := strings.TrimSpace(prompt); p != "" {
		args = append(args, "--prompt", p)
	}
	run := exec.CommandContext(ctx, s.Whisper, args...)
	platform.Background(run)
	if out, err := run.CombinedOutput(); err != nil {
		if missingRuntime(err) {
			log.Printf("transcription: the engine does not start on this computer (%s)", exitDetail(err))
			e.markUnavailable(s.Whisper, UnavailableWindowsRuntime)
			return "", 0, unavailableError(UnavailableWindowsRuntime)
		}
		return "", 0, fmt.Errorf("whisper.cpp falhou: %s", failureText(out, err, 3))
	}
	text, err := os.ReadFile(base + ".txt")
	if err != nil {
		return "", 0, fmt.Errorf("whisper.cpp não devolveu texto: %w", err)
	}
	// 16 kHz mono, 16 bits a sample: 32000 bytes a second after the header.
	var seconds float64
	if fi, err := os.Stat(wav); err == nil && fi.Size() > 44 {
		seconds = float64(fi.Size()-44) / 32000
	}
	return cleanTranscript(string(text)), seconds, nil
}

// cleanTranscript joins Whisper's segments into one paragraph and drops the
// markers it writes for silence.
func cleanTranscript(s string) string {
	var parts []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "[BLANK_AUDIO]" || line == "[MUSIC]" {
			continue
		}
		parts = append(parts, line)
	}
	return strings.Join(parts, " ")
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " ")
}
