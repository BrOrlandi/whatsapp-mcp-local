package mediaread

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/enums"
	pdfiumerrors "github.com/klippa-app/go-pdfium/errors"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"golang.org/x/image/draw"
)

// pdfTimeout bounds one document, and the wait for the one before it. A call
// into PDFium cannot be cut short (wazero can, but checking for it makes
// rendering five times slower), so a document past it is left to finish on its
// own and the caller gets an error instead of waiting.
var pdfTimeout = 2 * time.Minute

const (
	// pdfMemoryPages caps PDFium's memory at 1 GiB (64 KiB pages).
	pdfMemoryPages = 16384
	maxRenderEdge  = 4096
)

// engine is PDFium compiled to WebAssembly. Compiling it takes about a second
// and a half, so it happens on the first PDF and is kept for the life of the
// process; each document then gets a fresh instance, under a millisecond to
// make, whose memory goes away with it.
var engine struct {
	once sync.Once
	pool pdfium.Pool
	err  error
}

// pdfSlot lets one document in at a time: PDFium is not thread-safe, and
// documents rendered side by side would only add up their memory.
var pdfSlot = make(chan struct{}, 1)

func pdfPool() (pdfium.Pool, error) {
	engine.once.Do(func() {
		engine.pool, engine.err = webassembly.Init(webassembly.Config{
			MinIdle:  0,
			MaxIdle:  1,
			MaxTotal: 1,
			// No filesystem: documents are handed over as readers.
			FSConfig: wazero.NewFSConfig(),
			RuntimeConfig: wazero.NewRuntimeConfig().
				WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling).
				WithMemoryLimitPages(pdfMemoryPages),
			// The MCP server may speak over stdout; PDFium must not write there.
			Stdout: io.Discard,
			Stderr: io.Discard,
		})
	})
	return engine.pool, engine.err
}

type pdfFunc[T any] func(ctx context.Context, p pdfium.Pdfium, doc references.FPDF_DOCUMENT, pages int) (T, error)

// withPDF opens a PDF in a fresh PDFium instance and runs fn on it, within
// pdfTimeout; fn should stop between pages once ctx is done.
func withPDF[T any](path string, fn pdfFunc[T]) (T, error) {
	var zero T
	pool, err := pdfPool()
	if err != nil {
		return zero, fmt.Errorf("starting the PDF reader: %w", err)
	}
	select {
	case pdfSlot <- struct{}{}:
	case <-time.After(pdfTimeout):
		return zero, errors.New("the PDF reader is still busy with another document; try again later")
	}
	ctx, cancel := context.WithTimeout(context.Background(), pdfTimeout)
	defer cancel()
	type result struct {
		v   T
		err error
	}
	done := make(chan result, 1)
	go func() {
		defer func() { <-pdfSlot }()
		v, err := openPDF(ctx, pool, path, fn)
		done <- result{v, err}
	}()
	select {
	case r := <-done:
		return r.v, r.err
	case <-ctx.Done():
		return zero, fmt.Errorf("the PDF took more than %s to read", pdfTimeout)
	}
}

func openPDF[T any](ctx context.Context, pool pdfium.Pool, path string, fn pdfFunc[T]) (T, error) {
	var zero T
	f, err := os.Open(path)
	if err != nil {
		return zero, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return zero, err
	}
	inst, err := pool.GetInstance(time.Minute)
	if err != nil {
		return zero, fmt.Errorf("starting the PDF reader: %w", err)
	}
	defer inst.Close()
	doc, err := inst.OpenDocument(&requests.OpenDocument{FileReader: f, FileReaderSize: st.Size()})
	if err != nil {
		return zero, pdfError(err)
	}
	count, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return zero, pdfError(err)
	}
	return fn(ctx, inst, doc.Document, count.PageCount)
}

func pdfError(err error) error {
	switch {
	case errors.Is(err, pdfiumerrors.ErrPassword):
		return errors.New("the PDF is protected by a password")
	case errors.Is(err, pdfiumerrors.ErrSecurity):
		return errors.New("the PDF uses an encryption this cannot read")
	case errors.Is(err, pdfiumerrors.ErrFormat), errors.Is(err, pdfiumerrors.ErrFile):
		return errors.New("the file is not a valid PDF")
	}
	return fmt.Errorf("reading the PDF: %w", err)
}

func pageRef(doc references.FPDF_DOCUMENT, i int) requests.Page {
	return requests.Page{ByIndex: &requests.PageByIndex{Document: doc, Index: i}}
}

