package mediaread

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// zipParts builds a zip with the given parts, in the order given.
func zipParts(t *testing.T, parts ...[2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, p := range parts {
		f, err := w.Create(p[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(p[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// makePDF writes a minimal PDF with one Letter page per content stream, a
// Helvetica font, and a correct xref table.
func makePDF(contents ...string) []byte {
	n := len(contents)
	font := 3 + 2*n
	kids := ""
	for i := range contents {
		kids += fmt.Sprintf("%d 0 R ", 3+2*i)
	}
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids, n),
	}
	for i, c := range contents {
		objs = append(objs,
			fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents %d 0 R /Resources << /Font << /F1 %d 0 R >> >> >>", 4+2*i, font),
			fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(c), c))
	}
	objs = append(objs, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

func textPage(s string) string { return fmt.Sprintf("BT /F1 24 Tf 72 720 Td (%s) Tj ET", s) }

// drawingPage has a filled square and no text, like a scan.
const drawingPage = "0 0 1 rg 100 100 300 300 re f"
