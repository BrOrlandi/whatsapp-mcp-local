package mcp

import (
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/localasr"
)

// The transcription check: a remote server or OpenAI that answers is ok; one
// that does not is still ok while this computer can stand in (a sleeping Mac
// is the usual case), and a warning when nothing can.
func TestTranscriptionHealth(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name   string
		rs     localasr.RemoteStatus
		local  bool
		status string
		detail string
		fix    string
	}{
		{"remote answers", localasr.RemoteStatus{Configured: true, Mode: "remote", URL: "http://mac:47823", Reachable: true, Ready: true}, false, "ok", "the transcription server at http://mac:47823", ""},
		{"openai answers", localasr.RemoteStatus{Configured: true, Mode: "openai", Model: "whisper-1", Reachable: true, Ready: true}, false, "ok", "OpenAI (whisper-1)", ""},
		{"mac asleep, local stands in", localasr.RemoteStatus{Configured: true, Mode: "remote", URL: "http://mac:47823", Error: "did not answer"}, true, "ok", "transcribed on this computer meanwhile", "transcription serve"},
		{"openai refused, nothing here", localasr.RemoteStatus{Configured: true, Mode: "openai", Model: "whisper-1", Error: "OpenAI refused the API key"}, false, "warn", "cannot be transcribed", "OpenAI API key"},
	} {
		in := healthy(now)
		rs := tc.rs
		in.Transcription, in.LocalTranscription = &rs, tc.local
		c := check(t, evaluate(in), "transcription")
		if c.Status != tc.status || !strings.Contains(c.Detail, tc.detail) || !strings.Contains(c.Fix, tc.fix) {
			t.Errorf("%s: %+v", tc.name, c)
		}
	}
}
