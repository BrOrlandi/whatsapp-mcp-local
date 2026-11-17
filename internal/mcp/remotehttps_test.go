package mcp

import (
	"net/http"
	"net/url"
	"testing"
)

// A redirect from https to http is refused for an agent outside.
func TestARedirectToHTTPIsRefusedFromOutside(t *testing.T) {
	u, _ := url.Parse("http://example.com/a.png")
	if err := remoteClient.CheckRedirect(&http.Request{URL: u}, nil); err == nil {
		t.Fatal("a redirect to http was followed")
	}
	u, _ = url.Parse("https://example.com/a.png")
	if err := remoteClient.CheckRedirect(&http.Request{URL: u}, nil); err != nil {
		t.Fatalf("a redirect to https: %v", err)
	}
}
