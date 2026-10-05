// Package updater keeps the app current from GitHub Releases: it looks for a
// newer version once a day, downloads the one file for this system, checks it
// against the release's checksums.txt (and, on macOS, its signature), and
// puts it in place of the running app, which then restarts into it.
//
// wacli ships inside the app, so it is updated with it and only with it.
package updater

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Phases of an update.
const (
	Idle        = "idle"
	Checking    = "checking"
	Current     = "current"
	Available   = "available"
	Downloading = "downloading"
	Ready       = "ready"
	// Manual is a version that has to be installed by hand: a .deb.
	Manual = "manual"
	Failed = "error"
	// Unsupported is a build that cannot update itself: a development
	// version, or an install the updater does not know how to replace.
	Unsupported = "unsupported"
)

// State is where an update stands.
type State struct {
	Phase     string
	Current   string
	Latest    string
	Page      string
	Progress  int
	Error     string
	CheckedAt time.Time
}

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type release struct {
	Tag    string  `json:"tag_name"`
	Page   string  `json:"html_url"`
	Draft  bool    `json:"draft"`
	Pre    bool    `json:"prerelease"`
	Assets []asset `json:"assets"`
}

type Updater struct {
	Repo    string // owner/name on GitHub
	Version string // the running version, as tagged without the v
	// API is GitHub's API, replaceable in tests.
	API    string
	Client *http.Client
	Logger *slog.Logger
	// Dir is where downloads are kept until they are installed.
	Dir string
	// Target says what is installed and how to replace it; the default is
	// worked out from the running program.
	Target Target

	mu       sync.Mutex
	state    State
	latest   *release
	file     asset
	staged   string
	running  bool
	applying bool
}

// ErrBusy means an update is already being downloaded or installed.
var ErrBusy = errors.New("a atualização já está em andamento")

// Apply downloads, checks and installs the new version, once at a time: a
// second call while one runs returns ErrBusy at once. The phase becomes
// downloading before Apply returns its first byte, so a page polling the
// state sees the work start.
func (u *Updater) Apply(ctx context.Context) error {
	u.mu.Lock()
	if u.applying {
		u.mu.Unlock()
		return ErrBusy
	}
	if u.latest == nil {
		u.mu.Unlock()
		return errors.New("nenhuma versão nova para instalar")
	}
	u.applying = true
	u.state.Phase, u.state.Progress, u.state.Error = Downloading, 0, ""
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.applying = false
		u.mu.Unlock()
	}()
	if _, err := u.Download(ctx); err != nil {
		return err
	}
	return u.Install(ctx)
}

// Target is the installed app this updater replaces.
type Target struct {
	// Kind is dmg (a macOS .app), setup (the Windows installer), appimage,
	// or deb (installed by a package manager, updated by hand).
	Kind string
	// Path is the .app bundle or the AppImage file; unused for setup.
	Path string
}

// DetectTarget works out how the running app was installed.
func DetectTarget(exe string) Target {
	switch runtime.GOOS {
	case "darwin":
		if i := strings.Index(exe, ".app/Contents/MacOS/"); i > 0 {
			return Target{Kind: "dmg", Path: exe[:i+len(".app")]}
		}
	case "windows":
		return Target{Kind: "setup"}
	case "linux":
		if p := os.Getenv("APPIMAGE"); p != "" {
			return Target{Kind: "appimage", Path: p}
		}
		return Target{Kind: "deb"}
	}
	return Target{}
}

// assetName is the file a release carries for this system.
func (t Target) assetName() string {
	switch t.Kind {
	case "dmg":
		return "WhatsApp-MCP.dmg"
	case "setup":
		return "WhatsApp-MCP-Setup.exe"
	case "appimage":
		arch := "x86_64"
		if runtime.GOARCH == "arm64" {
			arch = "aarch64"
		}
		return "WhatsApp-MCP-" + arch + ".AppImage"
	}
	return ""
}

// Enabled reports whether this build can update itself: a tagged version,
// installed in a way the updater knows.
func (u *Updater) Enabled() bool {
	_, ok := parseVersion(u.Version)
	return ok && u.Repo != "" && u.Target.Kind != ""
}

func (u *Updater) State() State {
	u.mu.Lock()
	defer u.mu.Unlock()
	s := u.state
	s.Current = u.Version
	if s.Phase == "" {
		s.Phase = Idle
	}
	return s
}

func (u *Updater) set(f func(*State)) {
	u.mu.Lock()
	f(&u.state)
	u.mu.Unlock()
}

func (u *Updater) logger() *slog.Logger {
	if u.Logger != nil {
		return u.Logger
	}
	return slog.Default()
}

func (u *Updater) client() *http.Client {
	if u.Client != nil {
		return u.Client
	}
	return &http.Client{Timeout: 30 * time.Minute}
}

