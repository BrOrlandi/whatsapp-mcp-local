package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// envFile reads the KEY=value lines of a shell file.
func envFile(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if k, v, ok := strings.Cut(line, "="); ok && regexp.MustCompile(`^[A-Z0-9_a-z]+$`).MatchString(k) {
			out[k] = v
		}
	}
	return out
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// A checkout on Windows ends lines with CRLF.
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// Every release is built with the Go in build/go.env: the release script
// builds with it and checks each binary, and the Dockerfiles default to it.
func TestReleasesAreBuiltWithThePinnedGo(t *testing.T) {
	v := envFile(t, "../../build/go.env")["GO_VERSION"]
	if !regexp.MustCompile(`^1\.\d+\.\d+$`).MatchString(v) {
		t.Fatalf("build/go.env: GO_VERSION %q", v)
	}
	for _, f := range []string{"../../build/docker/Dockerfile", "../../build/linux/Dockerfile"} {
		if !strings.Contains(read(t, f), "ARG GO_VERSION="+v+"\n") {
			t.Errorf("%s does not default to Go %s", f, v)
		}
	}
	if !strings.Contains(read(t, "../../build/docker/Dockerfile"), "golang:${GO_VERSION}-") {
		t.Error("the Docker image's builder does not use GO_VERSION")
	}
	release := read(t, "../../scripts/release.sh")
	for _, want := range []string{". build/go.env", `export GOTOOLCHAIN="go$GO_VERSION"`, `"golang:$GO_VERSION"`,
		`--build-arg GO_VERSION="$GO_VERSION"`, `go_of "cli/$bin"`} {
		if !strings.Contains(release, want) {
			t.Errorf("scripts/release.sh lacks %q", want)
		}
	}
	if strings.Contains(release, "golang:1.") {
		t.Error("scripts/release.sh names a Go image by hand")
	}
}

// install.sh installs the wacli pinned in build/wacli.env, the same version
// and checksums.
func TestInstallShPinsTheWacliOfTheRelease(t *testing.T) {
	pin := envFile(t, "../../build/wacli.env")
	script := envFile(t, "../../install.sh")
	if script["WACLI_VERSION"] != pin["WACLI_VERSION"] {
		t.Errorf("install.sh installs wacli %q, build/wacli.env pins %q", script["WACLI_VERSION"], pin["WACLI_VERSION"])
	}
	for _, sys := range []string{"darwin_arm64", "darwin_amd64", "linux_amd64", "linux_arm64"} {
		k := "WACLI_SHA256_" + sys
		if script[k] == "" || script[k] != pin[k] {
			t.Errorf("install.sh %s = %q, build/wacli.env has %q", k, script[k], pin[k])
		}
	}
}

// A download that does not match its checksum is never installed: neither
// wacli nor whatsapp-mcp.
func TestInstallShRefusesADownloadThatDoesNotMatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is for macOS and Linux")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	_ = os.MkdirAll(bin, 0o755)
	// curl stands in for the network: every file it saves is the wrong one.
	fake := `#!/bin/sh
out=""; url=""; fmt=""
while [ $# -gt 0 ]; do case "$1" in -o) out="$2"; shift 2;; -w) fmt="$2"; shift 2;; -*) shift;; *) url="$1"; shift;; esac; done
echo "$url" >> "$CURL_LOG"
case "$url" in
  */releases/latest) [ -n "$fmt" ] && printf 'https://github.com/x/y/releases/tag/v9.9.9'; exit 0;;
  *checksums.txt) printf '%s  %s\n' 0000000000000000000000000000000000000000000000000000000000000000 "whatsapp-mcp_${OS}_${ARCH}.tar.gz" > "$out";;
  *) printf 'not the file\n' > "$out";;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "curl"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	src, _ := filepath.Abs("../../install.sh")
	for fn, want := range map[string]string{"install_wacli": "wacli download does not match", "install_mcp": "does not match the release's checksums.txt"} {
		// The script's functions alone, without running its install steps.
		harness := `set -euo pipefail
