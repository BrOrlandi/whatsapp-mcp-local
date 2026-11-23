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
	// ChangelogBaseURL is the site's redirect to the release notes:
	// /changelog lists every version, /changelog/1.9.6 one of them.
	ChangelogBaseURL = SiteURL + "/changelog"
)

// ChangelogURL is the page with what changed in version (1.9.6 or v1.9.6).
// A version that is not a release's, such as a development build's "dev",
// gets the list of every version.
func ChangelogURL(version string) string {
	v := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if !releaseVersion(v) {
		return ChangelogBaseURL
	}
	return ChangelogBaseURL + "/" + v
}

// releaseVersion reports whether v is a release's X.Y.Z.
func releaseVersion(v string) bool {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" {
			return false
		}
	}
	return true
}
