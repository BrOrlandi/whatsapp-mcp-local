package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/localasr"
)

func transcriptionCmd(cfg config, args []string) error {
	const usage = "usage: whatsapp-mcp transcription install|status|local|remote|openai|context|serve (whatsapp-mcp help transcription)"
	if len(args) == 0 {
		return fail(exitUsage, usage)
	}
	engine := localasr.New(cfg.DataDir)
	switch args[0] {
	case "install":
		return transcriptionInstall(engine)
	case "status":
		return transcriptionStatus(engine)
	case "local":
		if err := engine.Configure(localasr.Change{Mode: localasr.ModeLocal}); err != nil {
			return err
		}
		return transcriptionStatus(engine)
	case "remote":
		return transcriptionRemote(engine, args[1:])
	case "openai":
		return transcriptionOpenAI(engine, args[1:])
	case "context":
		return transcriptionContext(engine, args[1:])
	case "serve":
		return transcriptionServe(cfg, engine, args[1:])
	}
	return fail(exitUsage, usage)
}

func transcriptionInstall(engine *localasr.Engine) error {
	engine.Recheck()
	if st := engine.Status(); !st.Supported {
		return errors.New("local transcription is not available on this system")
	} else if st.Ready {
		fmt.Println("local transcription is already installed")
		return nil
	}
	fmt.Println("setting up local transcription (whisper.cpp, about 600 MB)…")
	defer func() {
		if st := engine.Status(); st.Unavailable != "" {
			fmt.Println(unavailableHint)
		}
	}()
	if err := engine.InstallNow(context.Background(), os.Stdout); err != nil {
		return err
	}
	fmt.Println("local transcription is ready: voice notes are now transcribed on this computer")
	return nil
}

// unavailableHint is what to do when the engine cannot run here.
const unavailableHint = "local transcription does not work on Windows yet; send voice notes to any OpenAI-compatible server (whatsapp-mcp transcription remote <address>) or to OpenAI (whatsapp-mcp transcription openai)"

func transcriptionStatus(engine *localasr.Engine) error {
	engine.Recheck()
	st := engine.Status()
	local := "not installed"
	switch {
	case st.Ready:
		local = "installed (" + st.Accel + ")"
	case st.Unavailable != "":
		local = "NOT available (" + st.Unavailable + "): " + unavailableHint
	}
	s := engine.Settings()
	fmt.Printf("mode: %s\n", s.Mode)
	fmt.Printf("on this computer: %s\n", local)
	rs := engine.RemoteStatus(context.Background(), true)
	if rs.Configured {
		what := "transcription server " + rs.URL
		if rs.Mode == localasr.ModeOpenAI {
			what = "OpenAI"
		}
		switch {
		case rs.Reachable && rs.Ready:
			var about []string
			for _, a := range []string{rs.Engine, rs.Accel, rs.Version} {
				if a != "" {
					about = append(about, a)
				}
			}
			detail := ""
			if len(about) > 0 {
				detail = " (" + strings.Join(about, ", ") + ")"
			}
			fmt.Printf("%s: answering, model %s%s\n", what, rs.Model, detail)
		default:
			fmt.Printf("%s: NOT usable: %s\n", what, rs.Error)
		}
	}
	if s.FromEnv {
		fmt.Println("  (set by " + localasr.EnvMode + " / " + localasr.EnvRemoteURL + ")")
	}
	fmt.Printf("voice notes go to: %s\n", engine.Backend())
	fmt.Printf("context sent with them: %s\n", contextWords(s))
	return nil
}

// contextWords says the context level in use and what it sends.
func contextWords(s localasr.Settings) string {
	var what string
	switch s.ContextLevel() {
	case localasr.ContextNone:
		what = "none (only the audio)"
	case localasr.ContextNames:
		what = "names (the chat's and the people's names, no messages)"
	default:
		what = "messages (the names and the latest text messages before the voice note)"
	}
	switch {
	case !s.Offsite():
		what += ", always when transcribing here"
	case s.ContextFromEnv:
		what += ", set by " + localasr.EnvContext
	case s.Context == "":
		what += ", the default for this mode"
	}
	if s.Offsite() && s.ContextLevel() != localasr.ContextNone {
		what += "; this text leaves this computer with the audio"
	}
	return what
}

// transcriptionContext sets how much of the conversation goes with each
// voice note: none, names, messages, or auto (the mode's default).
func transcriptionContext(engine *localasr.Engine, args []string) error {
	if len(args) != 1 {
		return fail(exitUsage, "usage: whatsapp-mcp transcription context none|names|messages|auto")
	}
	if err := engine.SetContext(args[0]); err != nil {
		if errors.Is(err, localasr.ErrContextFromEnv) || errors.Is(err, localasr.ErrContextLocal) {
			return err
		}
		return fail(exitUsage, "%v", err)
	}
	fmt.Printf("context sent with voice notes: %s\n", contextWords(engine.Settings()))
	return nil
}

// readSecret reads a token or an API key: without showing it in a terminal,
// else one line from the standard input. It is never a command-line
// argument, which other users of the computer can see. Empty keeps the one
// kept.
func readSecret(prompt string) (string, error) {
	if !isTerminal(os.Stdin) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		return strings.TrimSpace(line), nil
	}
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return strings.TrimSpace(string(b)), err
}

