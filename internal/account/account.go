// Package account says which WhatsApp number a connection is: the label the
// person gave it ("Pessoal", "Trabalho"), its phone and the profile name.
// The MCP status, the webhooks and the panel all show the same Info.
package account

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// LabelSetting is the state setting that holds a number's label.
const LabelSetting = "account_label"

// MaxLabel is the longest label, in characters.
const MaxLabel = 30

// Info is one number as the person knows it. Slug is empty for the main
// number; each further number has its own, fixed when it is added.
type Info struct {
	Slug  string `json:"slug,omitempty"`
	Label string `json:"label,omitempty"`
	Phone string `json:"phone,omitempty"`
	Name  string `json:"name,omitempty"`
}

// Title is how a client shows the connection: "WhatsApp", or "WhatsApp
// Trabalho" once the number has a label.
func (i Info) Title() string {
	if i.Label == "" {
		return "WhatsApp"
	}
	return "WhatsApp " + i.Label
}

// CleanLabel trims a label, folds its spaces and cuts it to MaxLabel
// characters.
func CleanLabel(s string) string {
	s = strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
	if utf8.RuneCountInString(s) > MaxLabel {
		s = strings.TrimSpace(string([]rune(s)[:MaxLabel]))
	}
	return s
}

// FormatPhone writes a number the way it is read in Brazil, and leaves others
// as +country digits.
func FormatPhone(p string) string {
	d := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, p)
	if d == "" {
		return ""
	}
	if strings.HasPrefix(d, "55") && (len(d) == 12 || len(d) == 13) {
		local := d[4:]
		return fmt.Sprintf("+55 (%s) %s-%s", d[2:4], local[:len(local)-4], local[len(local)-4:])
	}
	return "+" + d
}
