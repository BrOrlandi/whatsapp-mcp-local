package index

import (
	"strings"
	"testing"
	"time"
)

func TestAudioContextPrompt(t *testing.T) {
	c := AudioContext{ChatName: "Felipe Cardoso - Lancer", Speaker: "Felipe Cardoso - Lancer", People: []string{"Felipe Cardoso - Lancer"},
		Before: []ContextLine{{At: time.Now(), Sender: "Felipe", Text: "Vai ter um da outliers no Capuava"}, {Sender: "Eu", Text: "Me manda o do velocitta"}}}
	p := c.Prompt()
	for _, want := range []string{"Felipe Cardoso - Lancer", "Capuava", "velocitta"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt %q lacks %q", p, want)
		}
	}
	if !strings.HasPrefix(p, "Felipe Cardoso - Lancer") {
		t.Errorf("names should lead the prompt: %q", p)
	}
	long := AudioContext{ChatName: "x"}
	for i := 0; i < 100; i++ {
		long.Before = append(long.Before, ContextLine{Text: strings.Repeat("palavra ", 10)})
	}
	if n := len(long.Prompt()); n > 800 {
		t.Errorf("prompt is %d characters; Whisper reads only a few hundred", n)
	}
}

func TestIsIdentifier(t *testing.T) {
	for in, want := range map[string]bool{"5511987654321@s.whatsapp.net": true, "+55 (11) 98765-4321": true, "": true, "Mãe": false, "Time 2": false} {
		if got := isJIDLike(in); got != want {
			t.Errorf("isJIDLike(%q) = %v, want %v", in, got, want)
		}
	}
}

// The names-only prompt carries the names and never a word of a message,
// and a name written as a sentence is cut to a few words.
func TestAudioContextNamesPrompt(t *testing.T) {
	c := AudioContext{ChatName: "Felipe Cardoso - Lancer", Speaker: "Felipe Cardoso - Lancer", People: []string{"Felipe Cardoso - Lancer", "Maria", "5511987654321@s.whatsapp.net"},
		Before: []ContextLine{{Sender: "Felipe", Text: "Vai ter um da outliers no Capuava"}, {Sender: "Eu", Text: "Me manda o do velocitta"}},
		After:  []ContextLine{{Sender: "Maria", Text: "Chego às quinze"}}}
	p := c.NamesPrompt()
	if p != "Felipe Cardoso - Lancer, Maria." {
		t.Errorf("names prompt = %q", p)
	}
	for _, l := range append(c.Before, c.After...) {
		for _, w := range strings.Fields(l.Text) {
			if len(w) > 3 && strings.Contains(p, w) {
				t.Errorf("names prompt %q carries %q from a message", p, w)
			}
		}
	}
	long := AudioContext{ChatName: "Churrasco sábado na casa do João depois do jogo!", People: []string{"Até amanhã!", "+55 11 98765-4321"}}
	for i := 0; i < 20; i++ {
		long.People = append(long.People, strings.Repeat("Nome", 20)+string(rune('A'+i)))
	}
	p = long.NamesPrompt()
	if !strings.HasPrefix(p, "Churrasco sábado na casa, Até amanhã, ") || strings.Contains(p, "João") || strings.Contains(p, "98765") || strings.Contains(p, "!") {
		t.Errorf("names prompt = %q", p)
	}
	if n := len(p); n > 200 {
		t.Errorf("names prompt is %d bytes", n)
	}
	if n := strings.Count(p, ", ") + 1; n > 6 {
		t.Errorf("names prompt has %d names", n)
	}
	if got := (AudioContext{}).NamesPrompt(); got != "" {
		t.Errorf("no names: %q", got)
	}
}
