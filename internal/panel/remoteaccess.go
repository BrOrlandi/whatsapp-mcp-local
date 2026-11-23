package panel

import (
	"bytes"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/httpserver"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/public"
)

// The server mode's Conexões page: the agents let in from outside, by an
// OAuth login or an API key, with what each may do, and a way to cut each
// one off. Without the server mode the page does not exist.

type accessItem struct {
	ID, KindKey, Name, Kind, Access, Created, Used string
	Expired                                        bool
}

type accessPage struct {
	layout
	Items      []accessItem
	Address    string
	NewKey     string
	NewKeyName string
}

func (p *Panel) registerRemoteAccess(mux httpserver.Mux) {
	mux.HandleFunc("GET /conexoes", p.page(func(w http.ResponseWriter, r *http.Request) (string, any) {
		if p.PublicURL == "" {
			http.NotFound(w, r)
			return "", nil
		}
		return "conexoes", p.accessPage(r, "", "")
	}))
	// Creating a key answers with the page itself, which shows the key the
	// one time it is ever seen: a redirect would carry it in an address.
	mux.HandleFunc("POST /conexoes/chaves", func(w http.ResponseWriter, r *http.Request) {
		if callerRefused(w, r) {
			return
		}
		if !p.fromOwnPage(r) || p.PublicURL == "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
		key, k, err := public.CreateKey(r.Context(), p.State, r.PostFormValue("name"), r.PostFormValue("read_only") != "", time.Now())
		if err != nil {
			msg := err.Error()
			if errors.Is(err, public.ErrKeyName) {
				msg = i18n.T("Dê um nome à chave, com até 60 letras, como o da ferramenta que vai usá-la.")
			}
			http.Redirect(w, r, withError("/conexoes#chave-dialog", msg), http.StatusSeeOther)
			return
		}
		var body bytes.Buffer
		if err := p.pages.ExecuteTemplate(&body, "conexoes", p.accessPage(r, key, k.Name)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		h := w.Header()
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Cache-Control", "no-store")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin") // a form posted from a no-referrer page sends Origin: null
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; frame-ancestors 'none'")
		_, _ = w.Write(body.Bytes())
	})
	mux.HandleFunc("POST /conexoes/revogar", p.form(func(r *http.Request) (string, error) {
		if p.PublicURL == "" {
			return "/", userError{i18n.T("o modo servidor não está ligado")}
		}
		id, now := r.PostFormValue("id"), time.Now()
		var err error
		switch r.PostFormValue("kind") {
		case "key":
			err = public.RevokeKey(r.Context(), p.State, id, now)
		case "login":
			err = p.State.RevokeConnection(r.Context(), id, now)
		default:
			return "/conexoes", userError{i18n.T("pedido inválido")}
		}
		if err != nil {
			return "/conexoes", userError{i18n.T("esta conexão já não existe")}
		}
		return "/conexoes?ok=" + url.QueryEscape(i18n.T("Acesso cortado.")), nil
	}))
}

func (p *Panel) accessPage(r *http.Request, newKey, newKeyName string) accessPage {
	s := p.snapshot(r.Context())
	pg := accessPage{layout: p.layout(r, i18n.N("Conexões"), "conexoes", s), Address: p.PublicURL + "/mcp",
		NewKey: newKey, NewKeyName: newKeyName}
	ctx, now := r.Context(), time.Now()
	used := func(t *time.Time) string {
		if t == nil {
			return i18n.T("ainda não usada")
		}
		return i18n.Tf("usada %s", relativeSince(*t))
	}
	access := func(readOnly bool) string {
		if readOnly {
			return i18n.T("só ler")
		}
		return i18n.T("tudo")
	}
	list := p.State.Connections
	if p.Logins != nil {
		list = p.Logins
	}
	if logins, err := list(ctx); err == nil {
		for _, c := range logins {
			pg.Items = append(pg.Items, accessItem{ID: c.ID, KindKey: "login", Name: c.ClientName, Kind: i18n.T("login"),
				Access: access(c.ReadOnly), Created: i18n.Tf("entrou %s", relativeSince(c.CreatedAt)), Used: used(c.LastUsedAt)})
		}
	}
	if keys, err := public.Keys(ctx, p.State, now); err == nil {
		for _, k := range keys {
			item := accessItem{ID: k.ID, KindKey: "key", Name: k.Name, Kind: i18n.T("chave"), Access: access(k.ReadOnly),
				Created: i18n.Tf("criada %s", relativeSince(k.CreatedAt)), Used: used(k.LastUsedAt), Expired: k.Expired}
			switch {
			case k.Expired:
				item.Used = i18n.T("ninguém usou nas primeiras 24 horas, e ela deixou de valer")
			case k.ExpiresAt != nil:
				item.Used = i18n.Tf("ainda não usada: deixa de valer %s se ninguém usar", strings.TrimSpace(momentAny(*k.ExpiresAt)))
			}
			pg.Items = append(pg.Items, item)
		}
	}
	return pg
}
