package daemon

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// A wrong port or a key without a name comes back inside its own dialog,
// open again, not at the top of the page; routes and fields are the same.
func TestDialogFormsShowTheirErrorInside(t *testing.T) {
	d := startServerMode(t, true)

	// Trocar a porta, on this computer.
	own := "http://127.0.0.1:" + strconv.Itoa(d.Port())
	form := map[string]string{"Origin": own, "Sec-Fetch-Site": "same-origin", "Content-Type": "application/x-www-form-urlencoded"}
	rec := local(d, "POST", "/configuracoes/porta", url.Values{"port": {"80"}}.Encode(), form)
	to := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.HasSuffix(to, "#porta-dialog") || !strings.Contains(to, "erro_em=porta-dialog") {
		t.Fatalf("a wrong port goes back to %d %q", rec.Code, to)
	}
	page := local(d, "GET", strings.TrimSuffix(to, "#porta-dialog"), "", nil).Body.String()
	assertInDialog(t, page, "porta-dialog", "a porta precisa ser um número de 1024 a 65535")

	// Criar uma chave, on the public address.
	h := d.PublicHandler()
	cookie := signedIn(t, h)
	rec = serveAgent(h, outsideRequest("POST", "/conexoes/chaves", url.Values{"name": {"  "}}.Encode(),
		map[string]string{"Cookie": cookie, "Origin": publicURL, "Sec-Fetch-Site": "same-origin", "Content-Type": "application/x-www-form-urlencoded"}))
	to = rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.HasSuffix(to, "#chave-dialog") || !strings.Contains(to, "erro_em=chave-dialog") {
		t.Fatalf("a key without a name goes back to %d %q", rec.Code, to)
	}
	rec = serveAgent(h, outsideRequest("GET", strings.TrimSuffix(to, "#chave-dialog"), "", map[string]string{"Cookie": cookie}))
	assertInDialog(t, rec.Body.String(), "chave-dialog", "Dê um nome à chave")
}

// assertInDialog checks that the error appears once, as an alert inside the
// dialog with the given id.
func assertInDialog(t *testing.T, page, dialog, msg string) {
	t.Helper()
	at := strings.Index(page, `id="`+dialog+`"`)
	alert := strings.Index(page, `<p class="alert" role="alert">`+msg)
	if at < 0 || alert < at || strings.Count(page, msg) != 1 {
		t.Fatalf("the error is not once inside #%s (dialog at %d, alert at %d, %d times)", dialog, at, alert, strings.Count(page, msg))
	}
}
