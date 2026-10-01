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
