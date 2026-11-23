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
		t.Errorf("a chosen level should win: %q", got)
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
	// It also applies when the variables set the mode.
	t.Setenv(EnvContext, "")
	t.Setenv(EnvMode, ModeLocal)
	if s := e.Settings(); !s.FromEnv || s.Mode != ModeLocal || s.ContextLevel() != ContextMessages {
		t.Errorf("mode from the variable keeps the kept context: %+v", s)
	}
}

func TestSetContext(t *testing.T) {
	fakeTools(t)
	e := engine(t, false)
	if err := e.SetContext("tudo"); err == nil {
		t.Error("an unknown level was kept")
	}
	// Here, choosing messages is kept even in local, where the config
	// would otherwise be dropped.
	if err := e.SetContext("messages"); err != nil {
		t.Fatal(err)
	}
	c, _ := appconfig.Load(e.data)
	if c.Transcription == nil || c.Transcription.Context != ContextMessages {
		t.Fatalf("kept: %+v", c.Transcription)
	}
	// The mode's own default is kept as following the mode, so names chosen
	// here do not reach OpenAI later; so is auto.
	if err := e.SetContext("names"); err != nil {
		t.Fatal(err)
	}
	if c, _ := appconfig.Load(e.data); c.Transcription != nil {
		t.Errorf("the default should leave nothing kept: %+v", c.Transcription)
	}
	_ = e.SetContext("none")
	if err := e.SetContext("auto"); err != nil {
		t.Fatal(err)
	}
	if s := e.Settings(); s.Context != "" || s.ContextLevel() != ContextNames {
		t.Errorf("auto: %+v", s)
	}
}

// A context chosen for this computer is not carried to a server or OpenAI;
// switching between local and back keeps it.
func TestSwitchingOffsiteStartsWithNoContext(t *testing.T) {
	fakeTools(t)
	e := engine(t, false)
	if err := e.SetContext("messages"); err != nil {
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
	if s := e.Settings(); s.ContextLevel() != ContextNames || s.Context != ContextNames {
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
