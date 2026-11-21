package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/localasr"
)

func transcriptionCmd(cfg config, args []string) error {
	if len(args) == 0 {
		return fail(exitUsage, "usage: whatsapp-mcp transcription install|status|remote|serve (whatsapp-mcp help transcription)")
	}
	engine := localasr.New(cfg.DataDir)
	switch args[0] {
	case "install":
		return transcriptionInstall(engine)
	case "status":
		return transcriptionStatus(engine)
	case "remote":
		return transcriptionRemote(engine, args[1:])
	case "serve":
		return transcriptionServe(cfg, engine, args[1:])
	}
	return fail(exitUsage, "usage: whatsapp-mcp transcription install|status|remote|serve (whatsapp-mcp help transcription)")
}

func transcriptionInstall(engine *localasr.Engine) error {
	if st := engine.Status(); !st.Supported {
		return errors.New("local transcription is not available on this system")
	} else if st.Ready {
		fmt.Println("local transcription is already installed")
		return nil
	}
	fmt.Println("setting up local transcription (whisper.cpp, about 600 MB)…")
	if err := engine.InstallNow(context.Background(), os.Stdout); err != nil {
		return err
	}
	fmt.Println("local transcription is ready: voice notes are now transcribed on this computer")
	return nil
}

func transcriptionStatus(engine *localasr.Engine) error {
	st := engine.Status()
	local := "not installed"
	if st.Ready {
		local = "installed (" + st.Accel + ")"
	}
	fmt.Printf("on this computer: %s\n", local)
	rs := engine.RemoteStatus(context.Background(), true)
	switch {
	case !rs.Configured:
		fmt.Println("transcription server: none")
	case rs.Reachable && rs.Ready:
		fmt.Printf("transcription server: %s, answering (%s, version %s)\n", rs.URL, rs.Accel, rs.Version)
	default:
		fmt.Printf("transcription server: %s, NOT usable: %s\n", rs.URL, rs.Error)
	}
	if rs.FromEnv {
		fmt.Println("  (set by " + localasr.EnvRemoteURL + ")")
	}
	fmt.Printf("voice notes go to: %s\n", engine.Backend())
	return nil
}

func transcriptionRemote(engine *localasr.Engine, args []string) error {
	fs := flag.NewFlagSet("transcription remote", flag.ContinueOnError)
	token := fs.String("token", "", "the token the transcription server printed")
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
		return fail(exitUsage, "usage: whatsapp-mcp transcription remote <address> [--token <token>] | off")
	}
	if addr == "off" {
		addr = ""
	}
	if err := engine.SetRemote(addr, *token); err != nil {
		return err
	}
	if addr == "" {
		fmt.Println("no transcription server: voice notes are transcribed on this computer")
		return nil
	}
	return transcriptionStatus(engine)
}

func transcriptionServe(cfg config, engine *localasr.Engine, args []string) error {
	fs := flag.NewFlagSet("transcription serve", flag.ContinueOnError)
	listen := fs.String("listen", "0.0.0.0:47823", "the address and port to listen on")
	token := fs.String("token", "", "the token requests must carry (default: kept in the data folder)")
	if err := fs.Parse(args); err != nil {
		return fail(exitUsage, "%v", err)
	}
	if !engine.Status().Ready {
		return errors.New("transcription is not installed on this computer: run whatsapp-mcp transcription install first")
	}
	if *token == "" {
		*token = strings.TrimSpace(os.Getenv(localasr.EnvRemoteToken))
	}
	if *token == "" {
		t, err := serverToken(cfg.DataDir)
		if err != nil {
			return err
		}
		*token = t
	}
	if len(*token) < 16 {
		return fail(exitUsage, "the token must have at least 16 characters")
	}
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	srv := engine.Server("", *token, version)
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	fmt.Printf("transcription server listening on %s (%s)\n", ln.Addr(), engine.Status().Accel)
	fmt.Println("on the other computer, point WhatsApp MCP at it, with one of these addresses:")
	for _, a := range lanAddresses() {
		fmt.Printf("  http://%s\n", net.JoinHostPort(a, port))
	}
	fmt.Printf("token: %s\n", *token)
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

// serverToken is the transcription server's token, kept in the data folder
// so it survives restarts; it is made the first time.
func serverToken(dataDir string) (string, error) {
	path := filepath.Join(dataDir, "transcription-server.token")
	if raw, err := os.ReadFile(path); err == nil {
		if t := strings.TrimSpace(string(raw)); t != "" {
			return t, nil
		}
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	t := hex.EncodeToString(b)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
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
