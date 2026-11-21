package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/index"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

// reviewGuidance asks the client to do what speech recognition cannot: read
// the transcript against the conversation and fix what was misheard.
const reviewGuidance = "This transcript was made by speech recognition, which mishears names, places and jargon. Read it against the context messages: where a word is clearly a mishearing of something the conversation mentions (a name, a place, a product, a term), call save_transcript with the corrected text. Change only what the context makes certain; keep the speaker's wording, slang and mistakes otherwise, and never add content. The original stays kept as raw_text."

// ErrNoEngine means transcription is set up neither here nor on a
// transcription server.
var ErrNoEngine = errors.New("transcription is not installed")

// Transcribe makes and keeps the transcript of one voice note, on a
// transcription server when one is set up, else on this computer. The
// conversation around the note is handed to the engine as context. backend
// says which made it, and fallback why the server was not used, when it was
// set up.
func (s *Server) Transcribe(ctx context.Context, m index.Message, language string) (t state.Transcript, audioCtx index.AudioContext, backend, fallback string, err error) {
	audioCtx, _ = s.index.AudioContext(ctx, m.ChatJID, m.ID, 12, 4)
	t = state.Transcript{ChatJID: m.ChatJID, MessageID: m.ID, Language: language}
	if s.asr == nil || !s.asr.Available() {
		return t, audioCtx, "", "", ErrNoEngine
	}
	d, err := s.fetchMedia(ctx, m)
	if err != nil {
		return t, audioCtx, "", "", err
	}
	r, err := s.asr.TranscribeAny(ctx, d.Path, language, audioCtx.Prompt())
	if err != nil {
		return t, audioCtx, "", "", err
	}
	t.Text, t.Raw, t.Model, t.Source = r.Text, r.Text, r.Model, r.Backend
	if err := s.state.SaveTranscript(ctx, t); err != nil {
		return t, audioCtx, r.Backend, r.Fallback, fmt.Errorf("transcribed, but could not keep the transcript: %w", err)
	}
	return t, audioCtx, r.Backend, r.Fallback, nil
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
	if s.asr == nil || !s.asr.Available() {
		return textResult(map[string]any{"error": "transcription is not installed on this computer", "setup": []string{
			"In the WhatsApp MCP app, open Configurações (the gear in the top right corner) and, under Transcrição de áudio, click the install button (in a browser: " + s.baseFor(ctx) + "/configuracoes#transcricao): it downloads about 600 MB once, and audio never leaves the computer. On the command line: `whatsapp-mcp transcription install`.",
			"Or point it at a transcription server on a computer with a GPU (`whatsapp-mcp transcription serve` there), in the same section or with `whatsapp-mcp transcription remote <address> --token <token>`.",
			"Meanwhile, download_media gives this audio as a file, to transcribe it another way.",
		}}, true)
	}
	// The work is not the call's: it goes on when the caller gives up, and
	// a later call for the same voice note joins it or finds it kept.
	job, joined := s.transcribing.start(m.ChatJID, m.ID, func(jctx context.Context) (state.Transcript, index.AudioContext, string, string, error) {
		return s.Transcribe(jctx, m, a.Language)
	})
	if !job.wait(ctx) {
		elapsed := time.Since(job.started)
		return textResult(map[string]any{"status": "in_progress", "joined": joined, "elapsed_seconds": int(elapsed.Seconds()),
			"retry_after_seconds": int(retryAfter.Seconds()), "backend": s.asr.Backend(),
			"message": fmt.Sprintf("This voice note is still being transcribed (for %ds). It goes on without you and the transcript is kept when it ends: call transcribe_audio again for this message in about %ds; calling again does not start over.", int(elapsed.Seconds()), int(retryAfter.Seconds()))}, false)
	}
	if errors.Is(job.err, ErrNoEngine) {
		return toolError("transcription is not installed")
	}
	if job.err != nil {
		return toolError("%v", job.err)
	}
	out := map[string]any{"transcript": job.t, "cached": false, "joined": joined, "backend": job.backend, "context": job.audioCtx, "review": reviewGuidance, "untrusted_content": UntrustedContent}
	if job.fallback != "" {
		out["fallback"] = "the transcription server was not used: " + job.fallback
	}
	return textResult(out, false)
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
