// Package transcribe sends a voice note to OpenAI's transcription endpoint.
package transcribe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

const (
	baseURL = "https://api.openai.com/v1"
	Model   = "whisper-1"
)

// ErrRejectedKey means OpenAI refused the key itself, as opposed to failing.
var ErrRejectedKey = errors.New("OpenAI rejected the API key")

var client = &http.Client{Timeout: 3 * time.Minute}

// CheckKey confirms OpenAI accepts a key before it is saved.
func CheckKey(ctx context.Context, key string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models/"+Model, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach OpenAI: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrRejectedKey
	case resp.StatusCode >= 300:
		return fmt.Errorf("OpenAI answered %s", resp.Status)
	}
	return nil
}

// Audio transcribes one file.
func Audio(ctx context.Context, key, path string, file []byte, language string) (string, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", audioName(path))
	if err != nil {
		return "", err
	}
	if _, err := part.Write(file); err != nil {
		return "", err
	}
	_ = form.WriteField("model", Model)
	_ = form.WriteField("response_format", "json")
	if language != "" {
		_ = form.WriteField("language", language)
	}
	if err := form.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/audio/transcriptions", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", form.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not reach OpenAI: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode == http.StatusUnauthorized {
		return "", ErrRejectedKey
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("OpenAI answered %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("could not read OpenAI's answer: %w", err)
	}
	return strings.TrimSpace(out.Text), nil
}

// audioName gives OpenAI a file name whose extension it recognises: WhatsApp
// voice notes are Ogg Opus, which it accepts as .ogg.
func audioName(path string) string {
	name := filepath.Base(path)
	switch strings.ToLower(filepath.Ext(name)) {
	case ".ogg", ".oga", ".opus", ".mp3", ".m4a", ".mp4", ".wav", ".webm", ".mpeg", ".mpga", ".flac":
		return name
	}
	return name + ".ogg"
}
