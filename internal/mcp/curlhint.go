package mcp

import "strings"

// curlHint is the command a result suggests for saving a link. Agents run
// it as given, and the file name comes from whoever sent the message, so
// the name keeps only letters, digits, dot, dash and underscore (never a
// leading dash or dot) and every word is quoted for a POSIX shell. A word
// made only of characters no shell treats specially goes unquoted, so the
// same line works in cmd.exe and PowerShell too.
func curlHint(name, url string) string {
	return "curl -sSf -o " + shellWord(safeFileName(name)) + " " + shellWord(url)
}

// safeFileName keeps a file name to [A-Za-z0-9._-], without a leading dash
// or dot, and of a sane length.
func safeFileName(name string) string {
	clean := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, name)
	clean = strings.TrimLeft(clean, "-.")
	if len(clean) > 120 {
		clean = clean[len(clean)-120:]
		clean = strings.TrimLeft(clean, "-.")
	}
	if clean == "" {
		return "download"
	}
	return clean
}

// shellWord quotes s for a POSIX shell, unless every character is one no
// shell (sh, cmd.exe, PowerShell) treats specially.
func shellWord(s string) string {
	plain := s != ""
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-/:", r)) {
			plain = false
			break
		}
	}
	if plain {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
