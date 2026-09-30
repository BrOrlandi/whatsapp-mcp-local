package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/index"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/wacli"
)

// Check is one verdict of the health report.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok, warn or fail
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// Health is the answer to "is this working?": one overall verdict and the
// checks it was made from, each with what to do when it is not ok.
type Health struct {
	Status   string         `json:"status"`
	Summary  string         `json:"summary"`
	Checks   []Check        `json:"checks"`
	Activity index.Activity `json:"activity"`
	At       time.Time      `json:"checked_at"`
}

// healthInputs is everything the verdict depends on, gathered first so the
// judgement itself is a pure function that can be tested.
type healthInputs struct {
	Now            time.Time
	Uptime         time.Duration
	DoctorErr      error
	Authenticated  bool
	SessionRevoked bool
	Sync           wacli.Status
	Activity       index.Activity
	ActivityErr    error
	Gaps           []index.Gap
	History        *historyJob
	MaxSilence     time.Duration
}

const defaultMaxSilence = 6 * time.Hour

// Health runs the checks. maxSilence is how long without an incoming message
// still counts as healthy; zero means the default.
func (s *Server) Health(ctx context.Context, maxSilence time.Duration) Health {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	now := time.Now()
	in := healthInputs{Now: now, Uptime: now.Sub(s.started), Sync: s.supervisor.Status(), History: s.historyStatus(), MaxSilence: maxSilence}

	var doctor struct {
		Authenticated  bool `json:"authenticated"`
		SessionRevoked bool `json:"session_revoked"`
	}
	in.DoctorErr = s.cli.Decode(ctx, &doctor, "--read-only", "doctor")
	in.Authenticated, in.SessionRevoked = doctor.Authenticated, doctor.SessionRevoked
	in.Activity, in.ActivityErr = s.index.Activity(ctx, now)
	in.Gaps, _ = s.index.Gaps(ctx, gapThreshold, 7*24*time.Hour, 20)
	return evaluate(in)
}

func evaluate(in healthInputs) Health {
	if in.MaxSilence <= 0 {
		in.MaxSilence = defaultMaxSilence
	}
	h := Health{At: in.Now.UTC(), Activity: in.Activity}
	add := func(name, status, detail, fix string) {
		h.Checks = append(h.Checks, Check{Name: name, Status: status, Detail: detail, Fix: fix})
	}

	add("daemon", "ok", fmt.Sprintf("answering, version %s, up for %s", Version, round(in.Uptime)), "")

	switch {
	case in.DoctorErr != nil:
		add("wacli", "fail", "wacli did not answer: "+in.DoctorErr.Error(), "check that wacli is installed (brew install openclaw/tap/wacli) and that WACLI_BIN points at it")
	case in.SessionRevoked:
		add("paired", "fail", "WhatsApp revoked this linked device", "pair again: wacli auth logout, then wacli auth")
	case !in.Authenticated:
		add("paired", "fail", "WhatsApp is not paired on this machine", "run wacli auth in a terminal and scan the QR code from Linked devices on the phone")
	default:
		add("paired", "ok", "WhatsApp is paired as a linked device", "")
	}

	sync := in.Sync
	for_ := round(in.Now.Sub(sync.Since))
	switch {
	case sync.State == "connected" && sync.Delegate:
		add("sync", "ok", "connected to WhatsApp for "+for_, "")
	case sync.State == "connected":
		add("sync", "warn", "connected, but its send socket is not up yet", "wait a few seconds; if it persists, restart the daemon")
	case sync.State == "paused":
		add("sync", "warn", fmt.Sprintf("paused for %s while it runs %s", for_, sync.PausedFor), "nothing to do: it restarts on its own when that operation ends")
	case sync.State == "starting" && in.Now.Sub(sync.Since) < 2*time.Minute:
		add("sync", "warn", "starting, for "+for_, "wait a minute: a first sync after pairing can take a while")
	case sync.State == "reconnecting":
		add("sync", "warn", "reconnecting to WhatsApp, for "+for_, "check the internet connection; it keeps retrying on its own")
	case sync.State == "not_paired":
		add("sync", "fail", "waiting for WhatsApp to be paired", "run wacli auth")
	case sync.State == "logged_out":
		add("sync", "fail", "WhatsApp logged this device out", "pair again: wacli auth logout, then wacli auth")
	default:
		detail := "not running (" + sync.State + ")"
		if sync.LastError != "" {
			detail += ": " + sync.LastError
		}
		add("sync", "fail", detail, "look at the daemon log (~/Library/Logs/whatsapp-mcp-v2.log); stop any wacli sync started by hand")
	}

	a := in.Activity
	switch {
	case in.ActivityErr != nil:
		add("receiving", "fail", in.ActivityErr.Error(), "pair WhatsApp with wacli auth")
	case a.NewestIncoming == nil:
		add("receiving", "warn", "no message from anyone has been stored yet", "the first sync after pairing may still be running; check again in a few minutes")
	default:
		age := in.Now.Sub(*a.NewestIncoming)
		detail := fmt.Sprintf("last message received %s ago (%s); %d messages in the last hour, %d in 24 hours",
			round(age), a.NewestIncoming.Local().Format("2006-01-02 15:04"), a.LastHour, a.LastDay)
		switch {
		case age <= in.MaxSilence:
			add("receiving", "ok", detail, "")
		case age <= 24*time.Hour:
			add("receiving", "warn", detail, "quiet for longer than usual: normal overnight or in a quiet account; to be sure, have someone send this account a message and check again")
		default:
			add("receiving", "fail", detail, "a whole day without any incoming message usually means the device is not receiving: check sync, and that the phone is online")
		}
	}

	if len(in.Gaps) > 0 {
		longest := in.Gaps[0]
		for _, g := range in.Gaps {
			if g.Hours > longest.Hours {
				longest = g
			}
		}
		add("coverage", "warn", fmt.Sprintf("%d window(s) in the last 7 days with no message in any conversation; the longest is %.0f hours from %s",
			len(in.Gaps), longest.Hours, longest.From.Local().Format("2006-01-02 15:04")),
			"these are usually a closed laptop; messages from them arrive if WhatsApp still had them queued, and whatsapp_status lists them")
	} else if a.NewestAny != nil {
		add("coverage", "ok", "no silent windows in the last 7 days", "")
	}

	if job := in.History; job != nil && len(job.Failures) > 0 {
		add("history_request", "warn", fmt.Sprintf("the last sync_history request failed for %d of %d conversations", len(job.Failures), len(job.Chats)),
			"the phone must be online to answer; whatsapp_status shows each failure")
	}

	h.Status = "ok"
	var failing, warning []string
	for _, c := range h.Checks {
		switch c.Status {
		case "fail":
			failing = append(failing, c.Name)
		case "warn":
			warning = append(warning, c.Name)
		}
	}
	switch {
	case len(failing) > 0:
		h.Status = "fail"
		h.Summary = fmt.Sprintf("not working: %v", failing)
	case len(warning) > 0:
		h.Status = "warn"
		h.Summary = fmt.Sprintf("working, with warnings: %v", warning)
	default:
		h.Summary = "working: connected to WhatsApp and receiving messages"
	}
	return h
}

func round(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	return d.Round(time.Minute).String()
}

func (s *Server) health(ctx context.Context, a arguments) map[string]any {
	h := s.Health(ctx, time.Duration(a.MaxSilenceHours*float64(time.Hour)))
	return textResult(h, false)
}
