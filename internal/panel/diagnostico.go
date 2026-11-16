package panel

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/diag"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/mcp"
)

// DiagnosticSaver is a Host that saves the diagnostic file itself (to the
// Downloads folder, shown in the file manager): the app's window cannot
// download a file the way a browser does.
type DiagnosticSaver interface {
	SaveDiagnostic(name string, body []byte) (path string, err error)
}

// diagnosticHours are the periods the page offers.
var diagnosticHours = map[int]bool{24: true, 72: true, 168: true}

// maxAppLogLines caps the app's own log in the file; the diagnostic log is
// the part that matters.
const maxAppLogLines = 4000

func (p *Panel) registerDiagnostic(mux *http.ServeMux) {
	mux.HandleFunc("GET /diagnostico/logs.txt", func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) && !p.fromAgent(r) {
			http.Error(w, "this file is for the panel and programs on this computer", http.StatusForbidden)
			return
		}
		hours := diagnosticPeriod(r.URL.Query().Get("horas"))
		name, body := p.diagnosticReport(r.Context(), hours, time.Now())
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		_, _ = w.Write(body)
	})
	mux.HandleFunc("POST /api/diagnostico", p.api(func(r *http.Request) (any, error) {
		var req struct {
			Hours int `json:"horas"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		hours := diagnosticPeriod(strconv.Itoa(req.Hours))
		saver, ok := p.Host.(DiagnosticSaver)
		if !ok {
			return map[string]string{"url": "/diagnostico/logs.txt?horas=" + strconv.Itoa(hours)}, nil
		}
		name, body := p.diagnosticReport(r.Context(), hours, time.Now())
		path, err := saver.SaveDiagnostic(name, body)
		if err != nil {
			return nil, userError{i18n.Tf("não foi possível salvar o arquivo: %v", err)}
		}
		return map[string]string{"path": path, "name": name}, nil
	}))
}

func diagnosticPeriod(v string) int {
	n, _ := strconv.Atoi(v)
	if diagnosticHours[n] {
		return n
	}
	return 72
}

// diagnosticReport is the file to send for support: what the app knows now,
// the diagnostic log of the period and the app's own log of the period, all
// anonymised.
func (p *Panel) diagnosticReport(ctx context.Context, hours int, now time.Time) (string, []byte) {
	since := now.Add(-time.Duration(hours) * time.Hour)
	var l *diag.Log
	if p.Diag != nil {
		l = p.Diag
	}
	anon := l.Anonymizer()
	clean := func(s string) string {
		if anon == nil {
			return s
		}
		return anon.Text(s)
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "WhatsApp MCP - diagnostics\n")
	fmt.Fprintf(&b, "Created: %s\n", now.Format("2006-01-02 15:04:05 -07:00"))
	fmt.Fprintf(&b, "Period: %s to now (%d hours)\n", since.Format("2006-01-02 15:04"), hours)
	fmt.Fprintf(&b, "App version: %s\nSystem: %s/%s\n", mcp.Version, runtime.GOOS, runtime.GOARCH)
	if p.Host != nil {
		fmt.Fprintf(&b, "wacli: %s\n", p.Host.Settings().WacliVersion)
	}
	b.WriteString("Phone numbers, WhatsApp addresses, message ids, e-mails and links are replaced by codes (user#…, group#…, msg#…).\n")
	b.WriteString("Message text, names and keys are not in this file.\n")

	b.WriteString("\n== Now ==\n")
	if p.Supervisor != nil {
		st := p.Supervisor.Status()
		fmt.Fprintf(&b, "sync: %s since %s, restarts: %d", st.State, st.Since.Format("2006-01-02 15:04:05"), st.Restarts)
		if st.PausedFor != "" {
			fmt.Fprintf(&b, ", paused for: %s", clean(st.PausedFor))
		}
		if st.LastError != "" {
			fmt.Fprintf(&b, ", last error: %s", clean(st.LastError))
		}
		b.WriteString("\n")
		for _, ev := range st.Recent {
			data := ""
			if len(ev.Data) > 0 && anon != nil {
				if js, err := json.Marshal(anon.Value("data", ev.Data)); err == nil {
					data = " " + string(js)
				}
			}
			fmt.Fprintf(&b, "  %s %s%s\n", ev.At.Format("2006-01-02 15:04:05"), ev.Event, data)
		}
	}
	if p.State != nil {
		if clients, err := p.State.Clients(ctx); err == nil && len(clients) > 0 {
			b.WriteString("AI tools that have used the MCP:\n")
			for _, c := range clients {
				fmt.Fprintf(&b, "  %s %s, first %s, last %s\n", clean(c.Name), clean(c.Version),
					c.FirstSeen.Format("2006-01-02 15:04"), c.LastSeen.Format("2006-01-02 15:04:05"))
			}
		}
	}

	b.WriteString("\n== Diagnostics ==\n")
	if l == nil {
		b.WriteString("(the diagnostic log is off)\n")
	} else if n, err := l.WriteSince(&b, since); err != nil {
		fmt.Fprintf(&b, "(could not be read: %s)\n", clean(err.Error()))
	} else if n == 0 {
		b.WriteString("(nothing in the period: the diagnostic log starts with the version that added it)\n")
	}

	b.WriteString("\n== App log ==\n")
	if anon == nil {
		b.WriteString("(left out: no anonymiser)\n")
	} else {
		lines := appLogSince(p.LogPath, since)
		if len(lines) > maxAppLogLines {
			fmt.Fprintf(&b, "(%d older lines left out)\n", len(lines)-maxAppLogLines)
			lines = lines[len(lines)-maxAppLogLines:]
		}
		for _, line := range lines {
			b.WriteString(anon.Text(line))
			b.WriteByte('\n')
		}
	}

	name := "whatsapp-mcp-diagnostics-" + now.Format("2006-01-02-1504") + ".txt"
	return name, b.Bytes()
}

// appLogSince is the app's log (the previous file too) from since on.
func appLogSince(path string, since time.Time) []string {
	if path == "" {
		return nil
	}
	var out []string
	for _, f := range []string{path + ".1", path} {
		file, err := os.Open(f)
		if err != nil {
			continue
		}
		out = append(out, readLogSince(file, since)...)
		file.Close()
	}
	return out
}

// readLogSince keeps the slog lines whose time= is at or after since.
func readLogSince(r io.Reader, since time.Time) []string {
	var out []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		rest, ok := strings.CutPrefix(line, "time=")
		if !ok {
			continue
		}
		stamp, _, _ := strings.Cut(rest, " ")
		at, err := time.Parse(time.RFC3339Nano, stamp)
		if err != nil || at.Before(since) {
			continue
		}
		out = append(out, line)
	}
	return out
}
