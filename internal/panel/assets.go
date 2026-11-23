package panel

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"strings"
)

// revalidated serves the panel's scripts with a tag of their contents, so a
// page that loads again (after a language change, say) gets a "not modified"
// instead of the whole file, and a new version of the app is never served
// the old one: the browser asks every time, and keeps nothing it cannot check.
func revalidated(files fs.FS) http.Handler {
	tags := map[string]string{}
	_ = fs.WalkDir(files, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		body, err := fs.ReadFile(files, name)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(body)
		tags[name] = `"` + hex.EncodeToString(sum[:8]) + `"`
		return nil
	})
	server := http.FileServer(http.FS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tag, ok := tags[strings.TrimPrefix(r.URL.Path, "/")]; ok {
			w.Header().Set("ETag", tag)
			w.Header().Set("Cache-Control", "no-cache")
		}
		server.ServeHTTP(w, r)
	})
}
