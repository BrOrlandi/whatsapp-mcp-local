package mcp

import (
	"errors"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/index"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/wacli"
)

func healthy(now time.Time) healthInputs {
	last := now.Add(-20 * time.Minute)
	return healthInputs{
		Now:           now,
		Uptime:        3 * time.Hour,
		Authenticated: true,
		Sync:          wacli.Status{State: "connected", Since: now.Add(-3 * time.Hour), Delegate: true},
		Activity:      index.Activity{NewestIncoming: &last, NewestAny: &last, LastHour: 12, LastDay: 140},
	}
}

func check(t *testing.T, h Health, name string) Check {
	t.Helper()
	for _, c := range h.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no %q check in %+v", name, h.Checks)
	return Check{}
}

func TestHealthVerdicts(t *testing.T) {
	now := time.Date(2026, 9, 30, 15, 0, 0, 0, time.UTC)
	hoursAgo := func(h float64) *time.Time {
		t := now.Add(-time.Duration(h * float64(time.Hour)))
		return &t
	}

	cases := []struct {
		name      string
		mutate    func(*healthInputs)
		want      string
		failing   string
		checkWant string
	}{
		{name: "all good", mutate: func(*healthInputs) {}, want: "ok"},
		{name: "not paired", mutate: func(in *healthInputs) {
			in.Authenticated = false
			in.Sync = wacli.Status{State: "not_paired", Since: now}
		}, want: "fail", failing: "paired", checkWant: "fail"},
		{name: "logged out", mutate: func(in *healthInputs) {
			in.SessionRevoked = true
		}, want: "fail", failing: "paired", checkWant: "fail"},
		{name: "wacli missing", mutate: func(in *healthInputs) {
			in.DoctorErr = errors.New("exec: wacli: not found")
		}, want: "fail", failing: "wacli", checkWant: "fail"},
		{name: "sync crashed", mutate: func(in *healthInputs) {
			in.Sync = wacli.Status{State: "exited", Since: now, LastError: "exit status 1"}
		}, want: "fail", failing: "sync", checkWant: "fail"},
		{name: "sync paused for a history request", mutate: func(in *healthInputs) {
			in.Sync = wacli.Status{State: "paused", Since: now.Add(-10 * time.Second), PausedFor: "history backfill"}
		}, want: "warn", failing: "sync", checkWant: "warn"},
		{name: "quiet for eight hours", mutate: func(in *healthInputs) {
			in.Activity.NewestIncoming = hoursAgo(8)
		}, want: "warn", failing: "receiving", checkWant: "warn"},
		{name: "quiet account with a higher tolerance", mutate: func(in *healthInputs) {
			in.Activity.NewestIncoming = hoursAgo(8)
			in.MaxSilence = 12 * time.Hour
		}, want: "ok"},
		{name: "a whole day without messages", mutate: func(in *healthInputs) {
			in.Activity.NewestIncoming = hoursAgo(30)
		}, want: "fail", failing: "receiving", checkWant: "fail"},
		{name: "empty store right after pairing", mutate: func(in *healthInputs) {
			in.Activity = index.Activity{}
		}, want: "warn", failing: "receiving", checkWant: "warn"},
		{name: "a closed-laptop window", mutate: func(in *healthInputs) {
			in.Gaps = []index.Gap{{From: now.Add(-50 * time.Hour), Until: now.Add(-30 * time.Hour), Hours: 20}}
		}, want: "warn", failing: "coverage", checkWant: "warn"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := healthy(now)
			tc.mutate(&in)
			h := evaluate(in)
			if h.Status != tc.want {
				t.Fatalf("status %q, want %q; checks %+v", h.Status, tc.want, h.Checks)
			}
			if tc.failing != "" {
				c := check(t, h, tc.failing)
				if c.Status != tc.checkWant {
					t.Fatalf("%s is %q, want %q", tc.failing, c.Status, tc.checkWant)
				}
				if c.Fix == "" {
					t.Fatalf("%s is not ok but says nothing about what to do", tc.failing)
				}
			}
		})
	}
}
