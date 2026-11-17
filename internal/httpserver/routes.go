package httpserver

import "github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"

// routeAccess classifies every route the server registers, by its pattern:
// which callers reach it (caller.Caller.May). A route missing here fails
// TestEveryRouteIsClassified, rather than being born open on a listener that
// faces the network.
//
// LocalOnly routes are the app's own (quit, erase, port, updates, open a
// folder or the window) and writing an AI tool's configuration: they act on
// the computer the server runs on, so only the local caller reaches them.
//
// The panel's pages, forms and API check the caller in their wrappers
// (panel.page, panel.form, panel.api); the routes outside them check it
// themselves.
var routeAccess = map[string]caller.Access{
	// The MCP and what it hands out.
	"/mcp":    caller.MCP,
	"/media/": caller.Media,
	"/health": caller.Health,
	// Liveness, static files and icons carry no data.
	"/healthz":                  caller.Open,
	"GET /assets/i18n.js":       caller.Open,
	"GET /assets/":              caller.Open,
	"GET /favicon.svg":          caller.Open,
	"GET /favicon.ico":          caller.Open,
	"GET /apple-touch-icon.png": caller.Open,
	// The pages' former addresses only redirect to the new ones.
	"GET /estado":       caller.Open,
	"GET /documentacao": caller.Open,
	"GET /transcricao":  caller.Open,

	// Pages.
	"GET /{$}":                   caller.PanelAccess,
	"GET /instalacao":            caller.PanelAccess,
	"GET /conectar":              caller.PanelAccess,
	"GET /conectar/{tool}":       caller.PanelAccess,
	"GET /whatsapp":              caller.PanelAccess,
	"GET /status":                caller.PanelAccess,
	"GET /funcoes":               caller.PanelAccess,
	"GET /receitas":              caller.PanelAccess,
	"GET /ajuda":                 caller.PanelAccess,
	"GET /configuracoes":         caller.PanelAccess,
	"GET /webhooks/documentacao": caller.PanelAccess,

	// Forms.
	"POST /instalacao/avancar":     caller.PanelAccess,
	"POST /whatsapp/sair":          caller.PanelAccess,
	"POST /whatsapp/nome":          caller.PanelAccess,
	"POST /conexoes/remover":       caller.PanelAccess,
	"POST /conexoes/renomear":      caller.PanelAccess,
	"POST /configuracoes/porta":    caller.LocalOnly,
	"POST /configuracoes/encerrar": caller.LocalOnly,
	"POST /configuracoes/apagar":   caller.LocalOnly,

	// The panel's files.
	"GET /api/pair/qr.png":      caller.PanelAccess,
	"GET /diagnostico/logs.txt": caller.PanelAccess,

	// The JSON API.
	"GET /api/status":                 caller.PanelAccess,
	"GET /api/state":                  caller.PanelAccess,
	"GET /api/pulse":                  caller.PanelAccess,
	"GET /api/chats":                  caller.PanelAccess,
	"GET /api/chats/{jid}":            caller.PanelAccess,
	"POST /api/history":               caller.PanelAccess,
	"POST /api/pair":                  caller.PanelAccess,
	"POST /api/pair/cancel":           caller.PanelAccess,
	"POST /api/logout":                caller.PanelAccess,
	"POST /api/clients/{client}":      caller.LocalOnly,
	"POST /api/idioma":                caller.PanelAccess,
	"GET /api/asr":                    caller.PanelAccess,
	"POST /api/asr/install":           caller.PanelAccess,
	"POST /api/diagnostico":           caller.PanelAccess,
	"GET /api/webhooks":               caller.PanelAccess,
	"POST /api/webhooks":              caller.PanelAccess,
	"GET /api/webhooks/{id}":          caller.PanelAccess,
	"PATCH /api/webhooks/{id}":        caller.PanelAccess,
	"DELETE /api/webhooks/{id}":       caller.PanelAccess,
	"POST /api/webhooks/{id}/test":    caller.PanelAccess,
	"GET /api/media":                  caller.PanelAccess,
	"POST /api/media/retention":       caller.PanelAccess,
	"POST /api/media/purge":           caller.PanelAccess,
	"POST /api/configuracoes":         caller.LocalOnly,
	"POST /api/abrir":                 caller.LocalOnly,
	"POST /api/app/open":              caller.LocalOnly,
	"POST /api/abrir-pasta":           caller.LocalOnly,
	"GET /api/atualizacao":            caller.LocalOnly,
	"POST /api/atualizacao/verificar": caller.LocalOnly,
	"POST /api/atualizacao/instalar":  caller.LocalOnly,
}

// RouteAccess is how a registered pattern is classified; ok is false for a
// pattern routeAccess does not list.
func RouteAccess(pattern string) (a caller.Access, ok bool) {
	a, ok = routeAccess[pattern]
	return a, ok
}

// ClassifiedPatterns are every pattern routeAccess lists.
func ClassifiedPatterns() []string {
	out := make([]string, 0, len(routeAccess))
	for p := range routeAccess {
		out = append(out, p)
	}
	return out
}
