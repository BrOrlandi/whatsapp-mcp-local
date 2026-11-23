package panel

import (
	"strings"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/localasr"
)

func settingsWith(t *testing.T, local localasr.Status) string {
	t.Helper()
	var b strings.Builder
	if err := parseTemplates().ExecuteTemplate(&b, "configuracoes", struct {
		layout
		Setup       setup
		Port        int
		PortChanged bool
		Local       localasr.Status
		ASR         transcriptionView
		Total       int
		Corrected   int
	}{layout: layout{Title: "Configurações", Active: "configuracoes", App: true}, Local: local, ASR: transcriptionView{Mode: localasr.ModeLocal}}); err != nil {
		t.Fatal(err)
	}
	page := b.String()
	start := strings.Index(page, `id="transcricao"`)
	end := strings.Index(page, `data-asr-submit`)
	if start < 0 || end < start {
		t.Fatal("no transcription card")
	}
	return page[start:end]
}

// visible says the element with attr is on the page and not hidden.
func visible(card, attr string) bool {
	i := strings.Index(card, attr)
	if i < 0 {
		return false
	}
	tag := card[i : i+strings.Index(card[i:], ">")]
	return !strings.Contains(tag, " hidden")
}

// A Windows where the engine cannot start: the card says Indisponível, says
// in plain words what to do instead, and no longer offers to install.
func TestTranscriptionCardWhenTheEngineIsUnavailable(t *testing.T) {
	card := settingsWith(t, localasr.Status{Supported: true, Accel: "cpu", Missing: []string{"modelo"}, Unavailable: localasr.UnavailableWindowsRuntime})
	for attr, want := range map[string]bool{
		`data-asr-pill="unavailable"`: true,
		`data-asr-pill="on"`:          false,
		`data-asr-pill="missing"`:     false,
		`data-asr-unavailable-note`:   true,
		`data-asr-missing`:            false,
		`data-asr-cpu`:                false,
		`data-asr-howto`:              false,
		`data-asr-install-error`:      false,
	} {
		if visible(card, attr) != want {
			t.Errorf("%s visible: %v, want %v", attr, !want, want)
		}
	}
	for _, want := range []string{"data-asr-unavailable", "ainda não funciona no Windows", "Visual C++", "Os áudios ainda não podem ser transcritos.", "Ainda não funciona no Windows.", `data-asr-local-hint="ok" hidden`, `btn--ghost btn--small" href="#asr-dialog"`} {
		if !strings.Contains(card, want) {
			t.Errorf("the card lacks %q", want)
		}
	}
	if strings.Contains(card, "whisper.cpp") {
		t.Error("the card names whisper.cpp")
	}
}

// A fresh Windows, before anything is installed, keeps the install button:
// it is how that computer learns whether it can run the engine.
func TestTranscriptionCardBeforeInstalling(t *testing.T) {
	card := settingsWith(t, localasr.Status{Supported: true, Accel: "cpu", Missing: []string{"whisper.cpp", "ffmpeg", "modelo"}})
	if !visible(card, `data-asr-missing`) || !visible(card, `data-asr-pill="missing"`) || visible(card, `data-asr-unavailable-note`) || visible(card, `data-asr-pill="unavailable"`) {
		t.Errorf("before installing: %s", card)
	}
	if strings.Contains(card, "data-asr-unavailable ") || visible(card, `data-asr-local-hint="unavailable"`) || !visible(card, `data-asr-local-hint="ok"`) {
		t.Error("says unavailable before any test")
	}
}
