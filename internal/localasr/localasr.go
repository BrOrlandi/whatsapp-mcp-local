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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

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

type Engine struct {
	data string // the data folder: the programs go in bin/, the model in models/
	dir  string // where the model lives

	mu      sync.Mutex
	install Install
	busy    sync.Mutex // one transcription at a time: the GPU is shared
}

// Install is the state of setting the engine up, for the panel.
type Install struct {
	State      string    `json:"state"` // idle, running, done, error
	Step       string    `json:"step,omitempty"`
	Downloaded int64     `json:"downloaded,omitempty"`
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
	Install Install  `json:"install"`
}

func New(dataDir string) *Engine {
	return &Engine{data: dataDir, dir: filepath.Join(dataDir, "models")}
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
	s.Ready = len(s.Missing) == 0
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
// from this project's release when missing, then the model.
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
			return fmt.Errorf("ainda não há transcrição local publicada para %s/%s", runtime.GOOS, runtime.GOARCH)
		}
		e.setInstall(func(i *Install) { i.Step, i.Downloaded, i.Total = "Baixando o whisper.cpp", 0, pkg.Size })
		err := sidecar.Install(ctx, e.data, pkg, func(done, total int64) {
			e.setInstall(func(i *Install) { i.Downloaded, i.Total = done, total })
		})
		if err != nil {
			return err
		}
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
	e.setInstall(func(i *Install) { i.Step, i.Total = "Baixando o modelo de transcrição", modelSize })
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ModelURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("não foi possível baixar o modelo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("o download do modelo respondeu %s", resp.Status)
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
			return fmt.Errorf("o download do modelo parou: %w", rerr)
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != modelSHA256 {
		return fmt.Errorf("o modelo baixado não confere (%d bytes, sha256 %s); tente de novo", n, got)
	}
	return os.Rename(tmp, e.modelPath())
}

// Transcribe turns an audio file into text. prompt is the conversation's
// context, which Whisper uses to spell names and terms; language is an
// ISO-639-1 code or empty for automatic detection.
func (e *Engine) Transcribe(ctx context.Context, audio, language, prompt string) (string, error) {
	s := e.Status()
	if !s.Ready {
		return "", fmt.Errorf("a transcrição local não está instalada (falta: %s)", strings.Join(s.Missing, ", "))
	}
	e.busy.Lock()
	defer e.busy.Unlock()

	dir, err := os.MkdirTemp("", "wamcp-asr-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	wav := filepath.Join(dir, "audio.wav")
	// WhatsApp voice notes are Ogg Opus; whisper.cpp wants 16 kHz mono PCM.
	conv := exec.CommandContext(ctx, s.FFmpeg, "-nostdin", "-loglevel", "error", "-i", audio, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wav)
	platform.Background(conv)
	if out, err := conv.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg não conseguiu ler o áudio: %s", lastLines(string(out), 2))
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
	args := []string{"-m", s.Model, "-f", wav, "-l", language, "-otxt", "-of", base, "-np", "-nt",
		"-t", fmt.Sprint(threads)}
	if p := strings.TrimSpace(prompt); p != "" {
		args = append(args, "--prompt", p)
	}
	run := exec.CommandContext(ctx, s.Whisper, args...)
	platform.Background(run)
	if out, err := run.CombinedOutput(); err != nil {
		return "", fmt.Errorf("whisper.cpp falhou: %s", lastLines(string(out), 3))
	}
	text, err := os.ReadFile(base + ".txt")
	if err != nil {
		return "", fmt.Errorf("whisper.cpp não devolveu texto: %w", err)
	}
	return cleanTranscript(string(text)), nil
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
