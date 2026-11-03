package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestEveryCommandHasHelp keeps `whatsapp-mcp <command> --help` working for
// every command main dispatches: a new case in its switch needs an entry in
// commandHelp.
func TestEveryCommandHasHelp(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	body = body[strings.Index(body, "\tswitch cmd {"):]
	body = body[:strings.Index(body, "\tdefault:")]
	cases := regexp.MustCompile(`case ([^:]+):`).FindAllStringSubmatch(body, -1)
	if len(cases) < 10 {
		t.Fatalf("found only %d cases in main's switch", len(cases))
	}
	for _, c := range cases {
		for _, name := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(c[1], -1) {
			if helpFor(name[1]) == "" {
				t.Errorf("no help for %q: add it to commandHelp (or helpAliases)", name[1])
			}
		}
	}
	for name, text := range commandHelp {
		if !strings.HasPrefix(text, "Usage: whatsapp-mcp "+name) {
			t.Errorf("help for %q does not start with its usage line", name)
		}
	}
}

func TestWantsHelp(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"--help"}, true},
		{[]string{"-h"}, true},
		{[]string{"add", "https://x", "-help"}, true},
		{[]string{"set", "-"}, false},
		{[]string{"help"}, false},
		{[]string{"--", "-h"}, false},
		{nil, false},
	} {
		if got := wantsHelp(tc.args); got != tc.want {
			t.Errorf("wantsHelp(%q) = %v, want %v", tc.args, got, tc.want)
		}
	}
}
