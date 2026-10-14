package mcp

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// What the tools answer goes to the AI and stays the same whatever language
// the panel shows: nothing in this package goes through the catalogue.
func TestAnswersDoNotFollowTheLanguage(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), f, src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range parsed.Imports {
			if path, _ := strconv.Unquote(imp.Path.Value); strings.HasSuffix(path, "/internal/i18n") {
				t.Errorf("%s imports i18n: the tools' answers would follow the panel's language", f)
			}
		}
	}
}
