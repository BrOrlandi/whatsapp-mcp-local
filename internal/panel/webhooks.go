package panel

import (
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"strings"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/webhook"
)

// The webhooks and the downloaded media are configured through the local
// API: a script that wants to hear about new messages registers itself, and
// gets the secret its deliveries are signed with. docs/api.md describes it.

// RegisterWebhooks adds the routes for webhooks and downloaded media.
// available is false when the wacli in use cannot post new messages.
func (p *Panel) RegisterWebhooks(mux *http.ServeMux, hooks *webhook.Manager, available bool) {
	// The documentation page joins the templates Register parsed.
	template.Must(p.pages.Parse(webhookDocsSource))
	mux.HandleFunc("GET /webhooks/documentacao", p.page(p.webhookDocs))

	mux.HandleFunc("GET /api/webhooks", p.api(func(r *http.Request) (any, error) {
		list, err := hooks.List(r.Context())
		if list == nil {
			list = []webhook.Status{}
		}
		return map[string]any{"available": available, "webhooks": list, "events": webhook.Events,
			"signature": "X-WhatsApp-MCP-Signature: sha256=<HMAC-SHA256 do corpo com o segredo do webhook, em hexadecimal>"}, err
	}))
	mux.HandleFunc("POST /api/webhooks", p.api(func(r *http.Request) (any, error) {
		var s webhook.Settings
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			return nil, userError{"pedido inválido"}
		}
		h, secret, err := hooks.Create(r.Context(), s)
		if err != nil {
			return nil, webhookError(err)
		}
		st, _ := hooks.Get(r.Context(), h.ID)
		out := map[string]any{"webhook": st, "secret": secret,
			"note": "guarde o segredo: ele não aparece de novo, e assina cada entrega no cabeçalho X-WhatsApp-MCP-Signature"}
		if !available {
			out["warning"] = "o wacli em uso não entrega mensagens novas a webhooks; atualize-o para que este webhook receba algo"
		}
		return out, nil
	}))
	mux.HandleFunc("GET /api/webhooks/{id}", p.api(func(r *http.Request) (any, error) {
		st, err := hooks.Get(r.Context(), r.PathValue("id"))
		return st, webhookError(err)
	}))
	mux.HandleFunc("PATCH /api/webhooks/{id}", p.api(func(r *http.Request) (any, error) {
		var s webhook.Settings
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			return nil, userError{"pedido inválido"}
		}
		if _, err := hooks.Update(r.Context(), r.PathValue("id"), s); err != nil {
			return nil, webhookError(err)
		}
		st, err := hooks.Get(r.Context(), r.PathValue("id"))
		return st, webhookError(err)
	}))
	mux.HandleFunc("DELETE /api/webhooks/{id}", p.api(func(r *http.Request) (any, error) {
		if err := hooks.Delete(r.Context(), r.PathValue("id")); err != nil {
			return nil, webhookError(err)
		}
		return map[string]bool{"deleted": true}, nil
	}))
	mux.HandleFunc("POST /api/webhooks/{id}/test", p.api(func(r *http.Request) (any, error) {
		out, err := hooks.Test(r.Context(), r.PathValue("id"))
		return out, webhookError(err)
	}))

	mux.HandleFunc("GET /api/media", p.api(func(r *http.Request) (any, error) {
		return p.Server.Inventory(r.Context())
	}))
	mux.HandleFunc("POST /api/media/retention", p.api(func(r *http.Request) (any, error) {
		var body struct {
			Days *int `json:"days"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Days == nil || *body.Days < 0 {
			return nil, userError{`mande {"days": N}: os dias que um arquivo baixado fica guardado, ou 0 para guardar sempre`}
		}
		if err := p.Server.SetRetention(r.Context(), *body.Days); err != nil {
			return nil, err
		}
		files, bytes, err := p.Server.SweepMedia(r.Context())
		return map[string]any{"retention_days": *body.Days, "deleted_files": files, "freed_bytes": bytes}, err
	}))
	mux.HandleFunc("POST /api/media/purge", p.api(func(r *http.Request) (any, error) {
		var body struct {
			What          string `json:"what"`
			OlderThanDays int    `json:"older_than_days"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.OlderThanDays < 0 {
			return nil, userError{`mande {"what": "media"} ou {"what": "exports"}`}
		}
		var files int
		var bytes int64
		var err error
		switch body.What {
		case "media":
			files, bytes, _, err = p.Server.PurgeMedia("", body.OlderThanDays, 0, false)
		case "exports":
			files, bytes, err = p.Server.PurgeExports()
		default:
			return nil, userError{`what precisa ser "media" (os arquivos baixados) ou "exports" (as exportações)`}
		}
		return map[string]any{"deleted_files": files, "freed_bytes": bytes}, err
	}))
}

// webhookError turns the webhook package's errors into the API's: what the
// caller can fix is a 409, in Portuguese.
func webhookError(err error) error {
	var ue webhook.UserError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &ue):
		return userError{ue.Msg}
	case errors.Is(err, state.ErrNoWebhook):
		return userError{"nenhum webhook tem esse id"}
	}
	return errors.New(strings.TrimSpace(err.Error()))
}
