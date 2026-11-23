//go:build !windows

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/localasr"
)

// The Docker image points TMPDIR at /data/tmp, which nothing but the
// daemon's serve made: transcription serve must make it, or every voice note
// fails with "stat /data/tmp".
func TestTranscriptionServeMakesTheTempFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake programs are shell scripts")
	}
	// Stand-ins for ffmpeg and whisper-cli, found on the PATH.
	bin := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("ffmpeg", `for a; do last=$a; done; : > "$last"`+"\n")
	write("whisper-cli", `while [ $# -gt 0 ]; do case "$1" in -of) of=$2; shift;; esac; shift; done
echo "ok from the fake" > "$of.txt"
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")

	// The model, as a sparse file of its size.
	data := t.TempDir()
	if err := os.MkdirAll(filepath.Join(data, "models"), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(data, "models", localasr.ModelName))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(600 << 20); err != nil {
		t.Fatal(err)
	}
	f.Close()

	tmp := filepath.Join(t.TempDir(), "data", "tmp") // does not exist yet
	t.Setenv("TMPDIR", tmp)
	const token = "a-token-of-the-test-0123456789"
	t.Setenv(localasr.EnvRemoteToken, token)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	done := make(chan error, 1)
	go func() {
		done <- transcriptionServe(config{DataDir: data}, localasr.New(data), []string{"--listen", addr})
	}()
	t.Cleanup(func() {
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM) // serve stops on SIGTERM
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("serve did not stop")
		}
	})
	up := false
	for i := 0; i < 100 && !up; i++ {
		select {
		case err := <-done:
			t.Fatalf("serve stopped: %v", err)
		default:
		}
		if c, err := net.Dial("tcp", addr); err == nil {
			c.Close()
			up = true
		} else {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if !up {
		t.Fatal("serve did not start")
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, _ := w.CreateFormFile("file", "note.ogg")
	_, _ = part.Write([]byte("OggS"))
	_ = w.Close()
	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/v1/audio/transcriptions", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out struct{ Text string }
	if resp.StatusCode != http.StatusOK || json.Unmarshal(raw, &out) != nil || !strings.Contains(out.Text, "ok from the fake") {
		t.Fatalf("transcription with TMPDIR=%s missing: %d %s", tmp, resp.StatusCode, raw)
	}
	if info, err := os.Stat(tmp); err != nil || !info.IsDir() {
		t.Fatalf("the temp folder was not made: %v", err)
	}
}
