package mcp

import (
	"context"
	"time"
)

func (s *Server) status(ctx context.Context, _ arguments) map[string]any {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	report := map[string]any{"gateway": map[string]any{"version": Version, "endpoint": s.base() + "/mcp"}}

	var problems []string
	sync := s.supervisor.Status()
	report["sync"] = sync

	var doctor map[string]any
	if err := s.cli.Decode(ctx, &doctor, "--read-only", "doctor"); err != nil {
		problems = append(problems, "wacli doctor failed: "+err.Error())
	} else {
		report["wacli"] = doctor
		if auth, _ := doctor["authenticated"].(bool); !auth {
			problems = append(problems, "WhatsApp is not paired on this machine: run `wacli auth` in a terminal and scan the QR code from Linked devices on the phone")
		}
	}

	switch sync.State {
	case "logged_out":
		problems = append(problems, "WhatsApp logged this device out; pair again with `wacli auth logout` then `wacli auth`")
	case "exited", "error":
		problems = append(problems, "sync is not running: "+sync.LastError)
	case "paused":
		report["note"] = "sync is paused while an operation that needs the store runs (" + sync.PausedFor + "); it restarts on its own"
	}

	if coverage, err := s.index.Coverage(ctx); err == nil {
		report["index"] = coverage
		if coverage.Newest != nil && time.Since(*coverage.Newest) > 24*time.Hour && sync.State == "connected" {
			problems = append(problems, "no message arrived in over a day although sync is connected")
		}
	}
	if gaps, err := s.index.Gaps(ctx, gapThreshold, 30*24*time.Hour, 10); err == nil && len(gaps) > 0 {
		report["unknown_windows"] = gaps
		report["unknown_windows_note"] = "windows of the last 30 days in which no conversation received anything, the shape a stopped sync (a closed laptop) leaves; sync_history cannot fill a window that has no later message in the same chat"
	}

	if job := s.historyStatus(); job != nil {
		report["history_request"] = job
	}
	transcription := map[string]any{"openai_key": nil, "mode": "on request only: transcribe_audio"}
	if key, _ := s.state.Setting(ctx, keySetting); key != "" {
		transcription["openai_key"] = "…" + key[len(key)-4:]
	}
	if s.asr != nil {
		st := s.asr.Status()
		transcription["local"] = map[string]any{"ready": st.Ready, "supported": st.Supported, "missing": st.Missing, "engine": "whisper.cpp large-v3-turbo"}
	}
	if total, corrected, err := s.state.TranscriptCount(ctx); err == nil {
		transcription["transcripts"] = total
		transcription["corrected_from_context"] = corrected
	}
	report["transcription"] = transcription
	if problems == nil {
		problems = []string{}
	}
	report["problems"] = problems
	return textResult(report, false)
}
