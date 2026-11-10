// Package transfer moves a number's data between this computer and a server
// (docs/modo-servidor.md#levar-os-dados-para-o-servidor): wacli's store, with
// the WhatsApp session, and the gateway's own data (transcripts, webhooks,
// settings, downloaded media). It travels as a gzipped tar, inside the app's
// encrypted session with the server.
package transfer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
	_ "modernc.org/sqlite"
)

// The tar's two roots: wacli's store and the gateway's data folder.
const (
	storeRoot = "wacli"
	dataRoot  = "mcp"
)

// dataItems are what of the data folder belongs to the number. Everything
// else there (the server's key, the API keys, the token, the app's
// settings, the transcription models) stays with the machine.
var dataItems = []string{"state.db", "media", "exports"}

// WacliVersion is what the wacli at bin says it is ("wacli 0.20.0"). Both
// sides must run the same one: the store's schema can change between
// versions.
func WacliVersion(ctx context.Context, bin string) string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// skip reports whether a file of wacli's store stays behind: its lock, its
// socket and its heartbeat belong to the process that ran it.
func skip(name string) bool {
	return name == "LOCK" || name == "HEARTBEAT" || strings.HasSuffix(name, ".sock")
}

// Size is roughly how many bytes Pack reads, for a progress bar.
func Size(storeDir, dataDir string) int64 {
	var n int64
	add := func(root string) {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && d.Type().IsRegular() && !skip(d.Name()) {
				if info, err := d.Info(); err == nil {
					n += info.Size()
				}
			}
			return nil
		})
	}
	add(storeDir)
	for _, item := range dataItems {
		add(filepath.Join(dataDir, item))
	}
	return n
}

// Pack writes the number's data as a gzipped tar. Nothing may write to
// wacli's store meanwhile: sync stopped, or the gateway closed. The state
// database may be open: it is copied with VACUUM INTO, which reads one
// consistent snapshot. progress, when set, gets the bytes of each file as
// they are read.
func Pack(ctx context.Context, w io.Writer, storeDir, dataDir string, progress func(int64)) error {
	gz, _ := gzip.NewWriterLevel(w, gzip.BestSpeed)
	tw := &tarWriter{Writer: tar.NewWriter(gz), progress: progress}
	if err := addTree(ctx, tw, storeDir, storeRoot); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dataDir, "state.db")); err == nil {
		tmp, err := os.MkdirTemp(dataDir, ".transfer-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		snapshot := filepath.Join(tmp, "state.db")
		if err := vacuumInto(ctx, filepath.Join(dataDir, "state.db"), snapshot); err != nil {
			return fmt.Errorf("copying state.db: %w", err)
		}
		if err := addFile(tw, snapshot, dataRoot+"/state.db"); err != nil {
			return err
		}
	}
	for _, item := range dataItems[1:] {
		if err := addTree(ctx, tw, filepath.Join(dataDir, item), dataRoot+"/"+item); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// tarWriter counts what goes into the tar's files.
type tarWriter struct {
	*tar.Writer
	progress func(int64)
}

func (t *tarWriter) Write(b []byte) (int, error) {
	n, err := t.Writer.Write(b)
	if t.progress != nil && n > 0 {
		t.progress(int64(n))
	}
	return n, err
}

func vacuumInto(ctx context.Context, src, dst string) error {
	db, err := sql.Open("sqlite", "file:"+src+"?_pragma=busy_timeout(10000)")
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, "VACUUM INTO ?", dst)
	return err
}

// addTree adds the regular files under root, a missing root adding nothing.
func addTree(ctx context.Context, tw *tarWriter, root, name string) error {
	if _, err := os.Stat(root); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if p == root || skip(d.Name()) || !(d.IsDir() || d.Type().IsRegular()) {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		target := path.Join(name, filepath.ToSlash(rel))
		if d.IsDir() {
			return tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: target + "/", Mode: 0o700})
		}
		return addFile(tw, p, target)
	})
}

func addFile(tw *tarWriter, p, name string) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o600, Size: info.Size(), ModTime: info.ModTime()}); err != nil {
		return err
	}
	_, err = io.CopyN(tw, f, info.Size())
	return err
}

// Staging is where Unpack puts what arrives, next to where it will live, so
// Install only renames.
type Staging struct {
	Store string // beside the store: <store>.recebendo
	Data  string // inside the data folder: <data>/.recebendo
}

// NewStaging makes empty staging folders for a store and a data folder.
func NewStaging(storeDir, dataDir string) (Staging, error) {
	s := Staging{Store: storeDir + ".recebendo", Data: filepath.Join(dataDir, ".recebendo")}
	s.Remove()
	for _, dir := range []string{s.Store, s.Data} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return s, err
		}
	}
	return s, nil
}

// Remove deletes what arrived.
func (s Staging) Remove() {
	_ = os.RemoveAll(s.Store)
	_ = os.RemoveAll(s.Data)
}

// Unpack reads a gzipped tar Pack wrote into the staging folders. It takes
// only the two roots, regular files and folders, and nothing that climbs
// out of them. progress, when set, gets the bytes of each file as they are
// written.
func Unpack(r io.Reader, s Staging, progress func(int64)) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	got := false
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		dest, ok := s.target(h.Name)
		if !ok {
			return fmt.Errorf("transfer: unexpected entry %q", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
				return err
			}
			f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				return err
			}
			_, err = io.Copy(&counter{w: f, progress: progress}, tr)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
			if !h.ModTime.IsZero() {
				_ = os.Chtimes(dest, h.ModTime, h.ModTime)
			}
			got = got || h.Name == storeRoot+"/session.db"
		default:
			return fmt.Errorf("transfer: %q is not a file or a folder", h.Name)
		}
	}
	if !got {
		return errors.New("transfer: the data has no WhatsApp session (wacli/session.db)")
	}
	return nil
}

