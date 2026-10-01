package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/index"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/localasr"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/state"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/transcribe"
)

const (
	keySetting  = "openai_api_key"
	autoSetting = "auto_transcribe" // "off" disables it; anything else is on
)

// reviewGuidance asks the client to do what speech recognition cannot: read
// the transcript against the conversation and fix what was misheard.
const reviewGuidance = "This transcript was made by speech recognition, which mishears names, places and jargon. Read it against the context messages: where a word is clearly a mishearing of something the conversation mentions (a name, a place, a product, a term), call save_transcript with the corrected text. Change only what the context makes certain; keep the speaker's wording, slang and mistakes otherwise, and never add content. The original stays kept as raw_text."

// ErrNoEngine means neither local transcription nor an OpenAI key is set up.
var ErrNoEngine = errors.New("no transcription engine is set up")

// Transcribe makes and keeps the transcript of one voice note: on this
// computer when the local engine is installed, with OpenAI otherwise. The
// conversation around the note is handed to the engine as context.
func (s *Server) Transcribe(ctx context.Context, m index.Message, language string, useOpenAI bool) (state.Transcript, index.AudioContext, error) {
	audioCtx, _ := s.index.AudioContext(ctx, m.ChatJID, m.ID, 12, 4)
	d, err := s.fetchMedia(ctx, m)
	if err != nil {
		return state.Transcript{}, audioCtx, err
	}
	t := state.Transcript{ChatJID: m.ChatJID, MessageID: m.ID, Language: language}
	switch {
	case !useOpenAI && s.asr != nil && s.asr.Status().Ready:
		text, err := s.asr.Transcribe(ctx, d.Path, language, audioCtx.Prompt())
		if err != nil {
			return t, audioCtx, err
		}
		t.Text, t.Raw, t.Model, t.Source = text, text, localasr.Label, "local"
	default:
		key, _ := s.state.Setting(ctx, keySetting)
		if key == "" {
			return t, audioCtx, ErrNoEngine
		}
		body, err := os.ReadFile(d.Path)
		if err != nil {
			return t, audioCtx, err
		}
		text, err := transcribe.Audio(ctx, key, d.Path, body, language)
		if err != nil {
			return t, audioCtx, err
		}
		t.Text, t.Raw, t.Model, t.Source = text, text, "openai "+transcribe.Model, "openai"
	}
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
	t, audioCtx, err := s.Transcribe(ctx, m, a.Language, a.Engine == "openai")
	switch {
	case errors.Is(err, ErrNoEngine):
		return textResult(map[string]any{"error": "no transcription engine is set up", "setup": []string{
			"On a Mac with Apple Silicon, install free local transcription: open the panel at " + s.baseURL + "/transcricao and click the install button, or run `whatsapp-mcp-v2 transcription install`. Audio never leaves the computer.",
			"Elsewhere, save an OpenAI API key with set_transcription_key (billed by OpenAI per minute of audio).",
		}}, true)
	case errors.Is(err, transcribe.ErrRejectedKey):
		return textResult(map[string]any{"error": "OpenAI rejected the saved key", "setup": []string{
			"Check the key at https://platform.openai.com/api-keys and that billing is active.",
			"Save a working key with set_transcription_key.",
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

func (s *Server) setTranscriptionKey(ctx context.Context, a arguments) map[string]any {
	if a.Remove {
		if err := s.state.SetSetting(ctx, keySetting, ""); err != nil {
			return toolError("%v", err)
		}
		return textResult(map[string]any{"removed": true}, false)
	}
	key := strings.TrimSpace(a.APIKey)
	if !strings.HasPrefix(key, "sk-") {
		return toolError("api_key must be an OpenAI key starting with sk-")
	}
	if err := transcribe.CheckKey(ctx, key); err != nil {
		return toolError("the key was not saved: %v", err)
	}
	if err := s.state.SetSetting(ctx, keySetting, key); err != nil {
		return toolError("%v", err)
	}
	return textResult(map[string]any{"saved": true, "key_hint": "…" + key[len(key)-4:]}, false)
}

// AutoStatus is what the background transcription has been doing.
type AutoStatus struct {
	Enabled bool      `json:"enabled"`
	Running bool      `json:"running"`
	Done    int       `json:"done"`
	Pending int       `json:"pending"`
	Failed  int       `json:"failed"`
	LastAt  time.Time `json:"last_at,omitempty"`
	LastErr string    `json:"last_error,omitempty"`
}

type autoState struct {
	mu     sync.Mutex
	status AutoStatus
	failed map[string]time.Time
}

func (s *Server) AutoTranscription() AutoStatus {
	s.auto.mu.Lock()
	defer s.auto.mu.Unlock()
	return s.auto.status
}

// autoWindow is how far back the background transcription reaches: the voice
// notes a person is likely to ask about, not the whole history.
const autoWindow = 7 * 24 * time.Hour

// RunAutoTranscription transcribes new voice notes in the background, newest
// first, with the local engine only: it costs nothing and nothing leaves the
// computer. A note that fails is retried after an hour, not in a loop.
func (s *Server) RunAutoTranscription(ctx context.Context) {
	s.auto.mu.Lock()
	s.auto.failed = map[string]time.Time{}
	s.auto.mu.Unlock()
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		s.autoPass(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Server) autoPass(ctx context.Context) {
	setting, _ := s.state.Setting(ctx, autoSetting)
	enabled := setting != "off" && s.asr != nil && s.asr.Status().Ready
	s.auto.mu.Lock()
	s.auto.status.Enabled = enabled
	s.auto.mu.Unlock()
	if !enabled {
		return
	}
	notes, err := s.index.RecentAudio(ctx, time.Now().Add(-autoWindow), 200)
	if err != nil {
		return
	}
	keys := make([]string, len(notes))
	for i, n := range notes {
		keys[i] = n.ChatJID + "/" + n.ID
	}
	done, err := s.state.Transcribed(ctx, keys)
	if err != nil {
		return
	}
	var todo []index.AudioMessage
	s.auto.mu.Lock()
	for _, n := range notes {
		k := n.ChatJID + "/" + n.ID
		if done[k] || time.Since(s.auto.failed[k]) < time.Hour {
			continue
		}
		todo = append(todo, n)
	}
	s.auto.status.Pending, s.auto.status.Running = len(todo), len(todo) > 0
	s.auto.mu.Unlock()

	for i, n := range todo {
		if ctx.Err() != nil {
			return
		}
		m, err := s.index.MessageByID(ctx, n.ID, n.ChatJID)
		if err == nil {
			cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			_, _, err = s.Transcribe(cctx, m, "", false)
			cancel()
		}
		s.auto.mu.Lock()
		s.auto.status.Pending = len(todo) - i - 1
		s.auto.status.LastAt = time.Now()
		if err != nil {
			s.auto.failed[n.ChatJID+"/"+n.ID] = time.Now()
			s.auto.status.Failed++
			s.auto.status.LastErr = err.Error()
		} else {
			s.auto.status.Done++
		}
		s.auto.mu.Unlock()
		if err != nil {
			s.logger.Warn("background transcription failed", "chat", n.ChatJID, "id", n.ID, "error", err)
		}
	}
	s.auto.mu.Lock()
	s.auto.status.Running = false
	s.auto.mu.Unlock()
}
