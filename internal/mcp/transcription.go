package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/index"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/localasr"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

// reviewGuidance asks the client to do what speech recognition cannot: read
// the transcript against the conversation and fix what was misheard.
const reviewGuidance = "This transcript was made by speech recognition, which mishears names, places and jargon. Read it against the context messages: where a word is clearly a mishearing of something the conversation mentions (a name, a place, a product, a term), call save_transcript with the corrected text. Change only what the context makes certain; keep the speaker's wording, slang and mistakes otherwise, and never add content. The original stays kept as raw_text."

// ErrNoEngine means local transcription is not installed.
var ErrNoEngine = errors.New("transcription is not installed")

// Transcribe makes and keeps the transcript of one voice note, on this
// computer. The conversation around the note is handed to the engine as
// context.
func (s *Server) Transcribe(ctx context.Context, m index.Message, language string) (state.Transcript, index.AudioContext, error) {
	audioCtx, _ := s.index.AudioContext(ctx, m.ChatJID, m.ID, 12, 4)
	d, err := s.fetchMedia(ctx, m)
	if err != nil {
		return state.Transcript{}, audioCtx, err
	}
	t := state.Transcript{ChatJID: m.ChatJID, MessageID: m.ID, Language: language}
	if s.asr == nil || !s.asr.Status().Ready {
		return t, audioCtx, ErrNoEngine
	}
	text, err := s.asr.Transcribe(ctx, d.Path, language, audioCtx.Prompt())
	if err != nil {
		return t, audioCtx, err
	}
	t.Text, t.Raw, t.Model, t.Source = text, text, localasr.Label, "local"
	if err := s.state.SaveTranscript(ctx, t); err != nil {
		return t, audioCtx, fmt.Errorf("transcribed, but could not keep the transcript: %w", err)
	}
	return t, audioCtx, nil
}

func (s *Server) transcribeAudio(ctx context.Context, a arguments) map[string]any {
	m, fail := s.message(ctx, a)
	if fail != nil {
		return fail
	}
	if m.MediaType != "audio" {
		return toolError("message %s is not a voice note (media_type is %q)", m.ID, m.MediaType)
	}
	if !a.Refresh {
		if t, err := s.state.Transcript(ctx, m.ChatJID, m.ID); err == nil {
			audioCtx, _ := s.index.AudioContext(ctx, m.ChatJID, m.ID, 12, 4)
			return textResult(map[string]any{"transcript": t, "cached": true, "context": audioCtx, "review": reviewGuidance, "untrusted_content": UntrustedContent}, false)
		}
	}
	t, audioCtx, err := s.Transcribe(ctx, m, a.Language)
	switch {
	case errors.Is(err, ErrNoEngine):
		return textResult(map[string]any{"error": "transcription is not installed on this computer", "setup": []string{
			"In the WhatsApp MCP app, open Configurações (the gear in the top right corner) and, under Transcrição de áudio, click the install button (in a browser: " + s.base() + "/configuracoes#transcricao): it downloads about 600 MB once, and audio never leaves the computer. On the command line: `whatsapp-mcp transcription install`.",
			"Meanwhile, download_media gives this audio as a file, to transcribe it another way.",
		}}, true)
	case err != nil:
		return toolError("%v", err)
	}
	return textResult(map[string]any{"transcript": t, "cached": false, "context": audioCtx, "review": reviewGuidance, "untrusted_content": UntrustedContent}, false)
}

func (s *Server) saveTranscript(ctx context.Context, a arguments) map[string]any {
	if strings.TrimSpace(a.Text) == "" {
		return toolError("text is required")
	}
	m, fail := s.message(ctx, a)
	if fail != nil {
		return fail
	}
	t := state.Transcript{ChatJID: m.ChatJID, MessageID: m.ID, Text: strings.TrimSpace(a.Text), Language: a.Language, Model: a.Model, Source: "client"}
	// A correction keeps what the engine first heard, so it can be checked.
	if prev, err := s.state.Transcript(ctx, m.ChatJID, m.ID); err == nil {
		t.Raw = prev.Raw
		if t.Raw == "" {
			t.Raw = prev.Text
		}
		t.Source = "corrected"
		if t.Model == "" {
			t.Model = prev.Model
		}
		if t.Language == "" {
			t.Language = prev.Language
		}
	}
	if err := s.state.SaveTranscript(ctx, t); err != nil {
		return toolError("%v", err)
	}
	return textResult(map[string]any{"saved": true, "transcript": t}, false)
}
