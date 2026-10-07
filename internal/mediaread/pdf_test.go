package mediaread

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"strings"
	"testing"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
)

func TestReadTextPDF(t *testing.T) {
	start := time.Now()
	got, err := ReadText(writeTemp(t, "one.pdf", makePDF(textPage("Hello PDF world"))), "application/pdf", TextOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("first PDF read, PDFium compile included: %s", time.Since(start))
	if got.Format != "pdf" || got.Text != "Hello PDF world" || got.Pages != 1 || got.Read != 1 || got.Truncated {
		t.Fatalf("got %+v", got)
	}
}

func TestReadTextPDFPages(t *testing.T) {
	path := writeTemp(t, "doc", makePDF(textPage("first"), drawingPage, textPage("third")))
	got, err := ReadText(path, "", TextOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := "--- page 1 ---\nfirst\n\n--- page 2 ---\n(no text on this page)\n\n--- page 3 ---\nthird"
	if got.Text != want || got.Pages != 3 || got.Read != 3 || got.Truncated {
		t.Fatalf("got %+v\nwant text %q", got, want)
	}

	got, err = ReadText(path, "", TextOptions{MaxPages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "--- page 1 ---\nfirst" || got.Pages != 3 || got.Read != 1 || !got.Truncated {
		t.Fatalf("MaxPages 1: got %+v", got)
	}
}

func TestReadTextPDFNoText(t *testing.T) {
	got, err := ReadText(writeTemp(t, "scan.pdf", makePDF(drawingPage)), "", TextOptions{})
	if !errors.Is(err, ErrNoText) {
		t.Fatalf("err = %v, want ErrNoText", err)
	}
	if got.Format != "pdf" || got.Pages != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestReadTextPDFInvalid(t *testing.T) {
	_, err := ReadText(writeTemp(t, "bad.pdf", []byte("%PDF-1.4 nothing else")), "application/pdf", TextOptions{})
	if err == nil {
		t.Fatal("no error for a broken PDF")
	}
}

// A document that keeps PDFium busy past the timeout returns an error, the
// next one waits for it no longer than the timeout either, and the reader
// works again once the first one is done.
func TestPDFTimeout(t *testing.T) {
	old := pdfTimeout
	pdfTimeout = 100 * time.Millisecond
	defer func() { pdfTimeout = old }()
	path := writeTemp(t, "one.pdf", makePDF(textPage("hi")))
	stuck := func(ctx context.Context, p pdfium.Pdfium, doc references.FPDF_DOCUMENT, pages int) (int, error) {
		time.Sleep(400 * time.Millisecond) // a call that ignores ctx, like PDFium would
		return pages, nil
	}
	if _, err := withPDF(path, stuck); err == nil || !strings.Contains(err.Error(), "took more than") {
		t.Fatalf("stuck document: err = %v", err)
	}
	if _, err := ReadText(path, "", TextOptions{}); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("while stuck: err = %v", err)
	}
	time.Sleep(400 * time.Millisecond)
	pdfTimeout = old
	if got, err := ReadText(path, "", TextOptions{}); err != nil || got.Text != "hi" {
		t.Fatalf("after: %q, %v", got.Text, err)
	}
}

func TestRenderPDF(t *testing.T) {
	path := writeTemp(t, "scan.pdf", makePDF(drawingPage, textPage("two")))
	start := time.Now()
	pages, total, err := RenderPDF(path, 1, 0, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("rendered %d pages in %s", len(pages), time.Since(start))
	if total != 2 || len(pages) != 2 || pages[0].Number != 1 || pages[1].Number != 2 {
		t.Fatalf("total %d, %d pages", total, len(pages))
	}
	img := pages[0].Image
	// A Letter page is 612x792 points: the long edge is the height.
	if img.MIME != "image/jpeg" || img.Height != 800 || img.Width < 617 || img.Width > 619 || img.OriginalWidth != 612 || img.OriginalHeight != 792 {
		t.Fatalf("got %s %dx%d from %dx%d", img.MIME, img.Width, img.Height, img.OriginalWidth, img.OriginalHeight)
	}
	m, err := jpeg.Decode(bytes.NewReader(img.Data))
	if err != nil {
		t.Fatal(err)
	}
	if b := m.Bounds(); b.Dx() != img.Width || b.Dy() != img.Height {
		t.Fatalf("JPEG is %dx%d", b.Dx(), b.Dy())
	}
	// The square is blue on a white page.
	if r, g, b, _ := m.At(10, 10).RGBA(); r < 0xf000 || g < 0xf000 || b < 0xf000 {
		t.Fatalf("corner is not white: %x %x %x", r, g, b)
	}
	if r, _, b, _ := m.At(250, 500).RGBA(); b < 0xc000 || r > 0x4000 {
		t.Fatalf("square is not blue: r=%x b=%x", r, b)
	}

	pages, _, err = RenderPDF(path, 2, 5, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].Number != 2 || pages[0].Image.Height != DefaultMaxEdge {
		t.Fatalf("from page 2: %d pages", len(pages))
	}

	if _, total, err := RenderPDF(path, 3, 1, 0); err == nil || total != 2 || !strings.Contains(err.Error(), "2 pages") {
		t.Fatalf("past the end: total %d, err %v", total, err)
	}
}
