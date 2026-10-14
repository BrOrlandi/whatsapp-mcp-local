package localasr

import (
	"context"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/sidecar"
)

// transcribe_audio hands the AI Transcribe's errors: they stay the same
// whatever language the panel shows.
func TestTranscribeErrorsDoNotFollowTheLanguage(t *testing.T) {
	defer i18n.Set(i18n.Source)
	old := systemTools
	systemTools = func() sidecar.Tools { return sidecar.Tools{} }
	defer func() { systemTools = old }()
	e := New(t.TempDir())
	var first string
	for _, lang := range []string{i18n.Source, "es", "en"} {
		i18n.Set(lang)
		_, err := e.Transcribe(context.Background(), "audio.ogg", "", "")
		if err == nil {
			t.Fatal("transcribed without the programs")
		}
		if first == "" {
			first = err.Error()
		} else if err.Error() != first {
			t.Errorf("%s: %q, want %q", lang, err, first)
		}
	}
}
