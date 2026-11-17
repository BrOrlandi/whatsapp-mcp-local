package public

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// A consent page shows hosts in their ASCII (punycode) form, so a host
// written with look-alike letters from another alphabet shows as the
// xn-- name it really is, and the app's self-chosen name carries no
// invisible characters that reorder or hide text.

// asciiHost is a host name with each label that is not ASCII written in
// punycode (RFC 3492), lowercased; a port, when there is one, is kept.
func asciiHost(host string) string {
	port := ""
	if i := strings.LastIndexByte(host, ':'); i >= 0 && !strings.Contains(host, "]") && strings.Count(host, ":") == 1 {
		host, port = host[:i], host[i:]
	}
	labels := strings.Split(strings.ToLower(host), ".")
	for i, l := range labels {
		if !isASCII(l) {
			if enc, ok := punycode(l); ok {
				labels[i] = "xn--" + enc
			}
		}
	}
	return strings.Join(labels, ".") + port
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// punycode encodes one label (RFC 3492, section 6.3).
func punycode(label string) (string, bool) {
	const (
		base, tMin, tMax, skew, damp = 36, 1, 26, 38, 700
		initialBias, initialN        = 72, 128
	)
	digit := func(d int) byte {
		if d < 26 {
			return byte('a' + d)
		}
		return byte('0' + d - 26)
	}
	adapt := func(delta, points int, first bool) int {
		if first {
			delta /= damp
		} else {
			delta /= 2
		}
		delta += delta / points
		k := 0
		for delta > ((base-tMin)*tMax)/2 {
			delta /= base - tMin
			k += base
		}
		return k + (base-tMin+1)*delta/(delta+skew)
	}
	runes := []rune(label)
	var out []byte
	for _, r := range runes {
		if r < 0x80 {
			out = append(out, byte(r))
		}
	}
	basic := len(out)
	h := basic
	if basic > 0 {
		out = append(out, '-')
	}
	n, delta, bias := initialN, 0, initialBias
	for h < len(runes) {
		m := int(unicode.MaxRune) + 1
		for _, r := range runes {
			if int(r) >= n && int(r) < m {
				m = int(r)
			}
		}
		if (m-n)*(h+1) > 1<<30 {
			return "", false
		}
		delta += (m - n) * (h + 1)
		n = m
		for _, r := range runes {
			if int(r) < n {
				delta++
			}
			if int(r) == n {
				q := delta
				for k := base; ; k += base {
					t := k - bias
					if t < tMin {
						t = tMin
					} else if t > tMax {
						t = tMax
					}
					if q < t {
						break
					}
					out = append(out, digit(t+(q-t)%(base-t)))
					q = (q - t) / (base - t)
				}
				out = append(out, digit(q))
				bias = adapt(delta, h+1, h == basic)
				delta = 0
				h++
			}
		}
		delta++
		n++
	}
	return string(out), true
}

// visibleName drops the characters that change how text shows without
// showing themselves: controls and format characters (bidi overrides,
// zero-width spaces and joiners).
func visibleName(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) || unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
}