func transcriptionRemote(engine *localasr.Engine, args []string) error {
	fs := flag.NewFlagSet("transcription remote", flag.ContinueOnError)
	model := fs.String("model", "", "the model to ask the server for (default whisper-1)")
	var addr string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		addr, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return fail(exitUsage, "%v", err)
	}
	if addr == "" && fs.NArg() > 0 {
		addr = fs.Arg(0)
	}
	if addr == "" {
		return fail(exitUsage, "usage: whatsapp-mcp transcription remote <address> [--model <model>] (the token is read from the standard input) | off")
	}
	if addr == "off" {
		if err := engine.Configure(localasr.Change{Mode: localasr.ModeLocal}); err != nil {
			return err
		}
		fmt.Println("voice notes are transcribed on this computer")
		return nil
	}
	token, err := readSecret("token (empty: keep the one kept, or none): ")
	if err != nil {
		return err
	}
	if err := engine.Configure(localasr.Change{Mode: localasr.ModeRemote, URL: addr, Token: token, Model: *model}); err != nil {
		return err
	}
	return transcriptionStatus(engine)
}

func transcriptionOpenAI(engine *localasr.Engine, args []string) error {
	fs := flag.NewFlagSet("transcription openai", flag.ContinueOnError)
	model := fs.String("model", "", "whisper-1 (default), gpt-4o-transcribe or gpt-4o-mini-transcribe")
	if err := fs.Parse(args); err != nil {
		return fail(exitUsage, "%v", err)
	}
	key, err := readSecret("OpenAI API key (empty: keep the one kept): ")
	if err != nil {
		return err
	}
	if err := engine.Configure(localasr.Change{Mode: localasr.ModeOpenAI, Token: key, Model: *model}); err != nil {
		return err
	}
	return transcriptionStatus(engine)
}

func transcriptionServe(cfg config, engine *localasr.Engine, args []string) error {
	fs := flag.NewFlagSet("transcription serve", flag.ContinueOnError)
	listen := fs.String("listen", "0.0.0.0:47823", "the address and port to listen on")
	tokenFile := fs.String("token-file", "", "the file holding the token requests must carry (default: transcription-server.token in the data folder, made the first time)")
	if err := fs.Parse(args); err != nil {
		return fail(exitUsage, "%v", err)
	}
	if st := engine.Status(); st.Unavailable != "" {
		return errors.New("local transcription is not available on this computer (" + st.Unavailable + "): it does not work on Windows yet, so this computer cannot serve it")
	} else if !st.Ready {
		return errors.New("transcription is not installed on this computer: run whatsapp-mcp transcription install first")
	}
	token := strings.TrimSpace(os.Getenv(localasr.EnvRemoteToken))
	where := localasr.EnvRemoteToken
	if token == "" {
		path := *tokenFile
		if path == "" {
			path = filepath.Join(cfg.DataDir, "transcription-server.token")
		}
		t, err := serverToken(path)
		if err != nil {
			return err
		}
		token, where = t, path
	}
	if len(token) < 16 {
		return fail(exitUsage, "the token must have at least 16 characters")
	}
	// Each voice note goes through a temporary folder. The Docker image
	// points TMPDIR at /data/tmp, which only the daemon's serve made, so
	// without this every transcription failed there.
	if err := os.MkdirAll(os.TempDir(), 0o700); err != nil {
		return fmt.Errorf("the folder for temporary files (%s): %w", os.TempDir(), err)
	}
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	srv := engine.Server("", token, version)
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	fmt.Printf("transcription server (OpenAI API) listening on %s (%s)\n", ln.Addr(), engine.Status().Accel)
	// Only an address that answers: the network ones when it listens on
	// all of them, else the one it was given.
	addrs := []string{ln.Addr().String()}
	if host, _, _ := net.SplitHostPort(ln.Addr().String()); net.ParseIP(host) != nil && net.ParseIP(host).IsUnspecified() {
		addrs = nil
		for _, a := range lanAddresses() {
			addrs = append(addrs, net.JoinHostPort(a, port))
		}
	}
	if len(addrs) == 1 {
		fmt.Println("on the computer that sends the voice notes, point WhatsApp MCP at it, with this address:")
	} else {
		fmt.Println("on the computer that sends the voice notes, point WhatsApp MCP at it, with one of these addresses:")
	}
	for _, a := range addrs {
		fmt.Printf("  http://%s\n", a)
	}
	// The token itself is never printed: this output often goes to a log.
	fmt.Printf("token: the one in %s\n", where)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// serverToken is the transcription server's token, kept in a file (only
// its owner can read it) so it survives restarts; it is made the first time.
func serverToken(path string) (string, error) {
	if raw, err := os.ReadFile(path); err == nil {
		if t := strings.TrimSpace(string(raw)); t != "" {
			_ = os.Chmod(path, 0o600)
			return t, nil
		}
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	t := hex.EncodeToString(b)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(t+"\n"), 0o600); err != nil {
		return "", err
	}
	return t, nil
}

// lanAddresses are this computer's IPv4 addresses on its networks, for the
// transcription server to print.
func lanAddresses() []string {
	var out []string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && !n.IP.IsLoopback() && n.IP.To4() != nil && !n.IP.IsLinkLocalUnicast() {
			out = append(out, n.IP.String())
		}
	}
	if len(out) == 0 {
		out = append(out, "127.0.0.1")
	}
	return out
}
