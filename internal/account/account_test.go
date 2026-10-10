package account

import "testing"

func TestCleanLabel(t *testing.T) {
	for in, want := range map[string]string{
		"  Trabalho ":                          "Trabalho",
		"Loja\t do   centro":                   "Loja do centro",
		"":                                     "",
		"abcdefghijklmnopqrstuvwxyz0123456789": "abcdefghijklmnopqrstuvwxyz0123",
		"Número de atendimento ao cliente":     "Número de atendimento ao clien",
	} {
		if got := CleanLabel(in); got != want {
			t.Errorf("CleanLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTitle(t *testing.T) {
	if got := (Info{}).Title(); got != "WhatsApp" {
		t.Errorf("no label: %q", got)
	}
	if got := (Info{Label: "Trabalho"}).Title(); got != "WhatsApp Trabalho" {
		t.Errorf("label: %q", got)
	}
}

func TestFormatPhone(t *testing.T) {
	for in, want := range map[string]string{
		"5511987654321": "+55 (11) 98765-4321",
		"551132654321":  "+55 (11) 3265-4321",
		"4915112345678": "+4915112345678",
		"":              "",
	} {
		if got := FormatPhone(in); got != want {
			t.Errorf("FormatPhone(%q) = %q, want %q", in, got, want)
		}
	}
}
