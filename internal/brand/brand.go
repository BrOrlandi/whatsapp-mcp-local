// Package brand holds the WhatsApp MCP visual identity, shared with the hosted
// v1 so both panels look like the same product. Every asset is embedded, so
// the panel depends on no external image or CDN.
package brand

import (
	_ "embed"
	"html/template"
	"strings"
)

//go:embed logo.svg
var logo string

// LogoSVG returns the logo mark ready to be inlined in an HTML document. It is
// trusted markup shipped with the binary, never user input.
func LogoSVG() template.HTML { return template.HTML(strings.TrimSpace(logo)) }

//go:embed favicon.svg
var faviconSVG []byte

//go:embed favicon.ico
var faviconICO []byte

//go:embed apple-touch-icon.png
var appleTouchIcon []byte

func FaviconSVG() []byte     { return faviconSVG }
func FaviconICO() []byte     { return faviconICO }
func AppleTouchIcon() []byte { return appleTouchIcon }

// The project's authorship and the links that go with it, the same as v1's.
const (
	Name          = "WhatsApp MCP"
	Author        = "Bruno Orlandi"
	AuthorURL     = "https://github.com/BrOrlandi"
	RepositoryURL = "https://github.com/BrOrlandi/whatsapp-mcp-v2"
	SupportURL    = "https://donate.stripe.com/8x200jdhA6c1d375jF9Ve06"
)
