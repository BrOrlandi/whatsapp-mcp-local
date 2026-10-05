// Package sidecar fetches and checks the programs transcription runs, which
// the app does not ship but downloads the first time transcription is turned
// on: whisper-cli and a minimal ffmpeg, built for each system by this
// project's CI and published as one archive per system.
//
// Every archive is pinned by its sha256 in manifest.json, which a release
// carries; after unpacking, each program's own sha256 is recorded and checked
// again before it runs.
package sidecar

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/platform"
)

// Package is one system's archive.
type Package struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	// Accel is how whisper runs from it: metal, cuda or cpu.
	Accel string `json:"accel"`
}

type manifest struct {
	Version  string             `json:"version"`
	Packages map[string]Package `json:"packages"`
}

//go:embed manifest.json
var embedded []byte

// loadManifest reads the pinned manifest, or the file WHATSAPP_MCP_SIDECARS
// names, for testing a build of the archives before it is published.
func loadManifest() (manifest, error) {
	raw := embedded
	if path := os.Getenv("WHATSAPP_MCP_SIDECARS"); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return manifest{}, err
		}
		raw = b
	}
	var m manifest
	err := json.Unmarshal(raw, &m)
	return m, err
}

// key names this system in the manifest: macOS has one universal archive.
func key() string {
	if runtime.GOOS == "darwin" {
		return "darwin/universal"
	}
	return runtime.GOOS + "/" + runtime.GOARCH
}

// hasNVIDIA reports whether an NVIDIA driver is installed, for the CUDA build
// of whisper on Windows.
var hasNVIDIA = func() bool { return platform.LookPath("nvidia-smi") != "" }

// ForThisSystem is the archive for this computer, if one is published: on
// Windows with an NVIDIA card, the CUDA one.
func ForThisSystem() (Package, bool) {
	m, err := loadManifest()
	if err != nil {
		return Package{}, false
	}
	if runtime.GOOS == "windows" && hasNVIDIA() {
		if p, ok := m.Packages[key()+"/cuda"]; ok && p.URL != "" {
			return p, true
		}
	}
	p, ok := m.Packages[key()]
	return p, ok && p.URL != ""
}

// Dir is where the programs are unpacked.
func Dir(dataDir string) string { return filepath.Join(dataDir, "bin") }

// installed is the record written after unpacking: which archive, and the
// sha256 of each program in it.
type installed struct {
	URL   string            `json:"url"`
	Accel string            `json:"accel"`
	Files map[string]string `json:"files"`
	At    time.Time         `json:"installed_at"`
}

func recordPath(dataDir string) string { return filepath.Join(Dir(dataDir), "manifest.json") }

// Tools are the programs, once installed and checked.
type Tools struct {
	Whisper string
	FFmpeg  string
	Accel   string
}

var verified sync.Map // path → "size:mtime:sha256" already checked

// Installed returns the unpacked programs when their sha256 still matches
// what was recorded at install. A program that changed is not run.
func Installed(dataDir string) (Tools, error) {
	raw, err := os.ReadFile(recordPath(dataDir))
	if err != nil {
		return Tools{}, err
	}
	var rec installed
	if err := json.Unmarshal(raw, &rec); err != nil {
		return Tools{}, err
	}
	t := Tools{Accel: rec.Accel}
	for name, dst := range map[string]*string{"whisper-cli": &t.Whisper, "ffmpeg": &t.FFmpeg} {
		path := filepath.Join(Dir(dataDir), platform.ExeName(name))
		if err := check(path, rec.Files[platform.ExeName(name)]); err != nil {
			return Tools{}, err
		}
		*dst = path
	}
	return t, nil
}