// Check asks GitHub for the latest release. It reports whether a newer one
// exists; a repository without releases, or out of reach, is up to date.
func (u *Updater) Check(ctx context.Context) (bool, error) {
	if !u.Enabled() {
		u.set(func(s *State) { s.Phase, s.CheckedAt = Unsupported, time.Now() })
		return false, nil
	}
	u.mu.Lock()
	if u.running {
		u.mu.Unlock()
		return u.State().Phase == Available, nil
	}
	u.running = true
	u.state.Phase, u.state.Error = Checking, ""
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.running = false
		u.mu.Unlock()
	}()

	rel, err := u.fetchLatest(ctx)
	now := time.Now()
	if err != nil {
		u.set(func(s *State) { s.Phase, s.Error, s.CheckedAt = Failed, err.Error(), now })
		return false, err
	}
	if rel == nil || !newer(rel.Tag, u.Version) {
		u.set(func(s *State) { s.Phase, s.CheckedAt = Current, now })
		return false, nil
	}
	latest := strings.TrimPrefix(rel.Tag, "v")
	if u.Target.Kind == "deb" {
		u.set(func(s *State) { s.Phase, s.Latest, s.Page, s.CheckedAt = Manual, latest, rel.Page, now })
		return true, nil
	}
	want := u.Target.assetName()
	var found *asset
	for i := range rel.Assets {
		if rel.Assets[i].Name == want {
			found = &rel.Assets[i]
		}
	}
	if found == nil {
		err := fmt.Errorf("a versão %s não tem o arquivo %s", latest, want)
		u.set(func(s *State) {
			s.Phase, s.Error, s.Latest, s.Page, s.CheckedAt = Failed, err.Error(), latest, rel.Page, now
		})
		return false, err
	}
	u.mu.Lock()
	u.latest, u.file = rel, *found
	u.state = State{Phase: Available, Latest: latest, Page: rel.Page, CheckedAt: now}
	u.mu.Unlock()
	return true, nil
}

func (u *Updater) fetchLatest(ctx context.Context) (*release, error) {
	api := u.API
	if api == "" {
		api = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+"/repos/"+u.Repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "whatsapp-mcp/"+u.Version)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := u.client().Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("não foi possível consultar as versões: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // no release yet, or a private repository
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("o GitHub respondeu %s", resp.Status)
	}
	var rel release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		return nil, err
	}
	if rel.Draft || rel.Pre {
		return nil, nil
	}
	return &rel, nil
}

// Download fetches the new version and checks it against the release's
// checksums.txt.
func (u *Updater) Download(ctx context.Context) (string, error) {
	u.mu.Lock()
	rel, file := u.latest, u.file
	u.mu.Unlock()
	if rel == nil {
		return "", errors.New("nenhuma versão nova para instalar")
	}
	want, err := u.checksum(ctx, rel, file.Name)
	if err != nil {
		u.fail(err)
		return "", err
	}
	if err := os.MkdirAll(u.Dir, 0o755); err != nil {
		u.fail(err)
		return "", err
	}
	path := filepath.Join(u.Dir, file.Name)
	u.set(func(s *State) { s.Phase, s.Progress, s.Error = Downloading, 0, "" })
	if err := u.fetch(ctx, file, path, want); err != nil {
		os.Remove(path)
		u.fail(err)
		return "", err
	}
	u.mu.Lock()
	u.staged = path
	u.mu.Unlock()
	return path, nil
}

func (u *Updater) fail(err error) {
	u.set(func(s *State) { s.Phase, s.Error = Failed, err.Error() })
	u.logger().Warn("update failed", "error", err)
}

// checksum finds the file's sha256 in checksums.txt.
func (u *Updater) checksum(ctx context.Context, rel *release, name string) (string, error) {
	var sums *asset
	for i := range rel.Assets {
		if rel.Assets[i].Name == "checksums.txt" {
			sums = &rel.Assets[i]
		}
	}
	if sums == nil {
		return "", errors.New("a versão nova não publicou checksums.txt; ela não será instalada")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sums.URL, nil)
	if err != nil {
		return "", err
	}
	resp, err := u.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksums.txt respondeu %s", resp.Status)
	}
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 1<<20))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name && len(fields[0]) == 64 {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("checksums.txt não tem %s", name)
}

func (u *Updater) fetch(ctx context.Context, file asset, path, want string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, file.URL, nil)
	if err != nil {
		return err
	}
	resp, err := u.client().Do(req)
	if err != nil {
		return fmt.Errorf("o download parou: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("o download respondeu %s", resp.Status)
	}
	total := file.Size
	if total <= 0 {
		total = resp.ContentLength
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	h := sha256.New()
	var done int64
	buf := make([]byte, 256<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				return err
			}
			h.Write(buf[:n])
			done += int64(n)
			if total > 0 {
				pct := int(done * 100 / total)
				u.set(func(s *State) { s.Progress = pct })
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("o download parou: %w", rerr)
		}
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("o arquivo baixado não confere com checksums.txt (sha256 %s, esperado %s)", got, want)
	}
	return out.Close()
}

// Install puts the downloaded version in place. It returns once the swap is
// staged; the app must then quit, and a helper starts the new version as
// soon as the old one is gone.
func (u *Updater) Install(ctx context.Context) error {
	u.mu.Lock()
	path := u.staged
	u.mu.Unlock()
	if path == "" {
		return errors.New("baixe a versão nova antes de instalar")
	}
	var err error
	switch u.Target.Kind {
	case "dmg":
		err = installDMG(ctx, path, u.Target.Path)
	case "setup":
		err = installSetup(path)
	case "appimage":
		err = installAppImage(path, u.Target.Path)
	default:
		err = errors.New("este jeito de instalar não se atualiza sozinho")
	}
	if err != nil {
		u.fail(err)
		return err
	}
	u.set(func(s *State) { s.Phase, s.Progress = Ready, 100 })
	return nil
}

// parseVersion reads 1.2.3 (with or without the v) as numbers.
func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		return out, false // a pre-release or a development build
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// newer reports whether tag is a later version than current.
func newer(tag, current string) bool {
	a, ok1 := parseVersion(tag)
	b, ok2 := parseVersion(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}
