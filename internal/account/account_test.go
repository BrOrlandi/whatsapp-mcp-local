package account

import (
	"strings"
	"testing"
)

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

func TestMaskPhone(t *testing.T) {
	for in, want := range map[string]string{
		"5516999996892":       "+55 (16) 9••••-6892",
		"+55 (16) 99999-6892": "+55 (16) 9••••-6892",
		"551633336892":        "+55 (16) ••••-6892",
		"14155550123":         "+••••••• 0123",
		"447700900123":        "+•••••••• 0123",
		"5512345":             "+•••••••",
		"1234":                "+••••",
		"":                    "",
		"abc":                 "",
	} {
		if got := MaskPhone(in); got != want {
			t.Errorf("MaskPhone(%q) = %q, want %q", in, got, want)
		}
	}
}

// Whatever its length, a masked number never shows the whole number, hides
// at least four digits (or all of a shorter one) and keeps the last four of
// a full-length one, for the person to recognise it.
func TestMaskPhoneHidesTheMiddle(t *testing.T) {
	digits := "551699999689212345"
	for n := 1; n <= len(digits); n++ {
		d := digits[:n]
		got := MaskPhone(d)
		shown := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, got)
		if hidden := strings.Count(got, "•"); strings.Contains(shown, d) || hidden < min(n, 4) || hidden+len(shown) != n {
			t.Errorf("MaskPhone(%q) = %q shows too much", d, got)
		}
		if n >= 8 && !strings.HasSuffix(shown, d[n-4:]) {
			t.Errorf("MaskPhone(%q) = %q lost the last digits", d, got)
		}
	}
}
