// Package diag keeps the diagnostic log: a line for each thing the app does
// that can reach WhatsApp or the person's phone (sync starting, stopping and
// reconnecting, every wacli command, every AI tool that connects and every
// tool it calls, the warnings wacli reports), so a report of something odd on
// the phone can be matched against what happened here, to the second.
//
// Everything is anonymised before it is written: phone numbers, WhatsApp
// addresses, message ids, e-mails and web addresses become short codes, the
// same code for the same value, and message text and names are never
// written. The file can be sent as it is.
//
// There is one file per day in the log folder's "diagnostics" folder, and the
// last seven days are kept.
package diag

import (
	"bufio"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// KeepDays is how many days of files are kept.
	KeepDays = 7
	// maxDayBytes caps one day's file, so a loop of errors cannot fill the
	// disk; past it, only the counters go on.
	maxDayBytes = 40 << 20
	filePrefix  = "diagnostics-"
	fileSuffix  = ".log"
	// stamp is how each line starts; Export reads it back.
	stamp = "2006-01-02 15:04:05.000 -07:00"
)

// Log is the diagnostic log. A nil *Log is valid and records nothing.
type Log struct {
	dir  string
	anon *Anonymizer
	now  func() time.Time

	mu      sync.Mutex
	day     string
	f       *os.File
	written int64
	capped  bool

	// counts are events too frequent to write one by one; they are written
	// together every few minutes.
	counts    map[string]int
	countFrom time.Time
	stop      chan struct{}
}

// Open opens the diagnostic log in dir. The anonymising key is kept in
// keyDir, so codes stay the same across restarts; it never leaves the
// computer and is not in the exported file.
func Open(dir, keyDir string) (*Log, error) { return open(dir, keyDir, time.Now) }

func open(dir, keyDir string, now func() time.Time) (*Log, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	key, err := loadKey(keyDir)
	if err != nil {
		return nil, err
	}
	home, _ := os.UserHomeDir()
	l := &Log{dir: dir, anon: NewAnonymizer(key, home), now: now, counts: map[string]int{}, stop: make(chan struct{})}
	l.countFrom = l.now()
	l.prune(l.now())
	go l.flushLoop()
	return l, nil
}

func loadKey(dir string) ([]byte, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "diagnostics.key")
	if b, err := os.ReadFile(path); err == nil && len(b) >= 16 {
		return b, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

// Dir is the folder the daily files are in.
func (l *Log) Dir() string {
	if l == nil {
		return ""
	}
	return l.dir
}

// Anonymizer is the one the log writes with, for text from elsewhere (the
// app's own log) that goes into an export.
func (l *Log) Anonymizer() *Anonymizer {
	if l == nil {
		return nil
	}
	return l.anon
}

// Close writes the pending counters and closes the file.
func (l *Log) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	select {
	case <-l.stop:
	default:
		close(l.stop)
	}
	l.flushCountsLocked()
	if l.f != nil {
		_ = l.f.Close()
		l.f = nil
	}
	l.mu.Unlock()
}

// Fields are an event's details. Values are anonymised as they are written;
// keys that hold text or names (text, caption, name...) are written as
// <omitted>, whatever they hold.
type Fields map[string]any

// Event writes one line: where it came from (sync, wacli, mcp, app), what
// happened, and its details.
func (l *Log) Event(source, event string, fields Fields) {
	if l == nil {
		return
	}
	now := l.now()
	var b strings.Builder
	b.WriteString(now.Format(stamp))
	b.WriteByte(' ')
	b.WriteString(source)
	b.WriteByte(' ')
	b.WriteString(event)
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteByte(' ')
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(formatValue(l.anon.Value(k, fields[k])))
	}
	b.WriteByte('\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writeLocked(now, b.String())
}

// Count adds one to a counter written with the others every few minutes,
// for events that come too often to be lines of their own.
func (l *Log) Count(name string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.counts[name]++
	l.mu.Unlock()
}

// FlushCounts writes the counters now (sync stopping, for instance), so the
// counts before an event are not mixed with those after it.
func (l *Log) FlushCounts() {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.flushCountsLocked()
	l.mu.Unlock()
}

