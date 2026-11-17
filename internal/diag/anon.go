package diag

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// Anonymizer turns what identifies a person into short codes.
type Anonymizer struct {
	key  []byte
	home string
}

// NewAnonymizer anonymises with key; home, the person's home folder, is
// written as ~ in paths.
func NewAnonymizer(key []byte, home string) *Anonymizer {
	return &Anonymizer{key: key, home: strings.TrimRight(home, `/\`)}
}

var (
	// A WhatsApp address: user (and device) at a server.
	jidRe = regexp.MustCompile(`([0-9][0-9-]{3,})(?:\.[0-9]+)?(:[0-9]+)?@(s\.whatsapp\.net|c\.us|lid|g\.us|newsletter|broadcast|hosted|hosted\.lid|bot)\b`)
	// Anything that looks like a phone number.
	phoneRe = regexp.MustCompile(`\+?\b[0-9]{10,15}\b`)
	// A formatted phone number, +55 11 91234-5678 and the like.
	fmtPhoneRe = regexp.MustCompile(`\+[0-9]{1,3}[ .-]?\(?[0-9]{2,3}\)?[ .-]?[0-9]{4,5}[ .-]?[0-9]{4}`)
	emailRe    = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	urlRe      = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s"'<>]+`)
	// WhatsApp message ids: long upper-case hex, or the 3EB0… style.
	msgIDRe = regexp.MustCompile(`\b[0-9A-F]{16,64}\b`)
	// A user name after a home folder that is not this one's (another
	// account's path, a path from a crash).
	homeRe = regexp.MustCompile(`(/Users/|/home/|\\Users\\)[^/\\\s"']+`)
)

// textKeys hold what people wrote or are called: they are never written.
var textKeys = map[string]bool{
	"text": true, "body": true, "caption": true, "content": true, "conversation": true,
	"name": true, "push_name": true, "pushname": true, "full_name": true, "first_name": true,
	"business_name": true, "display_name": true, "verified_name": true, "notify": true,
	"title": true, "subject": true, "topic": true, "description": true, "about": true, "status_text": true,
	"query": true, "search": true, "filename": true, "file_name": true, "vcard": true,
	"quoted_text": true, "preview": true, "snippet": true, "label": true, "address": true,
	"latitude": true, "longitude": true, "lat": true, "lng": true, "options": true, "poll": true,
	"qr": true, "secret": true, "token": true, "webhook_secret": true, "password": true,
	"pairing_code": true, "transcript": true, "emoji": true, "reaction": true,
}

// idKeys hold message ids.
var idKeys = map[string]bool{"msg_id": true, "message_id": true, "id": true, "ids": true, "message_ids": true,
	"stanza_id": true, "quoted_id": true, "reply_to": true, "target_id": true}

// Value anonymises a field's value, by what its key says it holds.
func (a *Anonymizer) Value(key string, v any) any {
	k := strings.ToLower(key)
	if textKeys[k] {
		if v == nil || v == "" {
			return v
		}
		return "<omitted>"
	}
	switch x := v.(type) {
	case string:
		if idKeys[k] && x != "" && !strings.Contains(x, "@") {
			return code(a.key, "msg", x)
		}
		return a.Text(x)
	case []string:
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = a.Value(key, s)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = a.Value(key, e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for kk, vv := range x {
			out[kk] = a.Value(kk, vv)
		}
		return out
	case Fields:
		return a.Value(key, map[string]any(x))
	case error:
		if x == nil {
			return nil
		}
		return a.Text(x.Error())
	case fmt.Stringer:
		return a.Text(x.String())
	default:
		return v
	}
}

// Text anonymises free text: WhatsApp addresses, phone numbers, e-mails,
// web addresses (but the computer's own), message ids and the home folder.
// Long text is cut.
func (a *Anonymizer) Text(s string) string {
	if a == nil || s == "" {
		return s
	}
	if a.home != "" && len(a.home) > 1 {
		s = strings.ReplaceAll(s, a.home, "~")
	}
	s = homeRe.ReplaceAllString(s, "${1}<user>")
	s = urlRe.ReplaceAllStringFunc(s, a.url)
	s = jidRe.ReplaceAllStringFunc(s, a.jid)
	s = emailRe.ReplaceAllStringFunc(s, func(m string) string { return code(a.key, "email", strings.ToLower(m)) })
	s = fmtPhoneRe.ReplaceAllStringFunc(s, a.phone)
	s = phoneRe.ReplaceAllStringFunc(s, a.phone)
	s = msgIDRe.ReplaceAllStringFunc(s, func(m string) string { return code(a.key, "msg", m) })
	if len(s) > 600 {
		s = s[:600] + "…"
	}
	return s
}

func (a *Anonymizer) phone(m string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, m)
	return code(a.key, "num", digits)
}

func (a *Anonymizer) jid(m string) string {
	sub := jidRe.FindStringSubmatch(m)
	if sub == nil {
		return m
	}
	user, device, server := sub[1], sub[2], sub[3]
	kind := map[string]string{
		"s.whatsapp.net": "user", "c.us": "user", "lid": "lid", "g.us": "group",
		"newsletter": "channel", "broadcast": "list", "hosted": "user", "hosted.lid": "lid", "bot": "bot",
	}[server]
	if kind == "" {
		kind = "jid"
	}
	// The same person in its phone and lid forms gets two codes; the kind
	// keeps them apart.
	return code(a.key, kind, user) + device
}

// url keeps the computer's own addresses (127.0.0.1, localhost), which say
// which port or path was used, and codes every other.
func (a *Anonymizer) url(m string) string {
	u, err := url.Parse(m)
	if err != nil {
		return code(a.key, "url", m)
	}
	host := u.Hostname()
	if host == "localhost" {
		return u.Scheme + "://" + u.Host + u.Path
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return u.Scheme + "://" + u.Host + u.Path
	}
	return u.Scheme + "://" + code(a.key, "url", host+u.Path)
}

// KindOf is the kind a code was made for (user, lid, group, msg, num...), or
// "" when s is not a code.
func KindOf(s string) string {
	kind, hash, ok := strings.Cut(strings.TrimSpace(s), "#")
	if !ok || kind == "" || len(hash) < 8 {
		return ""
	}
	return kind
}

// Is says whether c is the code of value for kind, on this computer. It is
// how a code from an exported file is matched back to what it stands for,
// which only works here, with this computer's key.
func (a *Anonymizer) Is(c, kind, value string) bool {
	if a == nil {
		return false
	}
	c = strings.TrimSpace(c)
	if i := strings.IndexByte(c, ':'); i > 0 { // a device suffix
		c = c[:i]
	}
	return code(a.key, kind, value) == c
}

// JIDCode splits a WhatsApp address into the kind and the part its code is
// made of, as the anonymiser does.
func JIDCode(jid string) (kind, value string, ok bool) {
	sub := jidRe.FindStringSubmatch(jid)
	if sub == nil {
		return "", "", false
	}
	kind = map[string]string{
		"s.whatsapp.net": "user", "c.us": "user", "lid": "lid", "g.us": "group",
		"newsletter": "channel", "broadcast": "list", "hosted": "user", "hosted.lid": "lid", "bot": "bot",
	}[sub[3]]
	if kind == "" {
		kind = "jid"
	}
	return kind, sub[1], true
}
