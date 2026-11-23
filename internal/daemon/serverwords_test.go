package daemon

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// The server mode's pages talk about "this server": the texts that say "this
// computer" belong to the desktop app and to the local listener.
func TestServerPagesSayServer(t *testing.T) {
	d := startServerMode(t, true)
	h := d.PublicHandler()
	cookie := signedIn(t, h)
	computer := regexp.MustCompile(`(?i)\b(este|neste|deste|o) computador\b`)
	for _, path := range []string{"/instalacao", "/whatsapp", "/status", "/ajuda", "/receitas", "/conectar"} {
		rec := serveAgent(h, outsideRequest("GET", path, "", map[string]string{"Cookie": cookie}))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: %d", path, rec.Code)
			continue
		}
		page := rec.Body.String()
		for _, at := range computer.FindAllStringIndex(page, -1) {
			from, to := max(at[0]-120, 0), min(at[1]+40, len(page))
			t.Errorf("%s says %q on the server: …%s…", path, page[at[0]:at[1]], page[from:to])
		}
	}

	// The same pages on this computer keep their words.
	page := local(d, "GET", "/whatsapp", "", nil).Body.String()
	if !strings.Contains(page, "Desconectar este computador do WhatsApp") {
		t.Error("the local listener's WhatsApp page lost \"Desconectar este computador do WhatsApp\"")
	}
}
