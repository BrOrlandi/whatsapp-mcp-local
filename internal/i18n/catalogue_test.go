package i18n

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The calls that take a text to translate: {{t "…"}} and {{tf "…" …}} in the
// templates, i18n.T, i18n.Tf and i18n.N in Go, t("…") in the panel's scripts.
var (
	templateCall = regexp.MustCompile(`\{\{-?\s*\(?\s*tf?\s+("(?:[^"\\]|\\.)*")`)
	goCall       = regexp.MustCompile(`\bi18n\.(?:Tf?|N)\(\s*("(?:[^"\\]|\\.)*"|` + "`[^`]*`" + `)`)
	scriptCall   = regexp.MustCompile(`\bt\(\s*("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*')`)
	verb         = regexp.MustCompile(`%[sdvq]`)
)

// keys finds every text the code asks to translate, with where it is.
func keys(t *testing.T) map[string]string {
	t.Helper()
	root := filepath.Join("..", "..")
	found := map[string]string{}
	add := func(lit, where string) {
		if strings.HasPrefix(lit, "'") {
			lit = `"` + strings.ReplaceAll(strings.ReplaceAll(lit[1:len(lit)-1], `\'`, `'`), `"`, `\"`) + `"`
		}
		s, err := strconv.Unquote(lit)
		if err != nil {
			t.Errorf("%s: %s is not a string literal: %v", where, lit, err)
			return
		}
		if _, ok := found[s]; !ok {
			found[s] = where
		}
	}
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || strings.HasSuffix(path, "_test.go") {
				return err
			}
			ext := filepath.Ext(path)
			if ext != ".go" && ext != ".js" {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			src, rel := string(raw), strings.TrimPrefix(path, root+string(filepath.Separator))
			res := []*regexp.Regexp{scriptCall}
			if ext == ".go" {
				res = []*regexp.Regexp{templateCall, goCall}
			}
			for _, re := range res {
				for _, m := range re.FindAllStringSubmatch(src, -1) {
					add(m[1], rel)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(found) == 0 {
		t.Fatal("found no text to translate")
	}
	return found
}

// TestCatalogues keeps every catalogue whole: an entry for each text, the
// same verbs in the same order, and nothing the code no longer asks for.
func TestCatalogues(t *testing.T) {
	want := keys(t)
	for _, l := range Languages[1:] {
		c := catalogues[l.Code]
		var missing []string
		for k, where := range want {
			tr, ok := c[k]
			if !ok || tr == "" {
				missing = append(missing, where+": "+strconv.Quote(k))
				continue
			}
			if a, b := verb.FindAllString(k, -1), verb.FindAllString(tr, -1); strings.Join(a, "") != strings.Join(b, "") {
				t.Errorf("locales/%s.json: %q has the verbs %v, its translation %v", l.Code, k, a, b)
			}
		}
		sort.Strings(missing)
		for _, m := range missing {
			t.Errorf("locales/%s.json has no translation for %s", l.Code, m)
		}
		for k := range c {
			if _, ok := want[k]; !ok {
				t.Errorf("locales/%s.json translates %q, which the code no longer uses", l.Code, k)
			}
		}
	}
}

func TestMatch(t *testing.T) {
	for in, want := range map[string]string{
		"pt_BR.UTF-8": Source, "pt-PT": Source, "es-CL": "es", "es_419": "es",
		"en-US": "en", "fr_FR.UTF-8": "en", "": "en",
	} {
		if got := match(in); got != want {
			t.Errorf("match(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFallback(t *testing.T) {
	defer current.Store(Source)
	current.Store("es")
	if got := T("um texto que nenhum catálogo tem"); got != "um texto que nenhum catálogo tem" {
		t.Errorf("T fell back to %q", got)
	}
}
