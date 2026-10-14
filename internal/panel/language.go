package panel

import (
	"encoding/json"
	"html/template"
	"net/http"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/appconfig"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
)

// chosenLanguage is the language the person picked, empty when the panel
// follows the system's. The app keeps it; on the command line it lives in
// config.json, in the data folder.
func (p *Panel) chosenLanguage() string {
	if p.Host != nil {
		return p.Host.Settings().Language
	}
	if p.DataDir == "" {
		return ""
	}
	c, err := appconfig.Load(p.DataDir)
	if err != nil {
		return ""
	}
	return c.Language
}

// setLanguage shows the panel in code (a code of i18n.Languages, or empty to
// follow the system's) from the next page on, and keeps it for the next run.
func (p *Panel) setLanguage(code string) error {
	if code != "" && !i18n.Supported(code) {
		return userError{i18n.T("pedido inválido")}
	}
	if p.Host != nil {
		return p.Host.SetLanguage(code)
	}
	if p.DataDir != "" {
		c, err := appconfig.Load(p.DataDir)
		if err != nil {
			// A config.json that does not read is not overwritten with
			// the defaults: the language changes for this run only.
			i18n.Set(code)
			return nil
		}
		c.Language = code
		if err := appconfig.Save(p.DataDir, c); err != nil {
			return err
		}
	}
	i18n.Set(code)
	return nil
}

// apiLanguage is the language buttons of the first screen and the choice in
// Configurações, in the app and on the command line alike.
func (p *Panel) apiLanguage(r *http.Request) (any, error) {
	var body struct {
		Language *string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Language == nil {
		return nil, userError{i18n.T("pedido inválido")}
	}
	if err := p.setLanguage(*body.Language); err != nil {
		return nil, err
	}
	return map[string]string{"language": *body.Language, "lang": i18n.Lang()}, nil
}

// flags are the languages' flags, drawn simply enough to read at 20 by 14
// pixels: emoji flags do not show on Windows.
var flags = map[string]string{
	i18n.Source: `<rect width="20" height="14" fill="#009b3a"/><path d="M10 1.6 18.2 7 10 12.4 1.8 7z" fill="#fedf00"/><circle cx="10" cy="7" r="3.1" fill="#002776"/><path d="M7 6.3c2-.6 4.2-.4 6 .7" stroke="#fff" stroke-width=".7" fill="none"/>`,
	"es":        `<rect width="20" height="14" fill="#aa151b"/><rect y="3.5" width="20" height="7" fill="#f1bf00"/>`,
	"en":        `<rect width="20" height="14" fill="#fff"/><path d="M0 0h20v1.08H0zm0 2.15h20v1.08H0zm0 2.16h20v1.07H0zm0 2.15h20v1.08H0zm0 2.15h20v1.08H0zm0 2.16h20v1.07H0zm0 2.15h20V14H0z" fill="#b22234"/><rect width="8.6" height="7.54" fill="#3c3b6e"/><g fill="#fff"><circle cx="1.5" cy="1.3" r=".45"/><circle cx="4.3" cy="1.3" r=".45"/><circle cx="7.1" cy="1.3" r=".45"/><circle cx="2.9" cy="2.8" r=".45"/><circle cx="5.7" cy="2.8" r=".45"/><circle cx="1.5" cy="4.3" r=".45"/><circle cx="4.3" cy="4.3" r=".45"/><circle cx="7.1" cy="4.3" r=".45"/><circle cx="2.9" cy="5.8" r=".45"/><circle cx="5.7" cy="5.8" r=".45"/></g>`,
}

// flag is a language's flag, beside its name: decoration, hidden from screen
// readers, which read the name.
func flag(code string) template.HTML {
	body, ok := flags[code]
	if !ok {
		return ""
	}
	return template.HTML(`<svg class="flag" viewBox="0 0 20 14" width="20" height="14" aria-hidden="true" focusable="false">` + body + `</svg>`)
}
