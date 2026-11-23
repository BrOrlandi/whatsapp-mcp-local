package mcp

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The suggested command is run by agents as given, and the file name comes
// from whoever sent the message: run in a shell, it saves a plain name and
// runs nothing else.
func TestTheCurlHintRunsNothingElse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	url := "https://wa.example.com/media/0123456789abcdef"
	for _, name := range []string{
		"3EB0ABC.$(touch PWNED)",
		"nota.`touch PWNED`",
		"x';touch PWNED;'",
		"-oPWNED --config=/etc/passwd",
		"x\"; touch PWNED; \"",
		"a b|touch PWNED&",
		"..",
	} {
		dir := t.TempDir()
		hint := curlHint(name, url)
		// curl stands in as a function that prints its arguments, one a line.
		cmd := exec.Command("sh", "-c", `curl() { for a in "$@"; do printf '%s\n' "$a"; done; }; `+hint)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%q: %v\n%s", hint, err, out)
		}
		if _, err := os.Stat(filepath.Join(dir, "PWNED")); err == nil {
			t.Fatalf("%q ran a command", hint)
		}
		args := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
		if len(args) != 4 || args[0] != "-sSf" || args[1] != "-o" || args[3] != url {
			t.Fatalf("%q gave the arguments %q", hint, args)
		}
		for _, r := range args[2] {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-", r)) {
				t.Fatalf("%q saves to %q", hint, args[2])
			}
		}
		if strings.HasPrefix(args[2], "-") || strings.HasPrefix(args[2], ".") {
			t.Fatalf("%q saves to %q", hint, args[2])
		}
	}
	// A plain name and link need no quotes, so the line works in cmd.exe and
	// PowerShell as well.
	if got := curlHint("3EB0ABC.pdf", "http://127.0.0.1:47821/media/ab12"); got != "curl -sSf -o 3EB0ABC.pdf http://127.0.0.1:47821/media/ab12" {
		t.Fatalf("a plain hint: %q", got)
	}
}
