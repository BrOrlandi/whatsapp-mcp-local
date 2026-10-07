package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/mediaread"
)

// defaultDocumentChars is how much of a document read_media returns unless
// asked for more: enough for a contract or a statement, without a long report
// filling the conversation.
const defaultDocumentChars = 60_000

func (s *Server) readMedia(ctx context.Context, a arguments) map[string]any {
	m, fail := s.message(ctx, a)
	if fail != nil {
		return fail
	}
	switch m.MediaType {
	case "":
		return toolError("message %s carries no media", m.ID)
	case "audio":
		return toolError("this is a voice note or audio: transcribe_audio turns it into text")
	case "video":
		return toolError("videos are not read; download_media returns the file")
	}
	as := a.As
	if as == "" {
		as = "auto"
	}
	if as != "auto" && as != "text" && as != "pages" {
		return toolError("as must be auto, text or pages")
	}
	d, err := s.fetchMedia(ctx, m)
	if err != nil {
		return toolError("%v", err)
	}
	meta := map[string]any{"message_id": m.ID, "chat_jid": m.ChatJID, "media_type": m.MediaType, "mime_type": m.MimeType,
		"filename": m.Filename, "path": d.Path, "bytes": d.Bytes}

	isImage := m.MediaType == "image" || m.MediaType == "sticker" || strings.HasPrefix(m.MimeType, "image/")
	isPDF := strings.HasPrefix(m.MimeType, "application/pdf") || strings.EqualFold(filepath.Ext(d.Path), ".pdf")
	switch {
	case isImage && as != "text":
		img, err := mediaread.ReadImage(d.Path, a.MaxEdge)
		if err != nil {
			return toolError("could not read the image: %v", err)
		}
		meta["width"], meta["height"], meta["resized"] = img.Width, img.Height, img.Resized
		if img.Resized {
			meta["original_size"] = map[string]int{"width": img.OriginalWidth, "height": img.OriginalHeight}
		}
		return contentResult(meta, imageBlock(img))
	case isPDF && as == "pages":
		return s.pdfPages(meta, d.Path, a)
	}

	maxChars := defaultDocumentChars
	if a.MaxContentChars > 0 {
		maxChars = min(a.MaxContentChars, 500_000)
	}
	text, err := mediaread.ReadText(d.Path, m.MimeType, mediaread.TextOptions{MaxChars: maxChars})
	switch {
	case errors.Is(err, mediaread.ErrNoText) && isPDF && as == "auto":
		// A scan: its pages are pictures, which a vision model reads.
		meta["note"] = "the PDF holds no text, so its pages come as pictures"
		return s.pdfPages(meta, d.Path, a)
	case errors.Is(err, mediaread.ErrUnsupported):
		return toolError("this kind of file (%s) cannot be read as text; download_media returns the file", orDefault(m.MimeType, filepath.Ext(d.Path)))
	case err != nil:
		return toolError("could not read the document: %v", err)
	}
	meta["format"], meta["pages"], meta["read"], meta["truncated"] = text.Format, text.Pages, text.Read, text.Truncated
	meta["text"] = text.Text
	meta["untrusted_content"] = UntrustedContent
	if text.Truncated {
		meta["next"] = "the text was cut; max_content_chars reads more (up to 500000)"
	}
	return textResult(meta, false)
}

// pdfPages answers with PDF pages rendered as pictures.
func (s *Server) pdfPages(meta map[string]any, path string, a arguments) map[string]any {
	first := max(a.FirstPage, 1)
	pages, total, err := mediaread.RenderPDF(path, first, a.Pages, a.MaxEdge)
	if err != nil {
		return toolError("could not render the PDF: %v", err)
	}
	numbers := make([]int, 0, len(pages))
	blocks := make([]any, 0, len(pages))
	for _, p := range pages {
		numbers = append(numbers, p.Number)
		blocks = append(blocks, imageBlock(p.Image))
	}
	meta["pages_total"], meta["pages_rendered"] = total, numbers
	if len(numbers) > 0 && numbers[len(numbers)-1] < total {
		meta["next"] = "more pages follow; first_page reads on from the next one"
	}
	return contentResult(meta, blocks...)
}

func imageBlock(img mediaread.Image) map[string]any {
	return map[string]any{"type": "image", "data": base64.StdEncoding.EncodeToString(img.Data), "mimeType": img.MIME}
}

// contentResult is a JSON description followed by content blocks.
func contentResult(meta map[string]any, blocks ...any) map[string]any {
	body, _ := json.MarshalIndent(meta, "", "  ")
	content := append([]any{map[string]any{"type": "text", "text": string(body)}}, blocks...)
	return map[string]any{"content": content, "isError": false}
}
