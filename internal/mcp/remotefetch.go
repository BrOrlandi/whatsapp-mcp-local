package mcp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/netguard"
)

// A tool called by an agent on another computer, through the server mode's
// public address, runs on the server: to that agent the server's files and
// its network are not "this machine". send_media_message then takes no path,
// and its URL may only reach the internet, never the server itself, its
// network or a cloud's metadata service. Called locally, nothing changes.

// fromRemote reports whether a tool call came from outside this computer.
func fromRemote(ctx context.Context) bool { return caller.From(ctx).Kind != caller.Local }

var errRemotePath = errors.New("from another computer, a file is sent by its https URL: paths on the server are not reachable")

var errRemoteHTTPS = errors.New("from another computer, a file is sent by its https URL: http is not accepted")

// remoteClient downloads for an agent outside: every connection checked,
// at most 5 redirects, all of them https. The call's context bounds the
// time.
var remoteClient = httpsOnly(netguard.Client(10*time.Minute, 5))

// httpsOnly refuses a redirect to anything but https.
func httpsOnly(c *http.Client) *http.Client {
	next := c.CheckRedirect
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return errRemoteHTTPS
		}
		if next != nil {
			return next(req, via)
		}
		return nil
	}
	return c
}

// downloadClient is the client a send's URL is fetched with.
func downloadClient(ctx context.Context) *http.Client {
	if fromRemote(ctx) {
		return remoteClient
	}
	return http.DefaultClient
}

// baseFor is the address links and texts give a caller: the public address
// for one on it, this computer's otherwise.
func (s *Server) baseFor(ctx context.Context) string {
	if o := caller.From(ctx).Origin; o != "" {
		return o
	}
	return s.base()
}
