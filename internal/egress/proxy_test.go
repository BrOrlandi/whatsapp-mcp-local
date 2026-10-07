package egress

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	for _, value := range []string{"", "direct", "http://proxy.example:8080", "https://proxy.example:8443", "socks5://100.101.102.103:1080", "socks5h://user:p%40ss@[fd7a:115c:a1e0::1]:1080"} {
		if _, err := Parse(value); err != nil {
			t.Errorf("valid proxy %q: %v", value, err)
		}
	}
	for _, value := range []string{"100.101.102.103", "socks5://host", "http://:1080", "http://host:0", "http://host:65536", "http://host:abc", "ftp://host:1080", "http://host:1080/path", "http://host:1080?", "http://host:1080?token=x", "http://host:1080#x", "http://host:1080\nHTTPS_PROXY=evil", "http://user:private-password@host:bad"} {
		if _, err := Parse(value); err == nil {
			t.Errorf("accepted invalid proxy %q", value)
		} else if strings.Contains(err.Error(), "private-password") {
			t.Fatal("validation exposed a password")
		}
	}
}

func TestExplicitProxyReplacesBypassesAndDirectClearsInheritedProxies(t *testing.T) {
	base := []string{"PATH=/bin", "HTTPS_PROXY=http://old:8080", "https_proxy=http://lower:8080", "ALL_PROXY=socks5://old:1080", "no_proxy=*", "NO_PROXY=whatsapp.com"}
	proxy, _ := Parse("socks5h://proxy.example:1080")
	for _, mode := range []Proxy{proxy, "direct"} {
		got := strings.Join(mode.Environment(base), "\n")
		if strings.Contains(got, "old") || strings.Contains(got, "lower") || strings.Contains(got, "no_proxy=*") || strings.Contains(got, "NO_PROXY=whatsapp.com") {
			t.Fatalf("%s kept an inherited proxy or bypass", mode)
		}
		if !strings.Contains(got, "PATH=/bin") || !strings.Contains(got, "NO_PROXY=localhost,127.0.0.1,::1") {
			t.Fatalf("%s lost the process environment or loopback bypass", mode)
		}
		if mode == "direct" && strings.Contains(strings.ToUpper(got), "HTTPS_PROXY=") {
			t.Fatal("direct mode still uses a proxy")
		}
	}
	if got := strings.Join(Proxy("").Environment(base), "\n"); got != strings.Join(base, "\n") {
		t.Fatal("default mode must preserve the inherited environment")
	}
}
