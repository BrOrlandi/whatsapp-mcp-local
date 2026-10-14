package index

import (
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
)

// The name of a chat without one reaches the tools and the webhooks the same
// in every language; only the panel shows it in its own (ShownName).
func TestFallbackNamesDoNotFollowTheLanguage(t *testing.T) {
	defer i18n.Set(i18n.Source)
	for _, lang := range []string{i18n.Source, "es", "en"} {
		i18n.Set(lang)
		for jid, want := range map[string]string{"120363@g.us": "Grupo sem nome", "120363@newsletter": "Canal do WhatsApp", "status@broadcast": "Lista de transmissão"} {
			if got := formatJID(jid); got != want {
				t.Errorf("%s: formatJID(%s) = %q, want %q", lang, jid, got, want)
			}
		}
	}
	i18n.Set("en")
	if got := ShownName("Grupo sem nome"); got != "Unnamed group" {
		t.Errorf("the panel in English shows %q", got)
	}
	if got := ShownName("Maria Silva"); got != "Maria Silva" {
		t.Errorf("a chat's own name changed to %q", got)
	}
}
