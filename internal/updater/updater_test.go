package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		tag, current string
		want         bool
	}{
		{"v1.2.0", "1.1.9", true},
		{"v1.10.0", "1.9.0", true},
		{"v2.0.0", "1.99.99", true},
		{"v1.2.0", "1.2.0", false},
		{"v1.1.0", "1.2.0", false},
		{"v1.3.0-rc1", "1.2.0", false},
		{"v1.3.0", "dev", false},
		{"v1.3.0", "8ace907", false},
	} {
		if got := newer(c.tag, c.current); got != c.want {
			t.Errorf("newer(%q, %q) = %v", c.tag, c.current, got)
		}
	}
}

// fakeGitHub serves a release with one file and its checksums.txt.
func fakeGitHub(t *testing.T, tag, name string, body []byte, sum string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/releases/latest":
			fmt.Fprintf(w, `{"tag_name":%q,"html_url":"https://example.test/r","assets":[
				{"name":%q,"browser_download_url":"%s/dl/file","size":%d},
				{"name":"checksums.txt","browser_download_url":"%s/dl/sums"}]}`, tag, name, srv.URL, len(body), srv.URL)
		case "/dl/file":
			_, _ = w.Write(body)
		case "/dl/sums":
			fmt.Fprintf(w, "%s  %s\n%s  other.zip\n", sum, name, strings.Repeat("0", 64))
		default:
			http.NotFound(w, r)
		}
	}))
	return srv
}

func TestCheckAndDownloadVerifiesTheChecksum(t *testing.T) {
	body := []byte("the new app")
	h := sha256.Sum256(body)
	target := Target{Kind: "appimage", Path: filepath.Join(t.TempDir(), "WhatsApp-MCP.AppImage")}
	srv := fakeGitHub(t, "v1.3.0", target.assetName(), body, hex.EncodeToString(h[:]))
	defer srv.Close()
	u := &Updater{Repo: "o/r", Version: "1.2.0", API: srv.URL, Dir: t.TempDir(), Target: target}

	found, err := u.Check(context.Background())
	if err != nil || !found {
		t.Fatalf("check: %v %v", found, err)
	}
	if st := u.State(); st.Phase != Available || st.Latest != "1.3.0" {
		t.Fatalf("state %+v", st)
	}
	path, err := u.Download(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "the new app" {
		t.Fatal("downloaded the wrong bytes")
	}
}

func TestDownloadRefusesAMismatch(t *testing.T) {
	body := []byte("tampered")
	target := Target{Kind: "appimage", Path: "/x"}
	srv := fakeGitHub(t, "v1.3.0", target.assetName(), body, strings.Repeat("a", 64))
	defer srv.Close()
	u := &Updater{Repo: "o/r", Version: "1.2.0", API: srv.URL, Dir: t.TempDir(), Target: target}
	if _, err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := u.Download(context.Background()); err == nil || !strings.Contains(err.Error(), "não confere") {
		t.Fatalf("a file that does not match checksums.txt must be refused, got %v", err)
	}
	if entries, _ := os.ReadDir(u.Dir); len(entries) != 0 {
		t.Fatal("a refused download must not stay on disk")
	}
}

func TestDebIsUpdatedByHand(t *testing.T) {
	srv := fakeGitHub(t, "v1.3.0", "whatever", nil, "")
	defer srv.Close()
	u := &Updater{Repo: "o/r", Version: "1.2.0", API: srv.URL, Dir: t.TempDir(), Target: Target{Kind: "deb"}}
	if found, err := u.Check(context.Background()); err != nil || !found {
		t.Fatal(found, err)
	}
	if st := u.State(); st.Phase != Manual || st.Page != "https://example.test/r" {
		t.Fatalf("state %+v", st)
	}
}

func TestNoReleaseIsUpToDate(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	u := &Updater{Repo: "o/r", Version: "1.2.0", API: srv.URL, Target: Target{Kind: "setup"}}
	if found, err := u.Check(context.Background()); err != nil || found || u.State().Phase != Current {
		t.Fatalf("%v %v %+v", found, err, u.State())
	}
}
