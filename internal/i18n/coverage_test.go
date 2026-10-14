package i18n

// TestCoverage looks for text a person reads that does not go through the
// catalogue: words in the templates outside {{t}}/{{tf}}, string literals in
// the panel's scripts outside t(…), and Portuguese string literals in Go
// outside i18n.T/Tf/N. It is a heuristic: what it cannot tell apart from a
// person's text (a brand, a field name, a CSS selector) goes in the
// allowlists below, each with why.

import (
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// Words that are the same in every language: brands, products, protocols,
// field and event names the webhook docs show as code.
var sameEverywhere = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`WhatsApp MCP Webhooks Webhook Claude Desktop Code Codex Cursor ChatGPT Windsurf
		Cowork Python Node js JSON JSONs HTTP HTTPS URL API Prompt GitHub Whisper NVIDIA Mac Content Type application json X Event Delivery Signature
		event delivery_id sent_at test id chat_jid chat_name group timestamp from_me sender_jid sender_name text media
		reply_to reaction location poll forwarded edited revoked receipt message_ids type
		receptor py PDF OK Ok v W`) {
		sameEverywhere[strings.ToLower(w)] = true
	}
}

var (
	tplComment = regexp.MustCompile(`(?s)\{\{/\*.*?\*/\}\}`)
	tplAction  = regexp.MustCompile(`(?s)\{\{.*?\}\}`)
	blockTags  = regexp.MustCompile(`(?is)<(script|style|code|svg)\b.*?</(script|style|code|svg)>`)
	tag        = regexp.MustCompile(`(?s)<[^>]*>`)
	personAttr = regexp.MustCompile(`(?i)\b(title|aria-label|alt|placeholder|data-busy|label)="([^"]*)"`)
	entity     = regexp.MustCompile(`&[a-zA-Z#0-9]+;`)
	word       = regexp.MustCompile(`[\p{L}][\p{L}\p{N}_']*`)
)

// untranslated is what a piece of template text says outside the actions,
// minus the words that need no translation; empty when nothing is left.
func untranslated(s string) string {
	s = entity.ReplaceAllString(s, " ")
	var left []string
	for _, w := range word.FindAllString(s, -1) {
		if !sameEverywhere[strings.ToLower(w)] {
			left = append(left, w)
		}
	}
	return strings.Join(left, " ")
}

func lineOf(src string, off int) int { return strings.Count(src[:off], "\n") + 1 }

// templateSources are the Go constants that hold templates.
func templateSources(t *testing.T, path string) (string, int) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out string
	var line int
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range vs.Names {
			if !strings.HasSuffix(name.Name, "Source") || i >= len(vs.Values) {
				continue
			}
			if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				s, _ := strconv.Unquote(lit.Value)
				out, line = s, fset.Position(lit.Pos()).Line
			}
		}
		return true
	})
	return out, line
}

func TestTemplateCoverage(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, file := range []string{"internal/panel/templates.go", "internal/panel/webhookdocs.go"} {
		src, base := templateSources(t, filepath.Join(root, file))
		// Blank what is not a person's text, keeping offsets for line numbers.
		blank := func(re *regexp.Regexp, s string) string {
			return re.ReplaceAllStringFunc(s, func(m string) string {
				return strings.Map(func(r rune) rune {
					if r == '\n' {
						return r
					}
					return ' '
				}, m)
			})
		}
		clean := blank(tplComment, src)
		clean = blank(tplAction, clean)
		clean = blank(blockTags, clean)
		for _, m := range personAttr.FindAllStringSubmatchIndex(clean, -1) {
			if left := untranslated(clean[m[4]:m[5]]); left != "" {
				t.Errorf("%s:%d: attribute %s=%q is not translated", file, base+lineOf(src, m[0])-1, clean[m[2]:m[3]], strings.TrimSpace(clean[m[4]:m[5]]))
			}
		}
		text := blank(tag, clean)
		for _, m := range regexp.MustCompile(`[^\n]+`).FindAllStringIndex(text, -1) {
			if left := untranslated(text[m[0]:m[1]]); left != "" {
				t.Errorf("%s:%d: text %q is not translated", file, base+lineOf(src, m[0])-1, strings.Join(strings.Fields(text[m[0]:m[1]]), " "))
			}
		}
	}
}

// jsString is a string literal in a script, with whether it is the first
// argument of t(…).
type jsString struct {
	line  int
	value string
	inT   bool
}

var tCall = regexp.MustCompile(`(^|[^.\w])t\(\s*$`)

// jsStrings finds a script's string literals, skipping comments and regular
// expressions well enough for the panel's own scripts.
func jsStrings(src string) []jsString {
	var out []jsString
	line := 1
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '\n':
			line++
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			line++
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			end := strings.Index(src[i+2:], "*/")
			line += strings.Count(src[i:i+2+end], "\n")
			i += end + 3
		case c == '/' && strings.ContainsRune("(,=:[!&|?{};", rune(lastNonSpace(src, i))):
			// a regular expression literal
			for i++; i < len(src) && src[i] != '/'; i++ {
				if src[i] == '\\' {
					i++
				}
			}
		case c == '"' || c == '\'':
			start := i
			for i++; i < len(src) && src[i] != c; i++ {
				if src[i] == '\\' {
					i++
				}
			}
			from := start - 12
			if from < 0 {
				from = 0
			}
			out = append(out, jsString{line: line, value: src[start+1 : i], inT: tCall.MatchString(src[from:start])})
		}
	}
	return out
}

