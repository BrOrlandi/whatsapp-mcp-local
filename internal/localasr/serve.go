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

// serverEngine names this engine in the server's answers.
const serverEngine = "whisper.cpp large-v3-turbo"

// Handler is the transcription server `whatsapp-mcp transcription serve`
// runs: this engine, over HTTP, for another computer's app. It speaks
// OpenAI's transcription API, so any client of that API can use it:
//
//	POST /v1/audio/transcriptions  multipart: file, model (any), language,
//	                               prompt, response_format (json, text or
//	                               verbose_json) → {"text"}
//	GET  /v1/models                OpenAI's list, plus "ready", "accel",
//	                               "engine" and "version"
//	GET  /v1/health                {"ready", "accel", "engine", "version"}
//
// Errors come as OpenAI's {"error": {"message", "type", "code"}}. Every
// request carries "Authorization: Bearer <token>"; token must not be empty.
func (e *Engine) Handler(token, version string) http.Handler {
	mux := http.NewServeMux()
	about := func() map[string]any {
		st := e.Status()
		out := map[string]any{"ready": st.Ready, "accel": st.Accel, "engine": serverEngine, "version": version}
		if st.Unavailable != "" {
			out["unavailable"] = st.Unavailable
		}
		return out
	}
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, about())
	})
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		out := about()
		data := []map[string]any{}
		if e.Status().Ready {
			// Any model name is taken; these are the ones it answers to.
			for _, id := range []string{DefaultModel, "whisper-large-v3-turbo"} {
				data = append(data, map[string]any{"id": id, "object": "model", "created": 0, "owned_by": "whatsapp-mcp"})
			}
		}
		out["object"], out["data"] = "list", data
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("POST /v1/audio/transcriptions", func(w http.ResponseWriter, r *http.Request) {
		if st := e.Status(); st.Unavailable != "" {
			apiError(w, http.StatusServiceUnavailable, "server_error", "engine_unavailable", unavailableError(st.Unavailable).Error())
			return
		} else if !st.Ready {
			apiError(w, http.StatusServiceUnavailable, "server_error", "engine_not_installed", "transcription is not installed on this computer: run `whatsapp-mcp transcription install`")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxAudio+1<<20)
		file, _, err := r.FormFile("file")
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				apiError(w, http.StatusRequestEntityTooLarge, "invalid_request_error", "file_too_large", "the audio file is too large")
				return
			}
			apiError(w, http.StatusBadRequest, "invalid_request_error", "missing_file", "send the audio as the multipart field file")
			return
		}
		defer file.Close()
		defer func() {
			if r.MultipartForm != nil {
				_ = r.MultipartForm.RemoveAll()
			}
		}()
		format := r.FormValue("response_format")
		switch format {
		case "", "json", "text", "verbose_json":
		default:
			apiError(w, http.StatusBadRequest, "invalid_request_error", "unsupported_response_format", "response_format must be json, text or verbose_json")
			return
		}
		dir, err := os.MkdirTemp("", "wamcp-asr-in-")
		if err != nil {
			apiError(w, http.StatusInternalServerError, "server_error", "", err.Error())
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
			apiError(w, http.StatusInternalServerError, "server_error", "", err.Error())
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
		// The request's context: when the client gives up, the work stops here.
		text, seconds, err := e.transcribe(r.Context(), path, language, prompt)
		if err != nil {
			apiError(w, http.StatusInternalServerError, "server_error", "transcription_failed", err.Error())
			return
		}
		switch format {
		case "text":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = io.WriteString(w, text+"\n")
		case "verbose_json":
			writeJSON(w, http.StatusOK, map[string]any{"task": "transcribe", "language": language, "duration": seconds, "text": text, "engine": serverEngine})
		default:
			writeJSON(w, http.StatusOK, map[string]any{"text": text, "engine": serverEngine})
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		apiError(w, http.StatusNotFound, "invalid_request_error", "unknown_url", "unknown request: "+r.Method+" "+r.URL.Path)
	})
	return requireToken(token, mux)
}

// apiError answers as OpenAI's API does.
func apiError(w http.ResponseWriter, code int, kind, errCode, msg string) {
	var c any
	if errCode != "" {
		c = errCode
	}
	writeJSON(w, code, map[string]any{"error": map[string]any{"message": msg, "type": kind, "param": nil, "code": c}})
}

func requireToken(token string, next http.Handler) http.Handler {
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if token == "" || subtle.ConstantTimeCompare(got, want) != 1 {
			apiError(w, http.StatusUnauthorized, "invalid_request_error", "invalid_api_key", "missing or wrong token")
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
