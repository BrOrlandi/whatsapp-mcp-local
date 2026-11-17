package mcp

import (
	"fmt"
	"regexp"
	"strings"
)

// Every value a caller gives that becomes a wacli argument is checked for
// its shape before it gets there. wacli honours flags anywhere on the line,
// after positional arguments too (checked with the pinned 0.26.1:
// `contacts check 5511… --help` prints the help), so a number such as
// --store=/elsewhere would be a flag. Positional values also go after "--",
// which wacli takes as the end of its flags. A value given after its own
// flag (--to <value>) is read as that value whatever it looks like, and is
// checked for its shape all the same.

// jidPattern is a WhatsApp address: a user part with an optional device,
// and a server named by its shape (s.whatsapp.net, g.us, lid, broadcast,
// newsletter, and any other form WhatsApp adds), so no address wacli hands
// out is refused.
var jidPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._\-]*(:[0-9]+)?@[a-z][a-z0-9.\-]*$`)

// validJID reports whether v is a WhatsApp address.
func validJID(v string) bool { return len(v) <= 128 && jidPattern.MatchString(v) }

// validTarget is a recipient or a conversation: a JID, or a phone number
// with its country code, in any common formatting.
func validTarget(name, v string) error {
	v = strings.TrimSpace(v)
	if validJID(v) || phoneDigits(v) != "" && !strings.HasPrefix(v, "-") {
		return nil
	}
	return fmt.Errorf("%s must be a JID or a phone number with country code, got %q", name, truncate(v, 60))
}

// checkTargets checks every recipient and conversation of a call that has
// one, before anything runs.
func checkTargets(name string, values ...string) map[string]any {
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			continue
		}
		if err := validTarget(name, v); err != nil {
			return toolError("%v", err)
		}
	}
	return nil
}