func pdfText(path string, opts TextOptions) (Text, error) {
	type extract struct {
		res   Text
		found bool
	}
	x, err := withPDF(path, func(ctx context.Context, p pdfium.Pdfium, doc references.FPDF_DOCUMENT, pages int) (extract, error) {
		x := extract{res: Text{Format: "pdf", Pages: pages}}
		out := newTextOut(opts.MaxChars)
		for i := 0; i < pages && i < opts.MaxPages && ctx.Err() == nil; i++ {
			text := "(this page could not be read)"
			if t, err := p.GetPageText(&requests.GetPageText{Page: pageRef(doc, i)}); err == nil {
				text = cleanPDFText(t.Text)
			}
			x.res.Read++
			if text == "" {
				text = "(no text on this page)"
			} else {
				x.found = true
			}
			if pages > 1 && !out.line(fmt.Sprintf("--- page %d ---", i+1)) {
				break
			}
			if !out.line(text) || !out.line("") {
				break
			}
		}
		x.res.Text = out.String()
		x.res.Truncated = out.truncated || x.res.Read < pages
		return x, nil
	})
	if err != nil {
		return Text{}, err
	}
	if !x.found {
		return Text{Format: "pdf", Pages: x.res.Pages, Read: x.res.Read}, ErrNoText
	}
	return x.res, nil
}

var pdfTextCleaner = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\x00", "", "￾", "")

func cleanPDFText(s string) string {
	lines := strings.Split(pdfTextCleaner.Replace(s), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// Page is one PDF page rendered as an image.
type Page struct {
	Number int // 1-based
	Image  Image
}

// RenderPDF renders pages first..first+count-1 (1-based; count default 5, max
// 20) as JPEG images with the long edge at most maxEdge (default 1568, max
// 4096), for reading a scanned PDF with a vision model. total is the page count.
// It would return ErrUnsupported in a build without rendering; every build has
// PDFium, so today it does not.
//
// A page is drawn to fill maxEdge, and its Image's OriginalWidth and
// OriginalHeight are the page size in points (1/72 inch).
func RenderPDF(path string, first, count, maxEdge int) (pages []Page, total int, err error) {
	first = max(first, 1)
	if count <= 0 {
		count = 5
	}
	count = min(count, 20)
	if maxEdge <= 0 {
		maxEdge = DefaultMaxEdge
	}
	maxEdge = min(maxEdge, maxRenderEdge)
	type rendered struct {
		pages []Page
		total int
	}
	r, err := withPDF(path, func(ctx context.Context, p pdfium.Pdfium, doc references.FPDF_DOCUMENT, n int) (rendered, error) {
		r := rendered{total: n}
		if first > n {
			return r, fmt.Errorf("the PDF has %d pages; there is no page %d", n, first)
		}
		for i := first - 1; i < n && i < first-1+count && ctx.Err() == nil; i++ {
			page, err := renderPage(p, doc, i, maxEdge)
			if err != nil {
				return r, fmt.Errorf("rendering page %d: %w", i+1, err)
			}
			r.pages = append(r.pages, page)
		}
		return r, nil
	})
	if err != nil {
		return nil, r.total, err
	}
	return r.pages, r.total, nil
}

func renderPage(p pdfium.Pdfium, doc references.FPDF_DOCUMENT, i, maxEdge int) (Page, error) {
	size, err := p.GetPageSize(&requests.GetPageSize{Page: pageRef(doc, i)})
	if err != nil {
		return Page{}, err
	}
	r, err := p.RenderPageInPixels(&requests.RenderPageInPixels{
		Page:        pageRef(doc, i),
		Width:       maxEdge,
		Height:      maxEdge,
		RenderFlags: enums.FPDF_RENDER_FLAG_ANNOT,
	})
	if err != nil {
		return Page{}, err
	}
	// The pixels live in PDFium's memory until Cleanup: encode first.
	defer r.Cleanup()
	img := r.Result.RenderedImage
	if r.Result.HasTransparency {
		// A page with transparency is drawn on a transparent background,
		// which JPEG would turn black.
		b := img.Bounds()
		white := image.NewRGBA(b)
		draw.Draw(white, b, image.White, image.Point{}, draw.Src)
		draw.Draw(white, b, img, b.Min, draw.Over)
		img = white
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return Page{}, err
	}
	w, h := int(math.Round(size.Width)), int(math.Round(size.Height))
	return Page{Number: i + 1, Image: Image{
		Data: buf.Bytes(), MIME: "image/jpeg",
		Width: r.Result.Width, Height: r.Result.Height,
		OriginalWidth: w, OriginalHeight: h,
		Resized: r.Result.Width != w || r.Result.Height != h,
	}}, nil
}
