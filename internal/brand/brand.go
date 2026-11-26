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

// LogoMarkSVG is the logo in one colour, the text's (currentColor), and
// hidden from screen readers: the site's header mark, for a page that writes
// "WhatsApp MCP" beside it on the dark green, where the gradient's darker end
// would vanish.
func LogoMarkSVG() template.HTML {
	s := strings.ReplaceAll(strings.TrimSpace(logo), "url(#wamcp-mark)", "currentColor")
	s = strings.Replace(s, `role="img"`, `aria-hidden="true" focusable="false"`, 1)
	s = strings.Replace(s, "<title>WhatsApp MCP</title>", "", 1)
	return template.HTML(s)
}

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
	RepositoryURL = "https://github.com/BrOrlandi/whatsapp-mcp-local"
	SupportURL    = "https://donate.stripe.com/8x200jdhA6c1d375jF9Ve06"
	// SupportEmail is where people send the logs from Configurações › Logs
	// para suporte (SupportURL above is the donation link, not this). Empty,
	// the logs dialog offers only the download, with no address to send to.
	SupportEmail = "contato@whatsappmcp.com.br"
	// SiteURL is the project's landing page. The apps link to it, never to
	// GitHub, for anything a person reads: when the repository changes name,
	// only the site's redirects change.
	SiteURL = "https://whatsappmcp.com.br"
	// ChangelogURL is the site's redirect to the release notes of every
	// version.
	ChangelogURL = SiteURL + "/changelog"
)
