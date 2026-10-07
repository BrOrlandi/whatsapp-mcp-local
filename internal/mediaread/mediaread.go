// Package mediaread turns media a chat received into something an AI model
// can read: pictures scaled to the size vision models look at, and the text of
// PDF, Word and Excel documents. It is pure Go, so it works in the command line
// built without cgo; PDFs go through PDFium compiled to WebAssembly, which runs
// in this process with no access to the filesystem.
package mediaread

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// ErrNoText means the document holds no extractable text: a scanned PDF, an
// image saved as PDF.
var ErrNoText = errors.New("the document holds no extractable text; it may be a scan or a picture saved as a document")

// ErrUnsupported means the format is not one this package reads.
var ErrUnsupported = errors.New("unsupported format")

// clip cuts s to at most n characters, never inside a UTF-8 sequence.
func clip(s string, n int) (string, bool) {
	if len(s) <= n {
		return s, false
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i], true
		}
		count++
	}
	return s, false
}

// textOut collects extracted text up to a number of characters, so a large
// document stops being read once the limit is reached.
type textOut struct {
	b         strings.Builder
	left      int
	truncated bool
	blank     bool // the last line was empty: another one is dropped
}

func newTextOut(maxChars int) *textOut { return &textOut{left: maxChars, blank: true} }

// write appends s, or the part of it that fits; false means the limit is
// reached and nothing more will be taken.
func (o *textOut) write(s string) bool {
	if o.truncated {
		return false
	}
	n := utf8.RuneCountInString(s)
	if n <= o.left {
		o.b.WriteString(s)
		o.left -= n
		return true
	}
	cut, _ := clip(s, o.left)
	o.b.WriteString(cut)
	o.left = 0
	o.truncated = true
	return false
}

// line appends s as a line, collapsing runs of empty lines into one.
func (o *textOut) line(s string) bool {
	s = strings.TrimRight(s, " \t")
	if s == "" {
		if o.blank {
			return !o.truncated
		}
		o.blank = true
	} else {
		o.blank = false
	}
	return o.write(s + "\n")
}

func (o *textOut) String() string { return strings.TrimRight(o.b.String(), "\n") }
