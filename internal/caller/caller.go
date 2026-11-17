// Package caller says who makes each request and what it may do.
//
// Every request carries a Caller in its context. Today the only one that
// exists at runtime is Local: a program on this computer, past the loopback
// guard and the optional token, or the app's own window. It may do
// everything, so the checks below always answer yes for it. The other kinds
// are the server mode's (docs/plano-servidor.md, section 2a): the panel's
// session, an OAuth login and an API key, each reaching only its part.
//
// The checks sit in four places: the panel's page, form and api wrappers,
// the /mcp endpoint, the tools list and call, and the few routes registered
// outside the wrappers.
package caller

import (
	"context"
	"slices"
)

// Kind is where a caller comes from.
type Kind string

const (
	// Local is a program on this computer, or the app's window.
	Local Kind = "local"
	// Panel is someone signed in to the panel over the public listener.
	Panel Kind = "panel"
	// OAuth is an agent with a valid OAuth access token.
	OAuth Kind = "oauth"
	// APIKey is an agent with a valid API key.
	APIKey Kind = "apikey"
)

// Access is the part of the server a route belongs to.
type Access string

const (
	// Open routes carry no data: static files, icons, liveness, redirects.
	Open Access = "open"
	// PanelAccess is the panel's pages, forms and JSON API, and the files
	// it serves (the pairing QR code, the support log).
	PanelAccess Access = "panel"
	// MCP is the /mcp endpoint.
	MCP Access = "mcp"
	// Media is the temporary links download_media hands out.
	Media Access = "media"
	// Health is the health report.
	Health Access = "health"
)

// Rules are the paid version's limits on a connection: which tools it may
// use. The public code only consults them.
type Rules interface {
	AllowTool(name, category string) bool
}

// Caller is who makes a request.
type Caller struct {
	Kind Kind
	// ConnectionID and ConnectionName identify the login or key behind an
	// OAuth or API key caller, for the connections list and the support log.
	ConnectionID   string
	ConnectionName string
	// Client is the transport's hint for the MCP client: the bridge's
	// X-MCP-Client header, or "ua:" and an HTTP client's User-Agent.
	Client string
	// Window is set for the app's own window, which reaches the routes in
	// memory rather than over the network.
	Window bool
	// Categories are the tool categories the caller may use; nil allows
	// every one.
	Categories []string
	// Rules are the paid version's limits, or nil.
	Rules Rules
}

type key struct{}

// With returns ctx carrying c.
func With(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, key{}, c)
}

// From is the caller ctx carries. A context without one is Local: every way
// into the server today is on this computer (the loopback listener, the app's
// window, and code that calls the handlers directly), and a listener that
// faces the network must set its caller before anything else runs.
func From(ctx context.Context) Caller {
	if c, ok := ctx.Value(key{}).(Caller); ok {
		return c
	}
	return Caller{Kind: Local}
}

// May reports whether the caller reaches routes of the given access.
func (c Caller) May(a Access) bool {
	switch a {
	case Open:
		return true
	case PanelAccess:
		return c.Kind == Local || c.Kind == Panel
	case MCP:
		return c.Kind == Local || c.Kind == OAuth || c.Kind == APIKey
	case Media, Health:
		return c.Kind == Local || c.Kind == Panel || c.Kind == OAuth || c.Kind == APIKey
	}
	return false
}

// MayUseTool reports whether the caller may list and call a tool of the
// given category.
func (c Caller) MayUseTool(name, category string) bool {
	if c.Categories != nil && !slices.Contains(c.Categories, category) {
		return false
	}
	return c.Rules == nil || c.Rules.AllowTool(name, category)
}

// ReadOnly reports whether the caller may only read.
func (c Caller) ReadOnly() bool {
	return c.Categories != nil && !slices.ContainsFunc(c.Categories, func(cat string) bool { return cat != ReadCategory })
}

// ReadCategory is the tool category that only reads; it matches
// mcp.CategoryRead.
const ReadCategory = "read"

// ReadOnlyCategories is what a read-only connection may use.
func ReadOnlyCategories() []string { return []string{ReadCategory} }