func lastNonSpace(src string, i int) byte {
	for j := i - 1; j >= 0; j-- {
		if src[j] != ' ' && src[j] != '\t' && src[j] != '\n' {
			return src[j]
		}
	}
	return '('
}

func TestScriptCoverage(t *testing.T) {
	dir := filepath.Join("..", "..", "internal", "panel", "assets")
	vocab := ptVocabulary()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".js" {
			continue
		}
		raw, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		for _, s := range jsStrings(string(raw)) {
			if s.inT || !portuguese(strings.TrimSpace(s.value), vocab) {
				continue
			}
			t.Errorf("internal/panel/assets/%s:%d: %q is not inside t()", e.Name(), s.line, s.value)
		}
	}
}

// portuguese says a Go literal reads as Portuguese: a letter only Portuguese
// uses, or a word the catalogues' Portuguese keys have and their English
// translations do not ("conectado", "mensagens"). A single lowercase word is
// an identifier, a route or a key, not text.
var ptLetters = regexp.MustCompile(`[ãõçáéíóúâêôàÃÕÇÁÉÍÓÚÂÊÔ]`)

func ptVocabulary() map[string]bool {
	words := regexp.MustCompile(`[\p{L}][\p{L}'-]*`)
	en := catalogues["en"]
	english := map[string]bool{}
	for _, v := range en {
		for _, w := range words.FindAllString(v, -1) {
			english[strings.ToLower(w)] = true
		}
	}
	out := map[string]bool{}
	for k := range en {
		for _, w := range words.FindAllString(k, -1) {
			if w = strings.ToLower(w); len([]rune(w)) >= 4 && !english[w] {
				out[w] = true
			}
		}
	}
	return out
}

func portuguese(s string, vocab map[string]bool) bool {
	t := strings.TrimSpace(s)
	if !strings.Contains(t, " ") && (t == "" || !unicode.IsUpper([]rune(t)[0])) {
		return false
	}
	if strings.HasPrefix(t, "GET ") || strings.HasPrefix(t, "POST ") || strings.HasPrefix(t, "http") {
		return false
	}
	if ptLetters.MatchString(t) {
		return true
	}
	for _, w := range regexp.MustCompile(`[\p{L}][\p{L}'-]*`).FindAllString(t, -1) {
		if vocab[strings.ToLower(w)] && !alsoEnglish[strings.ToLower(w)] {
			return true
		}
	}
	return false
}

// alsoEnglish are Portuguese words of the catalogue that English uses too.
var alsoEnglish = map[string]bool{"serve": true}

// goAllow are files whose Portuguese is not a person's text: what the AI
// reads (tool descriptions, which quote WhatsApp users' Portuguese), what
// the code parses, and the stylesheet.
var goAllow = map[string]bool{
	"internal/mcp/triage.go": true, // short Portuguese replies the triage recognises
	"internal/mcp/tools.go":  true, // tool descriptions, for the AI
	"internal/mcp/health.go": true, // the health report, for the AI
	// What the AI reads, in English, naming the panel's Portuguese labels as
	// they were before the languages: the same whatever language the panel
	// shows.
	"internal/mcp/server.go":        true, // the server's instructions
	"internal/mcp/status.go":        true, // whatsapp_status
	"internal/mcp/transcription.go": true, // transcribe_audio
	"internal/panel/style.go":       true,
	"internal/i18n/i18n.go":         true, // the languages' own names
}

// contract is Portuguese that programs receive, which the language of the
// interface does not change: the webhooks' test event and the errors
// transcribe_audio gives the AI.
var contract = map[string]bool{
	"Teste":                             true,
	"Mensagem de teste do WhatsApp MCP": true,
	"a transcrição local não está instalada (falta: %s)": true,
	"ffmpeg não conseguiu ler o áudio: %s":               true,
	"whisper.cpp falhou: %s":                             true,
	"whisper.cpp não devolveu texto: %w":                 true,
}

func TestGoCoverage(t *testing.T) {
	root := filepath.Join("..", "..")
	vocab := ptVocabulary()
	var problems []string
	for _, top := range []string{"cmd", "internal"} {
		_ = filepath.WalkDir(filepath.Join(root, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return err
			}
			rel := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
			if goAllow[rel] {
				return nil
			}
			src, _ := os.ReadFile(path)
			fset := token.NewFileSet()
			var s scanner.Scanner
			s.Init(fset.AddFile(path, -1, len(src)), src, nil, 0)
			var window []token.Token
			var lits []string
			for {
				pos, tok, lit := s.Scan()
				if tok == token.EOF {
					break
				}
				if tok == token.STRING {
					// i18n.T( "…" ): IDENT PERIOD IDENT LPAREN STRING
					n := len(window)
					wrapped := n >= 4 && window[n-1] == token.LPAREN && window[n-2] == token.IDENT && window[n-3] == token.PERIOD && window[n-4] == token.IDENT &&
						lits[n-4] == "i18n" && (lits[n-2] == "T" || lits[n-2] == "Tf" || lits[n-2] == "N")
					v, _ := strconv.Unquote(lit)
					if !wrapped && !contract[v] && portuguese(v, vocab) && !strings.HasSuffix(rel, "templates.go") && !strings.HasSuffix(rel, "webhookdocs.go") {
						short := v
						if len(short) > 90 {
							short = short[:90] + "…"
						}
						problems = append(problems, rel+":"+strconv.Itoa(fset.Position(pos).Line)+": "+strconv.Quote(short))
					}
				}
				window = append(window, tok)
				lits = append(lits, lit)
			}
			return nil
		})
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Errorf("%s is Portuguese outside i18n", p)
	}
}
