package transfer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// pair puts a device in the state database of dir.
func pair(t *testing.T, dir, id string) {
	t.Helper()
	st, err := state.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.AddRemoteClient(context.Background(), state.RemoteClient{ID: id, Name: id, PublicKey: []byte(id), Scope: "admin", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

func devices(t *testing.T, dir string) []string {
	t.Helper()
	st, err := state.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	list, err := st.RemoteClients(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, c := range list {
		ids = append(ids, c.ID)
	}
	return ids
}

func TestMoveANumber(t *testing.T) {
	// The sending side: a store with its session, and the data folder with
	// what is the number's and what is the machine's.
	from := t.TempDir()
	store, data := filepath.Join(from, "wacli"), filepath.Join(from, "data")
	write(t, filepath.Join(store, "session.db"), "session")
	write(t, filepath.Join(store, "wacli.db"), "messages")
	write(t, filepath.Join(store, "LOCK"), "")
	write(t, filepath.Join(data, "media", "a", "photo.jpg"), "photo")
	write(t, filepath.Join(data, "api-keys"), "secret hashes")
	write(t, filepath.Join(data, "remote-server.key"), "key")
	pair(t, data, "laptop") // a device paired with the sending side

	var moved int64
	var buf bytes.Buffer
	if err := Pack(context.Background(), &buf, store, data, func(n int64) { moved += n }); err != nil {
		t.Fatal(err)
	}
	if moved == 0 || Size(store, data) == 0 {
		t.Errorf("no progress: moved %d, size %d", moved, Size(store, data))
	}

	// The receiving side already has its own store, data and devices.
	to := t.TempDir()
	rstore, rdata := filepath.Join(to, "wacli"), filepath.Join(to, "data")
	write(t, filepath.Join(rstore, "session.db"), "empty session")
	write(t, filepath.Join(rdata, "api-keys"), "server hashes")
	pair(t, rdata, "server-app")

	staging, err := NewStaging(rstore, rdata)
	if err != nil {
		t.Fatal(err)
	}
	if err := Unpack(&buf, staging, nil); err != nil {
		t.Fatal(err)
	}
	aside, err := staging.Install(rstore, rdata, KeepRemoteClients)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(rstore, "session.db")); got != "session" {
		t.Errorf("session.db = %q", got)
	}
	if got := read(t, filepath.Join(rdata, "media", "a", "photo.jpg")); got != "photo" {
		t.Errorf("the media did not come: %q", got)
	}
	if _, err := os.Stat(filepath.Join(rstore, "LOCK")); err == nil {
		t.Error("the sender's lock came along")
	}
	if got := read(t, filepath.Join(rdata, "api-keys")); got != "server hashes" {
		t.Errorf("the machine's API keys changed: %q", got)
	}
	if _, err := os.Stat(filepath.Join(rdata, "remote-server.key")); err == nil {
		t.Error("the sender's server key came along")
	}
	if got := devices(t, rdata); len(got) != 1 || got[0] != "server-app" {
		t.Errorf("the paired devices are %v, want the receiver's own", got)
	}
	if len(aside) != 2 {
		t.Fatalf("aside = %v", aside)
	}
	if got := read(t, filepath.Join(aside[0], "session.db")); got != "empty session" {
		t.Errorf("the previous store was not kept aside: %q", got)
	}
	for _, p := range []string{staging.Store, staging.Data} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("staging %s left behind", p)
		}
	}

	if err := RemoveData(store, data); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{store, filepath.Join(data, "state.db"), filepath.Join(data, "media")} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s was not deleted", p)
		}
	}
	if _, err := os.Stat(filepath.Join(data, "api-keys")); err != nil {
		t.Error("deleting the number's data took the machine's too")
	}
}

func TestUnpackRefusesWhatIsNotTheNumbers(t *testing.T) {
	for _, name := range []string{"wacli/../../evil", "/etc/passwd", "mcp/api-keys", "mcp/../wacli/x", "other/file", "mcp/media/../../x"} {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o600, Size: 1})
		_, _ = tw.Write([]byte("x"))
		_ = tw.Close()
		_ = gz.Close()
		dir := t.TempDir()
		staging, _ := NewStaging(filepath.Join(dir, "wacli"), filepath.Join(dir, "data"))
		if err := Unpack(&buf, staging, nil); err == nil || !strings.Contains(err.Error(), "unexpected entry") {
			t.Errorf("%q: %v", name, err)
		}
	}
	// A symbolic link is refused too.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeSymlink, Name: "wacli/link", Linkname: "/etc"})
	_ = tw.Close()
	_ = gz.Close()
	dir := t.TempDir()
	staging, _ := NewStaging(filepath.Join(dir, "wacli"), filepath.Join(dir, "data"))
	if err := Unpack(&buf, staging, nil); err == nil {
		t.Error("a symbolic link was accepted")
	}
}

func TestUnpackNeedsASession(t *testing.T) {
	from := t.TempDir()
	write(t, filepath.Join(from, "wacli", "wacli.db"), "messages")
	var buf bytes.Buffer
	if err := Pack(context.Background(), &buf, filepath.Join(from, "wacli"), filepath.Join(from, "data"), nil); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	staging, _ := NewStaging(filepath.Join(dir, "wacli"), filepath.Join(dir, "data"))
	if err := Unpack(&buf, staging, nil); err == nil {
		t.Error("data without a session was accepted")
	}
}
