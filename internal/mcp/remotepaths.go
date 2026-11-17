package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// An agent on another computer never reads where things are on the server:
// in its answers, the server's folders (media, exports, wacli's store, the
// app's data, the home and temporary folders, wacli's own path), wherever
// they appear (media_stats, the purge_media preview, whatsapp_status's
// wacli report, an error wacli wrote), become a short name. Programs on
// the server itself get every answer as it is.

type serverPath struct{ path, name string }

// serverPaths are the folders an answer may name, longest first.
func (s *Server) serverPaths() []serverPath {
	var out []serverPath
	add := func(p, name string) {
		if p == "" {
			return
		}
		p = filepath.Clean(p)
		if p == "." || p == string(filepath.Separator) || !filepath.IsAbs(p) {
			return
		}
		out = append(out, serverPath{p, name})
	}
	add(s.mediaDir, "media")
	add(s.exportDir, "exports")
	if s.mediaDir != "" {
		add(filepath.Dir(s.mediaDir), "data")
	}
	if s.cli != nil {
		add(s.cli.StoreDir, "wacli-store")
		add(s.cli.Bin, "wacli")
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(home, "~")
	}
	add(os.TempDir(), "tmp")
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].path) > len(out[j].path) })
	return out
}

// withoutServerPaths rewrites a tool answer for an agent outside.
func (s *Server) withoutServerPaths(result map[string]any) map[string]any {
	paths := s.serverPaths()
	content, _ := result["content"].([]any)
	for _, item := range content {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		for _, key := range []string{"text", "uri"} {
			if v, ok := m[key].(string); ok {
				m[key] = replacePaths(v, paths)
			}
		}
		if r, ok := m["resource"].(map[string]any); ok {
			for _, key := range []string{"text", "uri"} {
				if v, ok := r[key].(string); ok {
					r[key] = replacePaths(v, paths)
				}
			}
		}
	}
	return result
}

// replacePaths puts each folder's name where the folder appears whole (as
// itself or followed by a separator), as written and as JSON writes it.
func replacePaths(text string, paths []serverPath) string {
	for _, p := range paths {
		escaped, _ := json.Marshal(p.path)
		forms := []string{p.path}
		if e := string(escaped[1 : len(escaped)-1]); e != p.path {
			forms = append([]string{e}, forms...)
		}
		for _, form := range forms {
			text = replaceWhole(text, form, p.name)
		}
	}
	return text
}

func replaceWhole(text, path, name string) string {
	var b strings.Builder
	from := 0
	for {
		j := strings.Index(text[from:], path)
		if j < 0 {
			b.WriteString(text[from:])
			return b.String()
		}
		i, end := from+j, from+j+len(path)
		prev, next := byte(0), byte(0)
		if i > 0 {
			prev = text[i-1]
		}
		if end < len(text) {
			next = text[end]
		}
		startsWhole := prev == 0 || strings.IndexByte("\"' \t\n([{=,:", prev) >= 0 || strings.HasSuffix(text[:i], "file://")
		endsWhole := next == 0 || strings.IndexByte("/\\\"' \t\n:,;)]}", next) >= 0
		b.WriteString(text[from:i])
		if startsWhole && endsWhole {
			b.WriteString(name)
		} else {
			b.WriteString(path)
		}
		from = end
	}
}
