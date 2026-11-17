package main

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/public"
)

func withPassword(t *testing.T, password string) {
	t.Helper()
	old := readPassword
	readPassword = func() (string, error) { return password, nil }
	t.Cleanup(func() { readPassword = old })
}

func TestPasswordHashPrintsTheHashNeverThePassword(t *testing.T) {
	const pw = "a throwaway password"
	withPassword(t, pw)
	out, err := run(t, passwordCmd, config{DataDir: t.TempDir()}, "hash")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, pw) {
		t.Fatal("the password was printed")
	}
	hash := strings.TrimSpace(out)
	if err := public.CheckHash(hash); err != nil {
		t.Fatalf("%q: %v", hash, err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) != nil {
		t.Fatal("the hash is not of the password")
	}
}

func TestPasswordSetKeepsTheHashInTheDataFolder(t *testing.T) {
	const pw = "another throwaway one"
	withPassword(t, pw)
	dir := t.TempDir()
	out, err := run(t, passwordCmd, config{DataDir: dir}, "set")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, pw) || strings.Contains(out, "$2") {
		t.Fatalf("set printed a secret: %q", out)
	}
	raw, err := os.ReadFile(public.PasswordPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(strings.TrimSpace(string(raw))), []byte(pw)) != nil {
		t.Fatal("the file does not hold the password's hash")
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(public.PasswordPath(dir)); fi.Mode().Perm() != 0o600 {
			t.Fatalf("the file is %v", fi.Mode().Perm())
		}
	}
	// The server mode then finds it.
	vars := map[string]string{public.EnvURL: "https://wa.example.com"}
	if c, err := public.FromEnv(func(k string) string { return vars[k] }, dir); err != nil || c == nil {
		t.Fatalf("the server mode does not find the password: %v", err)
	}
}

func TestPasswordRefusals(t *testing.T) {
	withPassword(t, "short")
	if _, err := run(t, passwordCmd, config{DataDir: t.TempDir()}, "hash"); exitCode(err) != exitUsage {
		t.Errorf("a short password: %v", err)
	}
	withPassword(t, "a long enough password")
	for _, args := range [][]string{nil, {"show"}, {"set", "a long enough password"}} {
		if _, err := run(t, passwordCmd, config{DataDir: t.TempDir()}, args...); exitCode(err) != exitUsage {
			t.Errorf("password %q: %v", args, err)
		}
	}
	t.Setenv(public.EnvPasswordHash, "$2a$10$abcdefghijklmnopqrstuuJ0k1n0wn0hash0f0n0thing00000000")
	if _, err := run(t, passwordCmd, config{DataDir: t.TempDir()}, "set"); exitCode(err) != exitConflict {
		t.Errorf("set with the variable: %v", err)
	}
}
