package localasr

import "testing"

func TestCleanTranscript(t *testing.T) {
	in := " Oi, tudo bem?\n[BLANK_AUDIO]\n\n Então, sobre amanhã... \n"
	if got, want := cleanTranscript(in), "Oi, tudo bem? Então, sobre amanhã..."; got != want {
		t.Fatalf("cleanTranscript = %q, want %q", got, want)
	}
}
