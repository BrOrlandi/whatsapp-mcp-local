package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/public"
)

// readPassword asks for the panel's password twice, without showing it, in
// a terminal; from a pipe it reads one line. It is never a command-line
// argument, which other users of the computer can see. Tests replace it.
var readPassword = func() (string, error) {
	if !isTerminal(os.Stdin) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	ask := func(prompt string) (string, error) {
		fmt.Fprint(os.Stderr, prompt)
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return string(b), err
	}
	first, err := ask(i18n.T("Senha do painel: "))
	if err != nil {
		return "", err
	}
	again, err := ask(i18n.T("De novo, para conferir: "))
	if err != nil {
		return "", err
	}
	if first != again {
		return "", fail(exitUsage, "%s", i18n.T("as duas senhas não são iguais"))
	}
	return first, nil
}

// passwordCmd is the server mode's panel password: `hash` prints the bcrypt
// hash for WHATSAPP_MCP_PANEL_PASSWORD_HASH, `set` keeps it in the data
// folder. Neither prints the password, and a new one signs every panel
// session out.
func passwordCmd(cfg config, args []string) error {
	f := newFlags("password")
	rest, err := f.parse(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 || (rest[0] != "hash" && rest[0] != "set") {
		return fail(exitUsage, "%s", i18n.T("uso: whatsapp-mcp password hash|set"))
	}
	if rest[0] == "set" && os.Getenv(public.EnvPasswordHash) != "" {
		return fail(exitConflict, "%s", i18n.Tf("a senha do painel está definida pela variável de ambiente %s; mude a variável", public.EnvPasswordHash))
	}
	password, err := readPassword()
	if err != nil {
		return err
	}
	hash, err := public.HashPassword(password)
	if err != nil {
		return fail(exitUsage, "%v", err)
	}
	if rest[0] == "hash" {
		if jsonOutput {
			printJSON(map[string]string{"hash": hash})
		} else {
			fmt.Fprintln(stdout, hash)
		}
		return nil
	}
	if err := writePasswordHash(cfg.DataDir, hash); err != nil {
		return err
	}
	if jsonOutput {
		printJSON(map[string]bool{"ok": true})
		return nil
	}
	say("%s", i18n.T("Senha do painel guardada. Quem estava no painel pelo endereço do servidor precisa entrar de novo."))
	return nil
}

func writePasswordHash(dataDir, hash string) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	path := public.PasswordPath(dataDir)
	tmp := path + ".new"
	if err := os.WriteFile(tmp, []byte(hash+"\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
