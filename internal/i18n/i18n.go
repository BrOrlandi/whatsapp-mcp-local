// Package i18n translates what the app shows a person. The panel, the tray and
// the dialogs are written in Brazilian Portuguese, and that text is the key:
// T looks it up in the catalogue of the language in use and falls back to the
// Portuguese when the catalogue has no entry. A catalogue is a JSON object in
// locales/, from the Portuguese to its translation, with the same %s and %d
// verbs in the same order. The language is the interface's only: what goes to
// programs, the tools' descriptions and results for the AI and the webhooks'
// events, is the same in every language, as it was before the languages
// (English for the AI, naming the panel's Portuguese labels; the messages,
// names and test event as they are). index.ShownName translates the name of a
// chat without one for the panel alone.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

// Source is the language the text is written in.
const Source = "pt-BR"

// Language is one the app can show.
type Language struct {
	Code string `json:"code"`
	// Name is the language in itself, as the pickers list it, beside the
	// flag that tells Brazil's Portuguese apart.
	Name string `json:"name"`
}

// Languages are the ones with a catalogue, Portuguese first.
var Languages = []Language{
	{Source, "Português"},
	{"es", "Español"},
	{"en", "English"},
}

//go:embed locales/*.json
var files embed.FS

var (
	catalogues = map[string]map[string]string{}
	current    atomic.Value // string
)

func init() {
	for _, l := range Languages[1:] {
		raw, err := files.ReadFile("locales/" + l.Code + ".json")
		if err != nil {
			panic(err)
		}
		c := map[string]string{}
		if err := json.Unmarshal(raw, &c); err != nil {
			panic("i18n: locales/" + l.Code + ".json: " + err.Error())
		}
		catalogues[l.Code] = c
	}
	current.Store(Source)
}

// Supported reports whether code names a language with a catalogue.
func Supported(code string) bool {
	for _, l := range Languages {
		if l.Code == code {
			return true
		}
	}
	return false
}

// Set changes the language in use. An empty or unknown code follows the
// system's language.
func Set(code string) {
	if !Supported(code) {
		code = Detect()
	}
	current.Store(code)
}

// Lang is the language in use, as an HTML lang attribute.
func Lang() string { return current.Load().(string) }

// T is s in the language in use.
func T(s string) string {
	if c := catalogues[Lang()]; c != nil {
		if t, ok := c[s]; ok && t != "" {
			return t
		}
	}
	return s
}

// N marks s as a text to translate where it is written, in a value kept for
// later (a list of recipes, a label in a table), and returns it as it is: T
// translates it when it is shown. The catalogue test finds the texts by it.
func N(s string) string { return s }

// Tf is T of a format, filled in with args.
func Tf(format string, args ...any) string { return fmt.Sprintf(T(format), args...) }

// Catalogue is the language in use's translations, for the panel's scripts.
// Portuguese has none: its text is the key.
func Catalogue() map[string]string { return catalogues[Lang()] }

// Detect picks the language from the system's: Portuguese, Spanish or English
// when it is one of those, and otherwise English, which the most people read.
func Detect() string {
	return match(systemLocale())
}

func match(locale string) string {
	l := strings.ToLower(strings.ReplaceAll(locale, "_", "-"))
	switch {
	case strings.HasPrefix(l, "pt"):
		return Source
	case strings.HasPrefix(l, "es"):
		return "es"
	default:
		return "en"
	}
}

// systemLocale is the person's language setting: LC_ALL, LC_MESSAGES or
// LANG where they are set, and otherwise what the system says (osLocale). An
// app opened from the Dock or the Start menu gets none of those variables.
func systemLocale() string {
	for _, v := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if s := os.Getenv(v); s != "" && s != "C" && s != "POSIX" && !strings.HasPrefix(s, "C.") {
			return s
		}
	}
	return osLocale()
}
