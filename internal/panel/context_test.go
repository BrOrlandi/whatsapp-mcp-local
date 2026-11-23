package panel

import (
	"strings"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/localasr"
)

// The card's line says what goes with the audio and, when it leaves the
// computer, says so as a notice.
func TestContextLine(t *testing.T) {
	for _, c := range []struct {
		s      localasr.Settings
		server bool
		level  string
		leaves bool
		says   []string
	}{
		{localasr.Settings{Mode: localasr.ModeLocal}, false, "names", false, []string{"O nome da conversa e os das pessoas", "Nada sai deste computador."}},
		{localasr.Settings{Mode: localasr.ModeLocal}, true, "names", false, []string{"Nada sai deste servidor."}},
		// Here a kept level does not count: always the names.
		{localasr.Settings{Mode: localasr.ModeLocal, Context: "messages"}, false, "names", false, []string{"O nome da conversa e os das pessoas", "Nada sai deste computador."}},
		{localasr.Settings{Mode: localasr.ModeOpenAI}, false, "none", false, []string{"Só o áudio vai para a transcrição"}},
		{localasr.Settings{Mode: localasr.ModeRemote}, false, "none", false, []string{"Só o áudio"}},
		{localasr.Settings{Mode: localasr.ModeOpenAI, Context: "names"}, false, "names", true, []string{"para a OpenAI", "Esse texto sai do computador."}},
		{localasr.Settings{Mode: localasr.ModeOpenAI, Context: "names"}, true, "names", true, []string{"para a OpenAI", "Esse texto sai do servidor."}},
		{localasr.Settings{Mode: localasr.ModeRemote, Context: "messages"}, false, "messages", true, []string{"servidor de transcrição", "últimas mensagens", "Esse texto sai do computador."}},
	} {
		v := contextOf(c.s, c.server)
		if v.Level != c.level || v.Leaves != c.leaves {
			t.Errorf("%+v: level %q leaves %v", c.s, v.Level, v.Leaves)
		}
		for _, w := range c.says {
			if !strings.Contains(v.Line, w) {
				t.Errorf("%+v: line %q lacks %q", c.s, v.Line, w)
			}
		}
		if v.Offsite != c.s.Offsite() {
			t.Errorf("%+v: offsite %v", c.s, v.Offsite)
		}
		if v.Where == "" {
			t.Errorf("%+v: no hint of where it goes", c.s)
		}
	}
}

func TestContextGroupInTheTranscriptionCard(t *testing.T) {
	render := func(asr transcriptionView) string {
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
		}{layout: layout{Title: "Configurações", Active: "configuracoes"}, ASR: asr}); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	open := transcriptionView{Mode: localasr.ModeOpenAI, Offsite: true}
	open.Ctx = contextOf(localasr.Settings{Mode: localasr.ModeOpenAI, Context: "messages"}, false)
	page := render(open)
	for _, want := range []string{
		"Contexto da conversa", `aria-describedby="tip-asr-context"`, `href="#asr-contexto-dialog"`, "Mudar o contexto",
		`id="asr-contexto-dialog"`, `action="/configuracoes/transcricao/contexto"`, `role="group" aria-labelledby="asr-contexto"`,
		`value="none"`, `value="names"`, `value="messages" data-asr-context-level checked`, "Nada", "Só os nomes", "Nomes e mensagens recentes",
		"Para que serve e o que é enviado", "pode sair traduzido", `<p class="note" style="margin:0" data-asr-context-line>`, "Esse texto sai do computador.",
		"Vai junto com o áudio para a OpenAI.", "até seis nomes",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the transcription card lacks %q", want)
		}
	}
	if strings.Contains(page, "O áudio e as mensagens ao redor dele vão para a OpenAI") {
		t.Error("the OpenAI choice still says the messages always go")
	}
	here := transcriptionView{Mode: localasr.ModeLocal}
	here.Ctx = contextOf(localasr.Settings{Mode: localasr.ModeLocal}, false)
	page = render(here)
	// Here the context is not a choice: the group is hidden (the script shows
	// it when voice notes go elsewhere).
	if !strings.Contains(page, `<div class="stack" data-asr-context hidden>`) {
		t.Error("here, the context group should be hidden")
	}
	if strings.Contains(render(open), `data-asr-context hidden`) {
		t.Error("offsite, the context group should show")
	}
	fixed := open
	fixed.Ctx = contextOf(localasr.Settings{Mode: localasr.ModeOpenAI, Context: "none", ContextFromEnv: true}, false)
	page = render(fixed)
	if strings.Contains(page, "asr-contexto-dialog") || !strings.Contains(page, "WHATSAPP_MCP_TRANSCRIPTION_CONTEXT") {
		t.Error("a context set by the variable has no dialog and says where it is set")
	}
}
