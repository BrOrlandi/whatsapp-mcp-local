package mcp

import (
	"strings"
	"testing"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/index"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

func TestPhoneDigits(t *testing.T) {
	for in, want := range map[string]string{"+55 (11) 91234-5678": "5511912345678", "5511912345678": "5511912345678",
		"Mãe": "", "123": "", "Grupo 2024": ""} {
		if got := phoneDigits(in); got != want {
			t.Errorf("phoneDigits(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMentionArgs(t *testing.T) {
	args, err := mentionArgs("oi @5511912345678 e @123456789012345", []string{"+55 11 91234-5678", "123456789012345@lid"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(args, " ") != "--mention 5511912345678 --mention 123456789012345@lid" {
		t.Errorf("args = %v", args)
	}
	if _, err := mentionArgs("oi pessoal", []string{"5511912345678"}); err == nil || !strings.Contains(err.Error(), "@5511912345678") {
		t.Errorf("a mention the text does not place should be refused, got %v", err)
	}
	if _, err := mentionArgs("oi", []string{"Maria"}); err == nil {
		t.Error("a name is not a mention")
	}
}

func TestIsClosing(t *testing.T) {
	own := []string{"5511999999999"}
	for text, want := range map[string]bool{"Obrigado!": true, "ok.": true, "👍": true, "@5511999999999 valeu": true,
		"obrigado, mas e o boleto?": false, "Pode me ligar?": false} {
		if got := isClosing(index.Row{Text: text}, own); got != want {
			t.Errorf("isClosing(%q) = %v, want %v", text, got, want)
		}
	}
	if !isClosing(index.Row{MediaType: "sticker"}, own) || isClosing(index.Row{MediaType: "image"}, own) {
		t.Error("a sticker closes, a photo does not")
	}
}

func TestShaping(t *testing.T) {
	if _, err := newShaping(arguments{Fields: []string{"bogus"}}, messageFields); err == nil {
		t.Error("an unknown field must be refused")
	}
	sh, err := newShaping(arguments{Fields: []string{"id", "text"}, MaxContentChars: 5}, messageFields)
	if err != nil {
		t.Fatal(err)
	}
	msgs := []message{{ID: "A", Text: "uma mensagem longa", ChatJID: "x"}, {ID: "B", Text: "curta"},
		{ID: "C", MediaType: "audio", Transcript: &state.Transcript{Text: "um áudio comprido", Raw: "um audio comprido"}}}
	out := sh.messages(msgs).([]map[string]any)
	if out[0]["text"] != "uma m…" || out[0]["text_truncated"] != true || out[0]["chat_jid"] != nil {
		t.Errorf("first = %v", out[0])
	}
	if _, cut := out[1]["text_truncated"]; cut || out[1]["text"] != "curta" {
		t.Errorf("second = %v", out[1])
	}
	if out[2]["text_truncated"] != true {
		t.Errorf("a cut transcript is marked: %v", out[2])
	}
	if msgs[2].Transcript.Text != "um áu…" {
		t.Errorf("transcript = %q", msgs[2].Transcript.Text)
	}
}