func check(path, want string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	stamp := fmt.Sprintf("%d:%d:%s", info.Size(), info.ModTime().UnixNano(), want)
	if v, ok := verified.Load(path); ok && v == stamp {
		return nil
	}
	got, err := fileSHA256(path)
	if err != nil {
		return err
	}
	if want == "" || got != want {
		return fmt.Errorf("%s mudou depois de instalado; instale a transcrição de novo", filepath.Base(path))
	}
	verified.Store(path, stamp)
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Install downloads the archive, checks its sha256, unpacks it into Dir and
// records each program's sha256. progress is told the bytes so far.
func Install(ctx context.Context, dataDir string, pkg Package, progress func(done, total int64)) error {
	dir := Dir(dataDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pkg.URL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("não foi possível baixar os programas de transcrição: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("o download dos programas de transcrição respondeu %s", resp.Status)
	}
	total := pkg.Size
	if total <= 0 {
		total = resp.ContentLength
	}
	h := sha256.New()
	var done int64
	buf := make([]byte, 256<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := tmp.Write(buf[:n]); err != nil {
				return err
			}
			h.Write(buf[:n])
			done += int64(n)
			if progress != nil {
				progress(done, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("o download dos programas de transcrição parou: %w", rerr)
		}
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, pkg.SHA256) {
		return fmt.Errorf("os programas de transcrição baixados não conferem (sha256 %s, esperado %s)", got, pkg.SHA256)
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	files, err := unpack(tmp.Name(), pkg.URL, dir)
	if err != nil {
		return err
	}
	rec := installed{URL: pkg.URL, Accel: pkg.Accel, Files: map[string]string{}, At: time.Now()}
	for _, name := range files {
		path := filepath.Join(dir, name)
		if runtime.GOOS == "darwin" {
			if err := checkSignature(path); err != nil {
				return err
			}
		}
		sum, err := fileSHA256(path)
		if err != nil {
			return err
		}
		rec.Files[name] = sum
	}
	for _, name := range []string{"whisper-cli", "ffmpeg"} {
		if rec.Files[platform.ExeName(name)] == "" {
			return fmt.Errorf("o pacote de transcrição não tem %s", name)
		}
	}
	body, _ := json.MarshalIndent(rec, "", "  ")
	return os.WriteFile(recordPath(dataDir), body, 0o644)
}

// unpack extracts a .tar.gz or .zip into dir, flat, and returns the names of
// the files it wrote.
func unpack(archive, name, dir string) ([]string, error) {
	var files []string
	write := func(base string, mode os.FileMode, r io.Reader) error {
		if base == "" || base == "." || strings.HasPrefix(base, ".") || strings.ContainsAny(base, `/\`) {
			return nil
		}
		path := filepath.Join(dir, base)
		tmp := path + ".new"
		f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, r); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		if err := os.Rename(tmp, path); err != nil {
			return err
		}
		files = append(files, base)
		return nil
	}
	if strings.HasSuffix(name, ".zip") {
		zr, err := zip.OpenReader(archive)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		for _, f := range zr.File {
			if f.FileInfo().IsDir() {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			err = write(filepath.Base(f.Name), 0o755, rc)
			rc.Close()
			if err != nil {
				return nil, err
			}
		}
		return files, nil
	}
	f, err := os.Open(archive)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return files, nil
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		mode := os.FileMode(0o644)
		if hdr.FileInfo().Mode()&0o111 != 0 {
			mode = 0o755
		}
		if err := write(filepath.Base(hdr.Name), mode, tr); err != nil {
			return nil, err
		}
	}
}

// checkSignature makes sure a downloaded program carries a valid signature
// and no quarantine flag, which would stop it from running.
func checkSignature(path string) error {
	_ = exec.Command("xattr", "-d", "com.apple.quarantine", path).Run()
	if out, err := exec.Command("file", "-b", path).Output(); err == nil && !strings.Contains(string(out), "Mach-O") {
		return nil // a licence or another text file
	}
	if out, err := exec.Command("codesign", "--verify", "--strict", path).CombinedOutput(); err != nil {
		return fmt.Errorf("a assinatura de %s não confere: %s", filepath.Base(path), strings.TrimSpace(string(out)))
	}
	return nil
}
