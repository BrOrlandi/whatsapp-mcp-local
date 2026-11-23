package localasr

import (
	"errors"
	"os"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/appconfig"
)

func TestContextDefaultsFollowTheMode(t *testing.T) {
	for mode, want := range map[string]string{ModeLocal: ContextNames, ModeRemote: ContextNone, ModeOpenAI: ContextNone, "": ContextNames} {
		if got := DefaultContext(mode); got != want {
			t.Errorf("DefaultContext(%q) = %q, want %q", mode, got, want)
		}
		if got := (Settings{Mode: mode}).ContextLevel(); got != want {
			t.Errorf("unset in %q: %q, want %q", mode, got, want)
		}
	}
	if got := (Settings{Mode: ModeOpenAI, Context: ContextMessages}).ContextLevel(); got != ContextMessages {
		t.Errorf("a chosen level should win offsite: %q", got)
	}
	// Here the context is not a choice: always the names.
	for _, level := range []string{ContextNone, ContextMessages} {
		if got := (Settings{Mode: ModeLocal, Context: level}).ContextLevel(); got != ContextNames {
			t.Errorf("%q kept, in local: %q, want names", level, got)
		}
	}
	for in, want := range map[string]string{"none": ContextNone, " Names ": ContextNames, "MESSAGES": ContextMessages, "nada": ContextNone} {
		if got, ok := ParseContext(in); !ok || got != want {
			t.Errorf("ParseContext(%q) = %q, %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "auto", "all", "tudo"} {
		if _, ok := ParseContext(in); ok {
			t.Errorf("ParseContext(%q) should not read", in)
		}
	}
}

func TestContextFromConfigAndVariable(t *testing.T) {
	fakeTools(t)
	e := engine(t, false)
	// A config.json from 1.9.5, without the field: the mode's default.
	if err := os.WriteFile(appconfig.Path(e.data), []byte(`{"port":47821,"language":"","transcription":{"mode":"openai","openai_key":"sk-test"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if s := e.Settings(); s.Mode != ModeOpenAI || s.Context != "" || s.ContextLevel() != ContextNone {
		t.Errorf("1.9.5 config: %+v", s)
	}
	// A value that does not read falls to the default, never to more.
	if err := os.WriteFile(appconfig.Path(e.data), []byte(`{"port":47821,"language":"","transcription":{"mode":"openai","openai_key":"sk-test","context":"everything"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if s := e.Settings(); s.ContextLevel() != ContextNone {
		t.Errorf("unreadable context: %+v", s)
	}
	if err := e.SetContext("messages"); err != nil {
		t.Fatal(err)
	}
	if s := e.Settings(); s.Context != ContextMessages || s.ContextLevel() != ContextMessages || s.ContextFromEnv {
		t.Errorf("chosen: %+v", s)
	}
	// The variable wins, on its own, without the mode's variables.
	t.Setenv(EnvContext, "names")
	if s := e.Settings(); s.ContextLevel() != ContextNames || !s.ContextFromEnv || s.FromEnv || s.Mode != ModeOpenAI {
		t.Errorf("variable: %+v", s)
	}
	if err := e.SetContext("none"); !errors.Is(err, ErrContextFromEnv) {
		t.Errorf("SetContext under the variable: %v", err)
	}
	t.Setenv(EnvContext, "bogus")
	if s := e.Settings(); s.ContextFromEnv || s.ContextLevel() != ContextMessages {
		t.Errorf("an unreadable variable should be as if unset: %+v", s)
	}
	// In local, neither the variable nor the kept level counts: the names.
	t.Setenv(EnvContext, "messages")
	t.Setenv(EnvMode, ModeLocal)
	if s := e.Settings(); !s.FromEnv || s.Mode != ModeLocal || s.ContextLevel() != ContextNames || s.ContextFromEnv || s.Context != "" {
		t.Errorf("in local the context should be the names, whatever is set: %+v", s)
	}
	// The level kept for OpenAI is still there for when it is used again.
	if c, _ := appconfig.Load(e.data); c.Transcription == nil || c.Transcription.Context != ContextMessages {
		t.Errorf("the kept level should be left alone: %+v", c.Transcription)
	}
}

func TestSetContext(t *testing.T) {
	fakeTools(t)
	e := engine(t, false)
	// Here it is not a choice: nothing is kept, and the error says why.
	if err := e.SetContext("messages"); !errors.Is(err, ErrContextLocal) {
		t.Errorf("SetContext in local: %v", err)
	}
	if c, _ := appconfig.Load(e.data); c.Transcription != nil {
		t.Errorf("nothing should be kept in local: %+v", c.Transcription)
	}
	// A config.json from 1.9.6 with a level chosen here: ignored, kept.
	if err := os.WriteFile(appconfig.Path(e.data), []byte(`{"port":47821,"language":"","transcription":{"mode":"local","context":"messages"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if s := e.Settings(); s.ContextLevel() != ContextNames {
		t.Errorf("1.9.6 level in local: %+v", s)
	}
	if err := e.Configure(Change{Mode: ModeOpenAI, Token: "sk-test"}); err != nil {
		t.Fatal(err)
	}
	if err := e.SetContext("tudo"); err == nil {
		t.Error("an unknown level was kept")
	}
	if err := e.SetContext("messages"); err != nil {
		t.Fatal(err)
	}
	if c, _ := appconfig.Load(e.data); c.Transcription == nil || c.Transcription.Context != ContextMessages {
		t.Fatalf("kept: %+v", c.Transcription)
	}
	// The mode's own default is kept as following the mode; so is auto.
	if err := e.SetContext("none"); err != nil {
		t.Fatal(err)
	}
	if c, _ := appconfig.Load(e.data); c.Transcription == nil || c.Transcription.Context != "" {
		t.Errorf("the default should keep no level: %+v", c.Transcription)
	}
	_ = e.SetContext("names")
	if err := e.SetContext("auto"); err != nil {
		t.Fatal(err)
	}
	if s := e.Settings(); s.Context != "" || s.ContextLevel() != ContextNone {
		t.Errorf("auto: %+v", s)
	}
}

// A context kept from an earlier version, or for another place, is not
// carried to a server or OpenAI: switching there starts from nothing.
func TestSwitchingOffsiteStartsWithNoContext(t *testing.T) {
	fakeTools(t)
	e := engine(t, false)
	if err := os.WriteFile(appconfig.Path(e.data), []byte(`{"port":47821,"language":"","transcription":{"mode":"local","context":"messages"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := e.Configure(Change{Mode: ModeOpenAI, Token: "sk-test"}); err != nil {
		t.Fatal(err)
	}
	if s := e.Settings(); s.Context != "" || s.ContextLevel() != ContextNone {
		t.Errorf("after switching to OpenAI: %+v", s)
	}
	// Chosen there, it stays while the OpenAI setting is saved again.
	if err := e.SetContext("names"); err != nil {
		t.Fatal(err)
	}
	if err := e.Configure(Change{Mode: ModeOpenAI}); err != nil {
		t.Fatal(err)
	}
	if s := e.Settings(); s.ContextLevel() != ContextNames {
		t.Errorf("saving OpenAI again dropped the context: %+v", s)
	}
	if err := e.Configure(Change{Mode: ModeLocal}); err != nil {
		t.Fatal(err)
	}
	if s := e.Settings(); s.ContextLevel() != ContextNames {
		t.Errorf("back here: %+v", s)
	}
	if err := e.Configure(Change{Mode: ModeRemote, URL: "192.168.0.10:47823"}); err != nil {
		t.Fatal(err)
	}
	if s := e.Settings(); s.ContextLevel() != ContextNone {
		t.Errorf("after switching to a server: %+v", s)
	}
	// Chosen for one server, it does not follow to another address.
	if err := e.SetContext("messages"); err != nil {
		t.Fatal(err)
	}
	if err := e.Configure(Change{Mode: ModeRemote, URL: "192.168.0.10:47823"}); err != nil {
		t.Fatal(err)
	}
	if s := e.Settings(); s.ContextLevel() != ContextMessages {
		t.Errorf("saving the same server dropped the context: %+v", s)
	}
	if err := e.Configure(Change{Mode: ModeRemote, URL: "192.168.0.11:47823"}); err != nil {
		t.Fatal(err)
	}
	if s := e.Settings(); s.ContextLevel() != ContextNone {
		t.Errorf("after moving to another server: %+v", s)
	}
}
