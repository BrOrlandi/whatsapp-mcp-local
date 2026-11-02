package wacli

import (
	"errors"
	"testing"
)

func TestDeviceLabel(t *testing.T) {
	cases := []struct{ os, host, want string }{
		{"macOS", "Brunos-MacBook", "WhatsApp MCP - macOS (Brunos-MacBook)"},
		{"Windows", "", "WhatsApp MCP - Windows"},
		{"", "box", "WhatsApp MCP - box"},
		{"", "", "WhatsApp MCP"},
	}
	for _, c := range cases {
		if got := deviceLabel(c.os, c.host); got != c.want {
			t.Errorf("deviceLabel(%q, %q) = %q, want %q", c.os, c.host, got, c.want)
		}
	}
}

func TestOSName(t *testing.T) {
	release := func(string) ([]byte, error) { return []byte("NAME=Ubuntu\nPRETTY_NAME=\"Ubuntu 24.04 LTS\"\n"), nil }
	missing := func(string) ([]byte, error) { return nil, errors.New("no") }
	if got := osName("linux", release); got != "Ubuntu 24.04 LTS" {
		t.Errorf("linux with os-release = %q", got)
	}
	if got := osName("linux", missing); got != "Linux" {
		t.Errorf("linux without os-release = %q", got)
	}
	if got := osName("windows", missing); got != "Windows" {
		t.Errorf("windows = %q", got)
	}
}
