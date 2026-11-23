package brand

import "testing"

func TestChangelogURL(t *testing.T) {
	for version, want := range map[string]string{
		"1.9.6":     "https://whatsappmcp.com.br/changelog/1.9.6",
		"v1.9.6":    "https://whatsappmcp.com.br/changelog/1.9.6",
		"10.0.12":   "https://whatsappmcp.com.br/changelog/10.0.12",
		"":          "https://whatsappmcp.com.br/changelog",
		"dev":       "https://whatsappmcp.com.br/changelog",
		"1.9":       "https://whatsappmcp.com.br/changelog",
		"1.9.6-rc1": "https://whatsappmcp.com.br/changelog",
		"1.9.6/../": "https://whatsappmcp.com.br/changelog",
	} {
		if got := ChangelogURL(version); got != want {
			t.Errorf("ChangelogURL(%q) = %q, want %q", version, got, want)
		}
	}
}
