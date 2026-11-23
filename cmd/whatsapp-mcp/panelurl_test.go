package main

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

// The browser a command opens lands signed in when there is a token: the
// address carries a single-use code, asked for with the token. Without a
// token, or when the service cannot give one, it is the plain panel.
func TestThePanelOpensSignedIn(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var auth string
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost || r.URL.Path != "/api/token/codigo" || auth != "Bearer s3cret-token-of-length" {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"/token?codigo=abc"}`))
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.Close() })
	port := ln.Addr().(*net.TCPAddr).Port
	base := "http://127.0.0.1:" + strconv.Itoa(port)

	if got := panelURL(config{Port: port}); got != base+"/" {
		t.Errorf("without a token: %s", got)
	}
	if got := panelURL(config{Port: port, Token: "s3cret-token-of-length"}); got != base+"/token?codigo=abc" {
		t.Errorf("with the token: %s (sent %q)", got, auth)
	}
	if got := panelURL(config{Port: port, Token: "another-token-entirely"}); got != base+"/" {
		t.Errorf("when no code comes: %s", got)
	}
}

// The container's folder for temporary files is made at start and emptied
// of what a stop left behind; a folder outside the data volume is never
// touched.
func TestTheTempFolderStartsEmpty(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the folder is made only in the container, and Windows has no Unix permissions")
	}
	data := t.TempDir()
	tmp := filepath.Join(data, "tmp")
	if err := freshTempDir(tmp, data); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(tmp); err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o700 {
		t.Fatalf("not made: %v %v", fi, err)
	}
	_ = os.MkdirAll(filepath.Join(tmp, "whatsapp-mcp-send-1"), 0o700)
	_ = os.WriteFile(filepath.Join(tmp, "whatsapp-mcp-send-1", "big.pdf"), []byte("x"), 0o600)
	_ = os.Chmod(tmp, 0o777)
	if err := freshTempDir(tmp, data); err != nil {
		t.Fatal(err)
	}
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
	if fi, _ := os.Stat(tmp); fi.Mode().Perm() != 0o700 {
		t.Fatalf("a folder made open stays %v", fi.Mode().Perm())
	}
	outside := t.TempDir()
	keep := filepath.Join(outside, "keep")
	_ = os.WriteFile(keep, []byte("x"), 0o600)
	for _, dir := range []string{outside, data, "", "/tmp"} {
		if err := freshTempDir(dir, data); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("a folder outside the data volume was emptied")
	}
	if _, err := os.Stat(tmp); err != nil {
		t.Fatal("the data folder itself was emptied")
	}
}
