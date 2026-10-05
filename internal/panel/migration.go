package panel

import (
	"bytes"
	"net/http"
	"net/url"
)

// MigrationOffer is an earlier installation the app found on this computer,
// still paired with WhatsApp.
type MigrationOffer struct {
	Name     string
	Phone    string
	StoreDir string
	DataDir  string
	// Service is set when the earlier installation runs as a login service,
	// which the move stops and removes.
	Service bool
}

// MigrationHandler is what the app's window shows before anything else runs,
// when it finds an earlier installation: keep the same WhatsApp connection,
// or start afresh. decide does the work and returns once the panel proper is
// ready to take over at /.
func MigrationHandler(offer MigrationOffer, decide func(keep bool) error) http.Handler {
	pages := parseTemplates()
	render := func(w http.ResponseWriter, r *http.Request, errMsg string) {
		var body bytes.Buffer
		data := struct {
			layout
			Offer MigrationOffer
		}{layout{Title: "Instalação anterior", App: true, Error: errMsg}, offer}
		data.Offer.Phone = formatPhone(offer.Phone)
		if err := pages.ExecuteTemplate(&body, "migracao", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(body.Bytes())
	}
	mux := http.NewServeMux()
	registerAssets(mux)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) { render(w, r, r.URL.Query().Get("erro")) })
	mux.HandleFunc("POST /migracao", func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if err := decide(r.FormValue("escolha") == "manter"); err != nil {
			http.Redirect(w, r, "/?erro="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	return Internal(mux)
}
