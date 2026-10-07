package mediaread

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// TextOptions bounds an extraction.
type TextOptions struct {
	MaxChars int // default 200_000
	MaxPages int // PDF pages read, default 20, max 500
	MaxRows  int // rows per spreadsheet sheet, default 500
}

// Text is the readable content of a document.
type Text struct {
	Format    string // pdf, docx, xlsx or text
	Text      string
	Pages     int  // pages (PDF) or sheets (XLSX) in the file
	Read      int  // pages or sheets actually read
	Truncated bool // a limit cut the text
}

const (
	mimeDOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	mimeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

// ReadText extracts the text of a PDF, DOCX or XLSX file, or returns a plain
// text file (text/*, application/json, csv) as is, within the limits. mime may
// be empty; the extension and the file's magic bytes decide then.
//
// ErrNoText comes with Format, Pages and Read filled, so the caller can offer
// RenderPDF for a scanned PDF.
func ReadText(path, mime string, opts TextOptions) (Text, error) {
	if opts.MaxChars <= 0 {
		opts.MaxChars = 200_000
	}
	if opts.MaxPages <= 0 {
		opts.MaxPages = 20
	}
	opts.MaxPages = min(opts.MaxPages, 500)
	if opts.MaxRows <= 0 {
		opts.MaxRows = 500
	}
	format := formatOfMIME(mime)
	if format == "" {
		var err error
		if format, err = sniff(path); err != nil {
			return Text{}, err
		}
	}
	switch format {
	case "pdf":
		return pdfText(path, opts)
	case "docx":
		return docxText(path, opts)
	case "xlsx":
		return xlsxText(path, opts)
	default:
		return plainText(path, opts)
	}
}

func formatOfMIME(mime string) string {
	mime, _, _ = strings.Cut(strings.ToLower(mime), ";")
	mime = strings.TrimSpace(mime)
	switch {
	case mime == "application/pdf" || mime == "application/x-pdf":
		return "pdf"
	case mime == mimeDOCX:
		return "docx"
	case mime == mimeXLSX:
		return "xlsx"
	case strings.HasPrefix(mime, "text/"), mime == "application/json", mime == "application/xml",
		mime == "application/csv", strings.HasSuffix(mime, "+json"), strings.HasSuffix(mime, "+xml"):
		return "text"
	}
	return ""
}

var errNotDocument = fmt.Errorf("%w: only PDF, DOCX, XLSX and plain text documents are read", ErrUnsupported)

// sniff decides the format from the file's first bytes, then its extension.
func sniff(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	head := make([]byte, 8<<10)
	n, err := io.ReadFull(f, head)
	f.Close()
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", err
	}
	head = head[:n]
	switch {
	case bytes.Contains(head[:min(len(head), 1024)], []byte("%PDF-")):
		return "pdf", nil
	case bytes.HasPrefix(head, []byte("PK\x03\x04")):
		return zipFormat(path)
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return "pdf", nil
	case ".docx":
		return "docx", nil
	case ".xlsx":
		return "xlsx", nil
	}
	if looksLikeText(head) {
		return "text", nil
	}
	return "", errNotDocument
}

// looksLikeText accepts UTF-16 with a byte order mark, or bytes with no NUL
// and few control characters, which covers UTF-8 and the Windows-1252 that
// spreadsheets exported in Brazil often are.
func looksLikeText(b []byte) bool {
	if bytes.HasPrefix(b, []byte{0xFF, 0xFE}) || bytes.HasPrefix(b, []byte{0xFE, 0xFF}) {
		return true
	}
	control := 0
	for _, c := range b {
		switch {
		case c == 0:
			return false
		case c < 0x20 && c != '\t' && c != '\n' && c != '\r' && c != '\f':
			control++
		}
	}
	return control*100 <= len(b)
}

func plainText(path string, opts TextOptions) (Text, error) {
	f, err := os.Open(path)
	if err != nil {
		return Text{}, err
	}
	defer f.Close()
	// Four bytes cover any character, in UTF-8 or UTF-16.
	limit := int64(opts.MaxChars)*4 + 4
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return Text{}, err
	}
	more := int64(len(data)) > limit
	if more {
		data = data[:limit]
	}
	s, cut := clip(decodeText(data), opts.MaxChars)
	return Text{Format: "text", Text: s, Truncated: cut || more}, nil
}

// decodeText returns b as valid UTF-8: UTF-16 by its byte order mark,
// Windows-1252 when nothing in it is UTF-8 beyond ASCII but some bytes are
// not, and otherwise UTF-8 with invalid bytes replaced.
func decodeText(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		b = b[3:]
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		return utf16Text(b[2:], binary.LittleEndian)
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		return utf16Text(b[2:], binary.BigEndian)
	}
	if utf8.Valid(b) {
		return string(b)
	}
	if !hasMultibyteUTF8(b) {
		if s, err := charmap.Windows1252.NewDecoder().Bytes(b); err == nil {
			return string(s)
		}
	}
	return strings.ToValidUTF8(string(b), "�")
}

func hasMultibyteUTF8(b []byte) bool {
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		if size > 1 && r != utf8.RuneError {
			return true
		}
		b = b[size:]
	}
	return false
}

func utf16Text(b []byte, bo binary.ByteOrder) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = bo.Uint16(b[2*i:])
	}
	return string(utf16.Decode(u))
}
