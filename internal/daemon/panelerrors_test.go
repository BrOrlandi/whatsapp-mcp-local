package daemon

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// What a dialog shows is a sentence for a person, saying what to do; a
// program calling the same API keeps the words that name the field.
func TestPanelDialogsSayWhatToDo(t *testing.T) {
	d := startServerMode(t, true)
	own := "http://127.0.0.1:" + strconv.Itoa(d.Port())
	page := map[string]string{"Origin": own, "Sec-Fetch-Site": "same-origin", "Content-Type": "application/json"}
	errorOf := func(t *testing.T, path, body string, header map[string]string) string {
		t.Helper()
		rec := local(d, "POST", path, body, header)
		if rec.Code != http.StatusConflict {
			t.Fatalf("%s %s: %d %s, want 409", path, body, rec.Code, rec.Body)
		}
		var out struct{ Error string }
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out.Error
	}

	hook := `{"url":"nope","events":["message"]}`
	if got := errorOf(t, "/api/webhooks", hook, page); got != "Escreva o endereço completo, começando com http:// ou https://." {
		t.Errorf("the webhook dialog says %q", got)
	}
	if got := errorOf(t, "/api/webhooks", hook, map[string]string{"Content-Type": "application/json"}); !strings.HasPrefix(got, "url precisa ser") {
		t.Errorf("a program is told %q, want the API's words", got)
	}

	for body, want := range map[string]string{
		`{"mode":"openai"}`:                  "Cole a chave da API da OpenAI no campo Chave da API.",
		`{"mode":"remote","url":""}`:         "Escreva o endereço do servidor de transcrição.",
		`{"mode":"remote","url":"ftp://x/"}`: "Escreva o endereço completo do servidor, como https://transcricao.exemplo.com.",
	} {
		if got := errorOf(t, "/api/asr/remote", body, page); got != want {
			t.Errorf("%s: the dialog says %q, want %q", body, got, want)
		}
	}
}

// Mudar o contexto saves through the API too, and answers with what the card
// shows, so the page changes it in place.
func TestContextSavesWithoutReloading(t *testing.T) {
	d := startServerMode(t, true)
	own := "http://127.0.0.1:" + strconv.Itoa(d.Port())
	page := map[string]string{"Origin": own, "Sec-Fetch-Site": "same-origin", "Content-Type": "application/json"}
	rec := local(d, "POST", "/api/asr/contexto", `{"context":"messages"}`, page)
	if rec.Code != http.StatusOK {
		t.Fatalf("saving the context: %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Level, Line, Where string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Level != "messages" || out.Line == "" || out.Where == "" {
		t.Errorf("the answer is %s (%v), want the level, the line and where it goes", rec.Body, err)
	}
	if rec := local(d, "POST", "/api/asr/contexto", `{"context":"tudo"}`, page); rec.Code != http.StatusConflict {
		t.Errorf("an unknown level: %d, want 409", rec.Code)
	}
}