// target is where an entry goes, false for one outside the two roots.
func (s Staging) target(name string) (string, bool) {
	clean := path.Clean(strings.TrimSuffix(name, "/"))
	if clean != name && clean+"/" != name || strings.HasPrefix(clean, "/") {
		return "", false
	}
	root, rest, _ := strings.Cut(clean, "/")
	if rest == "" || rest == ".." || strings.HasPrefix(rest, "../") {
		return "", false
	}
	switch root {
	case storeRoot:
		return filepath.Join(s.Store, filepath.FromSlash(rest)), true
	case dataRoot:
		item, _, _ := strings.Cut(rest, "/")
		for _, allowed := range dataItems {
			if item == allowed {
				return filepath.Join(s.Data, filepath.FromSlash(rest)), true
			}
		}
	}
	return "", false
}

// Install puts what arrived in place, with the gateway closed. What was
// there goes aside, to <store>.antes-<time> and <data>/antes-<time>, never
// deleted: a transfer that went wrong can be undone by hand. keep, when
// set, runs on the arriving state database with the previous one (or ""),
// for what of the machine's must survive.
func (s Staging) Install(storeDir, dataDir string, keep func(arriving, previous string) error) (aside []string, err error) {
	stamp := time.Now().Format("20060102-150405")
	if keep != nil {
		previous := filepath.Join(dataDir, "state.db")
		if _, err := os.Stat(previous); err != nil {
			previous = ""
		}
		if err := keep(filepath.Join(s.Data, "state.db"), previous); err != nil {
			return nil, err
		}
	}
	oldStore := storeDir + ".antes-" + stamp
	if err := os.Rename(storeDir, oldStore); err == nil {
		aside = append(aside, oldStore)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err := os.Rename(s.Store, storeDir); err != nil {
		return aside, err
	}
	oldData := filepath.Join(dataDir, "antes-"+stamp)
	for _, item := range dataItems {
		names := []string{item}
		if item == "state.db" {
			names = append(names, "state.db-wal", "state.db-shm")
		}
		for _, name := range names {
			from := filepath.Join(dataDir, name)
			if _, err := os.Stat(from); err != nil {
				continue
			}
			if err := os.MkdirAll(oldData, 0o700); err != nil {
				return aside, err
			}
			if err := os.Rename(from, filepath.Join(oldData, name)); err != nil {
				return aside, err
			}
		}
		for _, name := range names {
			arrived := filepath.Join(s.Data, name)
			if _, err := os.Stat(arrived); err != nil {
				continue
			}
			if err := os.Rename(arrived, filepath.Join(dataDir, name)); err != nil {
				return aside, err
			}
		}
	}
	if _, err := os.Stat(oldData); err == nil {
		aside = append(aside, oldData)
	}
	_ = os.RemoveAll(s.Data)
	return aside, nil
}

// RemoveData deletes a number's data from a store and a data folder, for when
// it moved to the server and this computer must not keep a copy.
func RemoveData(storeDir, dataDir string) error {
	errs := []error{os.RemoveAll(storeDir)}
	for _, item := range append(dataItems, "state.db-wal", "state.db-shm") {
		errs = append(errs, os.RemoveAll(filepath.Join(dataDir, item)))
	}
	return errors.Join(errs...)
}

// KeepRemoteClients is a keep for Install: the devices paired with this
// machine (state.db's remote_clients) stay this machine's, and none of the
// other side's come along.
func KeepRemoteClients(arriving, previous string) error {
	if _, err := os.Stat(arriving); err != nil {
		return nil // no state database came: the gateway makes a new one
	}
	// Opening it brings its tables up to this version's.
	st, err := state.Open(filepath.Dir(arriving))
	if err != nil {
		return err
	}
	st.Close()
	db, err := sql.Open("sqlite", "file:"+arriving+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`DELETE FROM remote_clients`); err != nil {
		return err
	}
	if previous != "" {
		if _, err := db.Exec(`ATTACH DATABASE ? AS prev`, "file:"+previous+"?mode=ro"); err != nil {
			return err
		}
		cols, err := common(db)
		if err != nil {
			return err
		}
		if cols != "" {
			if _, err := db.Exec(`INSERT INTO main.remote_clients (` + cols + `) SELECT ` + cols + ` FROM prev.remote_clients`); err != nil {
				return err
			}
		}
		if _, err := db.Exec(`DETACH DATABASE prev`); err != nil {
			return err
		}
	}
	// One file, with nothing left in a write-ahead log.
	_, err = db.Exec(`PRAGMA journal_mode=DELETE`)
	return err
}

// common lists the columns remote_clients has on both sides, "" when the
// previous database has no such table.
func common(db *sql.DB) (string, error) {
	columns := func(schema string) (map[string]bool, []string, error) {
		rows, err := db.Query(`SELECT name FROM pragma_table_info('remote_clients', ?)`, schema)
		if err != nil {
			return nil, nil, err
		}
		defer rows.Close()
		set, list := map[string]bool{}, []string{}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, nil, err
			}
			set[name] = true
			list = append(list, name)
		}
		return set, list, rows.Err()
	}
	prev, _, err := columns("prev")
	if err != nil {
		return "", err
	}
	_, main, err := columns("main")
	if err != nil {
		return "", err
	}
	var both []string
	for _, c := range main {
		if prev[c] {
			both = append(both, `"`+c+`"`)
		}
	}
	return strings.Join(both, ", "), nil
}

type counter struct {
	w        io.Writer
	progress func(int64)
}

func (c *counter) Write(b []byte) (int, error) {
	n, err := c.w.Write(b)
	if c.progress != nil && n > 0 {
		c.progress(int64(n))
	}
	return n, err
}
