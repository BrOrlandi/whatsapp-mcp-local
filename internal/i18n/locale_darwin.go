package i18n

import (
	"os/exec"
	"strings"
)

// osLocale is the first of the languages in System Settings › General ›
// Language & Region, the one the person put on top.
func osLocale() string {
	out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return ""
	}
	// (\n    "es-CL",\n    "en-US"\n)
	s := strings.Trim(strings.TrimSpace(string(out)), "()")
	s = strings.SplitN(s, ",", 2)[0]
	return strings.Trim(s, "\" \n\t")
}
