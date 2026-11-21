package localasr

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxAudio is the largest voice note a transcription server takes: WhatsApp
// voice notes are Opus at a few KB a second, so this is hours of speech.
const maxAudio = 64 << 20

// maxPrompt bounds the context sent with a voice note; whisper.cpp itself
// keeps only its last 224 tokens.
const maxPrompt = 8 << 10

// Handler is the transcription server `whatsapp-mcp transcription serve`
// runs: this engine, over HTTP, for another computer's app.
//
//	GET  /v1/health      {"ready", "accel", "engine", "version"}
//	POST /v1/transcribe  multipart: audio (the file), language, prompt
//	                     → {"text", "engine", "accel", "seconds"}
//
// Every request carries "Authorization: Bearer <token>"; token must not be
// empty.
func (e *Engine) Handler(token, version string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		st := e.Status()
		writeJSON(w, http.StatusOK, health{Ready: st.Ready, Accel: st.Accel, Engine: "whisper.cpp large-v3-turbo", Version: version})
	})
	mux.HandleFunc("POST /v1/transcribe", func(w http.ResponseWriter, r *http.Request) {
		if !e.Status().Ready {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "transcription is not installed on this computer: run `whatsapp-mcp transcription install`"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxAudio+1<<20)
		file, _, err := r.FormFile("audio")
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "the voice note is too large"})
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "send the voice note as the multipart field audio"})
			return
		}
		defer file.Close()
		defer func() {
			if r.MultipartForm != nil {
				_ = r.MultipartForm.RemoveAll()
			}
		}()
		dir, err := os.MkdirTemp("", "wamcp-asr-in-")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		defer os.RemoveAll(dir)
		path := filepath.Join(dir, "audio")
		out, err := os.Create(path)
		if err == nil {
			_, err = io.Copy(out, file)
			if cerr := out.Close(); err == nil {
				err = cerr
			}
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		language := strings.TrimSpace(r.FormValue("language"))
		if len(language) > 8 {
			language = ""
		}
		prompt := r.FormValue("prompt")
		if len(prompt) > maxPrompt {
			prompt = prompt[len(prompt)-maxPrompt:]
		}
		start := time.Now()
		// The request's context: when the app gives up, the work stops here.
		text, err := e.Transcribe(r.Context(), path, language, prompt)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"text": text, "engine": "whisper.cpp large-v3-turbo", "accel": e.Status().Accel,
			"seconds": time.Since(start).Seconds()})
	})
	return requireToken(token, mux)
}

func requireToken(token string, next http.Handler) http.Handler {
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if token == "" || subtle.ConstantTimeCompare(got, want) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing or wrong token"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// Server is the transcription server's HTTP server. It has no read or write
// deadline: a transcription takes as long as it takes, and a deadline that
// passes while it runs would cancel it.
func (e *Engine) Server(addr, token, version string) *http.Server {
	return &http.Server{Addr: addr, Handler: e.Handler(token, version), ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 32 << 10}
}