func (l *Log) flushLoop() {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-t.C:
			l.FlushCounts()
		}
	}
}

func (l *Log) flushCountsLocked() {
	now := l.now()
	if len(l.counts) == 0 {
		l.countFrom = now
		return
	}
	names := make([]string, 0, len(l.counts))
	for k := range l.counts {
		names = append(names, k)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString(now.Format(stamp))
	b.WriteString(" app counts since=")
	b.WriteString(l.countFrom.Format("15:04:05"))
	for _, k := range names {
		fmt.Fprintf(&b, " %s=%d", k, l.counts[k])
	}
	b.WriteByte('\n')
	l.counts = map[string]int{}
	l.countFrom = now
	l.writeLocked(now, b.String())
}

func (l *Log) writeLocked(now time.Time, line string) {
	day := now.Format("2006-01-02")
	if day != l.day || l.f == nil {
		if l.f != nil {
			_ = l.f.Close()
			l.f = nil
		}
		l.day, l.written, l.capped = day, 0, false
		path := filepath.Join(l.dir, filePrefix+day+fileSuffix)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		if info, err := f.Stat(); err == nil {
			l.written = info.Size()
		}
		l.f = f
		go l.prune(now)
	}
	if l.capped {
		return
	}
	if l.written+int64(len(line)) > maxDayBytes {
		l.capped = true
		line = now.Format(stamp) + " app daily_limit_reached\n"
	}
	n, _ := io.WriteString(l.f, line)
	l.written += int64(n)
}

// prune deletes the files older than KeepDays.
func (l *Log) prune(now time.Time) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return
	}
	oldest := now.AddDate(0, 0, -KeepDays).Format("2006-01-02")
	for _, e := range entries {
		day, ok := fileDay(e.Name())
		if ok && day < oldest {
			_ = os.Remove(filepath.Join(l.dir, e.Name()))
		}
	}
}

func fileDay(name string) (string, bool) {
	if !strings.HasPrefix(name, filePrefix) || !strings.HasSuffix(name, fileSuffix) {
		return "", false
	}
	day := strings.TrimSuffix(strings.TrimPrefix(name, filePrefix), fileSuffix)
	if _, err := time.Parse("2006-01-02", day); err != nil {
		return "", false
	}
	return day, true
}

// WriteSince copies to w the lines written since since, oldest first, and
// says how many there were.
func (l *Log) WriteSince(w io.Writer, since time.Time) (int, error) {
	if l == nil {
		return 0, nil
	}
	l.FlushCounts()
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return 0, err
	}
	first := since.Format("2006-01-02")
	var days []string
	for _, e := range entries {
		if day, ok := fileDay(e.Name()); ok && day >= first {
			days = append(days, day)
		}
	}
	sort.Strings(days)
	n := 0
	for _, day := range days {
		f, err := os.Open(filepath.Join(l.dir, filePrefix+day+fileSuffix))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if len(line) < len(stamp) {
				continue
			}
			at, err := time.Parse(stamp, line[:len(stamp)])
			if err != nil || at.Before(since) {
				continue
			}
			if _, err := io.WriteString(w, line+"\n"); err != nil {
				f.Close()
				return n, err
			}
			n++
		}
		f.Close()
	}
	return n, nil
}

// formatValue writes a value as a word, quoted when it has spaces, or as
// compact JSON when it is a list or an object.
func formatValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "-"
	case string:
		if x == "" {
			return `""`
		}
		if strings.ContainsAny(x, " \t\n\"=") {
			return strconv.Quote(x)
		}
		return x
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case time.Duration:
		return strconv.FormatInt(x.Milliseconds(), 10) + "ms"
	default:
		var buf strings.Builder
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(x); err != nil {
			return strconv.Quote(fmt.Sprint(x))
		}
		return strings.TrimSuffix(buf.String(), "\n")
	}
}

// code is the short code for a value: the same value always gets the same
// code on this computer, and the code does not give the value back.
func code(key []byte, kind, value string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(kind))
	m.Write([]byte{0})
	m.Write([]byte(value))
	return kind + "#" + hex.EncodeToString(m.Sum(nil)[:4])
}
