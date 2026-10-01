// Package localasr transcribes voice notes on this computer with whisper.cpp,
// the way Handy does: no audio leaves the machine and nothing is billed.
//
// Whisper rather than Parakeet because Whisper takes an initial prompt, and the
// prompt is where the conversation's context goes: the chat's name, the people
// in it, what was just written. Names and terms the speaker uses come out
// spelled the way the chat spells them instead of the way they sound.
package localasr

import (
	"context"
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
)

const (
	// ModelName is Whisper large-v3-turbo, quantised: about 574 MB, fast on
	// Apple Silicon through Metal, and good at Portuguese.
	ModelName = "ggml-large-v3-turbo-q5_0.bin"
	ModelURL  = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/" + ModelName
	modelSize = 574_041_195
	// Label says which engine wrote a transcript.
	Label = "whisper.cpp large-v3-turbo (local)"
)

type Engine struct {
	dir string // where the model lives

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
	Supported bool     `json:"supported"` // Apple Silicon
	Ready     bool     `json:"ready"`
	Whisper   string   `json:"whisper,omitempty"`
	FFmpeg    string   `json:"ffmpeg,omitempty"`
	Model     string   `json:"model,omitempty"`
	Brew      bool     `json:"brew"`
	Missing   []string `json:"missing,omitempty"`
	Install   Install  `json:"install"`
}

func New(dataDir string) *Engine {
	return &Engine{dir: filepath.Join(dataDir, "models")}
}

func find(names ...string) string {
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
		for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin"} {
			p := filepath.Join(dir, n)
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				return p
			}
		}
	}
	return ""
}

func (e *Engine) modelPath() string { return filepath.Join(e.dir, ModelName) }

func (e *Engine) Status() Status {
	s := Status{
		Supported: runtime.GOOS == "darwin" && runtime.GOARCH == "arm64",
		Whisper:   find("whisper-cli"),
		FFmpeg:    find("ffmpeg"),
		Brew:      find("brew") != "",
	}
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
	return s
}

func (e *Engine) setInstall(f func(*Install)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	f(&e.install)
	e.install.At = time.Now()
}

var ErrInstalling = errors.New("the installation is already running")

// StartInstall sets the engine up in the background: whisper.cpp and ffmpeg
// through Homebrew when missing, then the model.
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
	if !s.Supported {
		return errors.New("a transcrição local é feita para Macs com Apple Silicon")
	}
	var pkgs []string
	if s.Whisper == "" {
		pkgs = append(pkgs, "whisper-cpp")
	}
	if s.FFmpeg == "" {
		pkgs = append(pkgs, "ffmpeg")
	}
	if len(pkgs) > 0 {
		brew := find("brew")
		if brew == "" {
			return fmt.Errorf("instale o Homebrew (https://brew.sh) e tente de novo; faltam: %s", strings.Join(pkgs, ", "))
		}
		e.setInstall(func(i *Install) { i.Step = "Instalando " + strings.Join(pkgs, " e ") + " pelo Homebrew" })
		cmd := exec.CommandContext(ctx, brew, append([]string{"install"}, pkgs...)...)
		cmd.Env = append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_INSTALL_CLEANUP=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("brew install %s falhou: %s", strings.Join(pkgs, " "), lastLines(string(out), 3))
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
	buf := make([]byte, 1<<20)
	var n int64
	for {
		k, rerr := resp.Body.Read(buf)
		if k > 0 {
			if _, err := f.Write(buf[:k]); err != nil {
				f.Close()
				return err
			}
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
	if n < modelSize/2 {
		return fmt.Errorf("o modelo baixado está incompleto (%d bytes)", n)
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
	if out, err := conv.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg não conseguiu ler o áudio: %s", lastLines(string(out), 2))
	}
	if language == "" {
		language = "auto"
	}
	base := filepath.Join(dir, "out")
	args := []string{"-m", s.Model, "-f", wav, "-l", language, "-otxt", "-of", base, "-np", "-nt",
		"-t", fmt.Sprint(max(2, runtime.NumCPU()/2))}
	if p := strings.TrimSpace(prompt); p != "" {
		args = append(args, "--prompt", p)
	}
	run := exec.CommandContext(ctx, s.Whisper, args...)
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
