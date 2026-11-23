package mcp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/localasr"
)

// On a Windows where the engine cannot start, transcribe_audio says local
// transcription is not available here (not "not installed", which would send
// the user to install it again) and gives steps the AI can act on.
func TestTranscribeAudioWhenTheEngineIsUnavailable(t *testing.T) {
	out := noEngineAnswer(localasr.Status{Supported: true, Unavailable: localasr.UnavailableWindowsRuntime}, "http://127.0.0.1:47821")
	body, _ := json.Marshal(out)
	text := string(body)
	for _, want := range []string{"not available on this computer", "Windows", "download_media", "Em um servidor compatível com a OpenAI", "Na OpenAI", "/configuracoes#transcricao", "does not help"} {
		if !strings.Contains(text, want) {
			t.Errorf("the answer lacks %q: %s", want, text)
		}
	}
	if out["unavailable"] != localasr.UnavailableWindowsRuntime || strings.Contains(text, "click the install button") {
		t.Errorf("%s", text)
	}
	// Not installed is still what it was: the install button.
	text = func() string {
		b, _ := json.Marshal(noEngineAnswer(localasr.Status{Supported: true, Missing: []string{"modelo"}}, "http://x"))
		return string(b)
	}()
	if !strings.Contains(text, "transcription is not installed") || !strings.Contains(text, "install button") || strings.Contains(text, "unavailable") {
		t.Errorf("not installed: %s", text)
	}
}

func TestStatusSaysTheEngineIsUnavailable(t *testing.T) {
	local := localTranscription(localasr.Status{Supported: true, Unavailable: localasr.UnavailableWindowsRuntime})
	if local["ready"] != false || local["unavailable"] != localasr.UnavailableWindowsRuntime || !strings.Contains(local["note"].(string), "not available on this computer") {
		t.Errorf("%v", local)
	}
	if local := localTranscription(localasr.Status{Supported: true, Ready: true}); local["unavailable"] != nil || local["note"] != nil {
		t.Errorf("ready: %v", local)
	}
}

// With a remote server down and the engine unable to run here, health says
// "not available", not "not installed".
func TestTranscriptionHealthWhenTheEngineIsUnavailable(t *testing.T) {
	in := healthy(time.Now())
	in.Transcription = &localasr.RemoteStatus{Configured: true, Mode: "remote", URL: "http://mac:47823", Error: "did not answer"}
	in.LocalUnavailable = true
	c := check(t, evaluate(in), "transcription")
	if c.Status != "warn" || !strings.Contains(c.Detail, "not available on this computer") || strings.Contains(c.Detail, "not installed") || !strings.Contains(c.Fix, "download_media") {
		t.Errorf("%+v", c)
	}
}
