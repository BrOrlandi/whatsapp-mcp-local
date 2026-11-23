package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/index"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/localasr"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

// reviewGuidance asks the client to do what speech recognition cannot: read
// the transcript against the conversation and fix what was misheard.
const reviewGuidance = "This transcript was made by speech recognition, which mishears names, places and jargon. Read it against the context messages: where a word is clearly a mishearing of something the conversation mentions (a name, a place, a product, a term), call save_transcript with the corrected text. Change only what the context makes certain; keep the speaker's wording, slang and mistakes otherwise, and never add content. The original stays kept as raw_text."

// ErrNoEngine means transcription is set up neither here nor offsite (a
// remote server or OpenAI).
var ErrNoEngine = errors.New("transcription is not installed")

// Transcribe makes and keeps the transcript of one voice note, on a remote
// server or OpenAI when one is set up, else on this computer. As much of the
// conversation as the context level allows goes with it (contextLevel):
// nothing, the names, or the names and the latest messages. backend says
// which made it (remote, openai or local), and fallback why the remote
// server or OpenAI was not used, when one was set up.
func (s *Server) Transcribe(ctx context.Context, m index.Message, language string) (t state.Transcript, audioCtx index.AudioContext, backend, fallback, contextLevel string, err error) {
	audioCtx, _ = s.index.AudioContext(ctx, m.ChatJID, m.ID, 12, 4)
	t = state.Transcript{ChatJID: m.ChatJID, MessageID: m.ID, Language: language}
	if s.asr == nil || !s.asr.Available() {
		return t, audioCtx, "", "", "", ErrNoEngine
	}
	d, err := s.fetchMedia(ctx, m)
	if err != nil {
		return t, audioCtx, "", "", "", err
	}
	contextLevel = s.asr.Settings().ContextLevel()
	r, err := s.asr.TranscribeAny(ctx, d.Path, language, enginePrompt(audioCtx, contextLevel))
	if err != nil {
		return t, audioCtx, "", "", contextLevel, err
	}
	t.Text, t.Raw, t.Model, t.Source = r.Text, r.Text, r.Model, r.Backend
	if err := s.state.SaveTranscript(ctx, t); err != nil {
		return t, audioCtx, r.Backend, r.Fallback, contextLevel, fmt.Errorf("transcribed, but could not keep the transcript: %w", err)
	}
	// The answer is the transcript as kept, with the date it was kept, the
	// same a later call finds; a call that joined this one gets it too.
	if kept, err := s.state.Transcript(ctx, m.ChatJID, m.ID); err == nil {
		t = kept
	}
	return t, audioCtx, r.Backend, r.Fallback, contextLevel, nil
}

// enginePrompt is what of the conversation goes to the engine at a context
// level: nothing, the names, or the names and the latest messages.
func enginePrompt(c index.AudioContext, level string) string {
	switch level {
	case localasr.ContextMessages:
		return c.Prompt()
	case localasr.ContextNames:
		return c.NamesPrompt()
	}
	return ""
}

// contextSent says, for the AI, what goes with a voice note to whatever
// transcribes it at a context level.
func contextSent(level string) string {
	switch level {
	case localasr.ContextMessages:
		return "names and recent messages: the chat's name, the people's names and the latest text messages before the voice note go with it to the transcription"
	case localasr.ContextNames:
		return "names only: the chat's name and the people's names go with the voice note to the transcription, no message text"
	}
	return "none: only the audio goes to the transcription, nothing of the conversation"
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
		var st localasr.Status
		if s.asr != nil {
			st = s.asr.Status()
		}
		return textResult(noEngineAnswer(st, s.baseFor(ctx)), true)
	}
	// The work is not the call's: it goes on when the caller gives up, and
	// a later call for the same voice note joins it or finds it kept.
	job, joined := s.transcribing.start(m.ChatJID, m.ID, func(jctx context.Context) (state.Transcript, index.AudioContext, string, string, string, error) {
		return s.Transcribe(jctx, m, a.Language)
	})
	if !job.wait(ctx) {
		elapsed := time.Since(job.started)
		return textResult(map[string]any{"status": "in_progress", "joined": joined, "elapsed_seconds": int(elapsed.Seconds()),
			"retry_after_seconds": int(retryAfter.Seconds()), "backend": s.asr.Backend(),
			"message": fmt.Sprintf("This voice note is still being transcribed (for %ds). It goes on without you and the transcript is kept when it ends: call transcribe_audio again for this message in about %ds; calling again does not start over.", int(elapsed.Seconds()), int(retryAfter.Seconds()))}, false)
	}
	if errors.Is(job.err, ErrNoEngine) {
		var st localasr.Status
		if s.asr != nil {
			st = s.asr.Status()
		}
		return textResult(noEngineAnswer(st, s.baseFor(ctx)), true)
	}
	if errors.Is(job.err, localasr.ErrUnavailable) {
		// The engine was installed but failed to start: the same steps as
		// when it is known not to run here.
		st := s.asr.Status()
		if st.Unavailable == "" {
			st.Unavailable = localasr.UnavailableEngine
		}
		out := noEngineAnswer(st, s.baseFor(ctx))
		out["detail"] = job.err.Error()
		return textResult(out, true)
	}
	if job.err != nil {
		return toolError("%v", job.err)
	}
	out := map[string]any{"transcript": job.t, "cached": false, "joined": joined, "backend": job.backend, "context": job.audioCtx,
		"context_level": job.context, "context_sent": contextSent(job.context), "review": reviewGuidance, "untrusted_content": UntrustedContent}
	if job.fallback != "" {
		out["fallback"] = "transcribed on this computer because the configured " + s.asr.Settings().Mode + " transcription failed: " + job.fallback
	}
	return textResult(out, false)
}

// noEngineAnswer is transcribe_audio's answer when nothing can transcribe:
// what is wrong and the steps the AI can take or walk the user through. st
// is this computer's engine: not installed, or installed but unable to run
// here (a Windows without the Visual C++ runtime), where installing again
// does not help.
func noEngineAnswer(st localasr.Status, base string) map[string]any {
	settings := "In the WhatsApp MCP app, open Configurações (the gear in the top right corner), Transcrição de áudio (in a browser: " + base + "/configuracoes#transcricao)"
	elsewhere := "send voice notes to any OpenAI-compatible server (\"Em um servidor compatível com a OpenAI\"), such as a Mac with an Apple chip running `whatsapp-mcp transcription serve`, or to OpenAI with the user's own API key (\"Na OpenAI\"); on the command line: `whatsapp-mcp transcription remote <address>` or `whatsapp-mcp transcription openai`."
	download := "Or, without changing any setting, call download_media for this message: it gives the voice note as a file, and you can transcribe it with your own tools."
	if st.Unavailable != "" {
		return map[string]any{
			"error":       "local transcription is not available on this computer yet: it does not work on Windows yet (the engine cannot start here)",
			"unavailable": st.Unavailable,
			"setup": []string{
				settings + ", click Trocar onde transcrever and choose where voice notes go instead: " + elsewhere,
				download,
				"Installing transcription again on this computer does not help.",
			},
		}
	}
	return map[string]any{"error": "transcription is not installed on this computer", "setup": []string{
		settings + ", and click the install button: it downloads about 600 MB once, and audio never leaves the computer. On the command line: `whatsapp-mcp transcription install`.",
		"Or, in the same section (Trocar onde transcrever), " + elsewhere,
		"Meanwhile, download_media gives this audio as a file, to transcribe it another way.",
	}}
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