say() { :; }; warn() { :; }; die() { echo "$*" >&2; exit 1; }
OS=linux ARCH=amd64 REPO=x/y BIN_DIR="$1/out" TMP="$1/tmp"
export OS ARCH
mkdir -p "$BIN_DIR" "$TMP"
eval "$(sed -n '/^WACLI_VERSION=/,/^}$/p; /^WACLI_SHA256_/p; /^sha256_of()/,/^}$/p; /^install_wacli()/,/^}$/p; /^install_mcp()/,/^}$/p' "$2")"
` + fn
		cmd := exec.Command("bash", "-c", harness, "harness", dir, src)
		curlLog := filepath.Join(dir, fn+".log")
		cmd.Env = append(os.Environ(), "PATH="+bin+":/usr/bin:/bin", "CURL_LOG="+curlLog)
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), want) {
			t.Errorf("%s with a wrong download: %v\n%s", fn, err, out)
		}
		// The release's file and its checksums come from one tag, resolved
		// once, never from two separate "latest" requests.
		if fn == "install_mcp" {
			urls := read(t, curlLog)
			if strings.Contains(urls, "latest/download") || strings.Count(urls, "/releases/download/v9.9.9/") != 2 {
				t.Errorf("install_mcp downloaded:\n%s", urls)
			}
		}
		if entries, _ := os.ReadDir(filepath.Join(dir, "out")); len(entries) != 0 {
			t.Errorf("%s installed a file that did not match", fn)
		}
	}
}

// The release script's check reads the Go a binary was built with even in a
// folder with a space, as the macOS app's is.
func TestTheReleaseChecksTheGoOfEachBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the release is built on a Mac")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go command")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "WhatsApp MCP.app", "Contents", "MacOS")
	_ = os.MkdirAll(dir, 0o755)
	bin := filepath.Join(dir, "WhatsApp MCP")
	b, _ := os.ReadFile(self)
	if err := os.WriteFile(bin, b, 0o755); err != nil {
		t.Fatal(err)
	}
	script, _ := filepath.Abs("../../scripts/release.sh")
	check := func(version string) error {
		harness := `set -euo pipefail
eval "$(sed -n '/^go_of() {/,/^}/p' "$1")"
GO_VERSION="$2"
go_of "$3"`
		out, err := exec.Command("bash", "-c", harness, "harness", script, version, bin).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %s", err, out)
		}
		return nil
	}
	if err := check(strings.TrimPrefix(runtime.Version(), "go")); err != nil {
		t.Fatalf("the Go it was built with: %v", err)
	}
	if err := check("1.0.0"); err == nil {
		t.Fatal("another Go was accepted")
	}
}

// Every transcription setting the program reads reaches it in Docker:
// compose.yml passes each one through from the .env, empty by default.
func TestComposePassesEveryTranscriptionSetting(t *testing.T) {
	compose := read(t, "../../compose.yml")
	re := regexp.MustCompile(`"(WHATSAPP_MCP_TRANSCRIPTION_[A-Z_]+)"`)
	seen := map[string]bool{}
	err := filepath.WalkDir("../../internal", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.Contains(path, "/pro/") {
			return err
		}
		for _, m := range re.FindAllStringSubmatch(read(t, path), -1) {
			seen[m[1]] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) < 5 {
		t.Fatalf("found only %v", seen)
	}
	for name := range seen {
		if !strings.Contains(compose, "      "+name+": ${"+name+":-}\n") {
			t.Errorf("compose.yml does not pass %s through", name)
		}
	}
}

// In Docker, files on their way go to the data volume's disk, never to a
// /tmp in memory: a document to send may be 2 GB and a small server has 1 GB
// of memory.
func TestDockerKeepsTemporaryFilesOnDisk(t *testing.T) {
	if !strings.Contains(read(t, "../../build/docker/Dockerfile"), "TMPDIR=/data/tmp") {
		t.Error("the image does not put TMPDIR on the data volume")
	}
	compose := read(t, "../../compose.yml")
	if m := regexp.MustCompile(`- /tmp:size=(\d+)m`).FindStringSubmatch(compose); m == nil || len(m[1]) > 3 {
		t.Error("compose.yml's /tmp is not a small tmpfs")
	}
}
