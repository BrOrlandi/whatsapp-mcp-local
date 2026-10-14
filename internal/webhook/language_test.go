package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
)

// The language is the interface's: what a webhook receives is the same
// whatever language the panel shows, the test event included.
func TestEventsDoNotFollowTheLanguage(t *testing.T) {
	defer i18n.Set(i18n.Source)
	ctx := context.Background()
	var payloads []string
	for _, lang := range []string{i18n.Source, "es", "en"} {
		i18n.Set(lang)
		m, _ := manager(t)
		r := newReceiver(t, func(int32) int { return http.StatusNoContent })
		h, secret, err := m.Create(ctx, Settings{URL: ptr(r.srv.URL)})
		if err != nil {
			t.Fatal(err)
		}
		r.secret = secret
		m.Dispatch(ctx, message("A", "1@s.whatsapp.net", false))
		waitFor(t, "the delivery", func() bool { return len(r.events()) == 1 })
		if _, err := m.Test(ctx, h.ID); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "the test delivery", func() bool { return len(r.events()) == 2 })
		var b strings.Builder
		for _, ev := range r.events() {
			msg := *ev.Message
			msg.Timestamp = time.Time{}
			j, _ := json.Marshal(struct {
				Kind    string
				Test    bool
				Message Message
			}{ev.Kind, ev.Test, msg})
			b.Write(j)
		}
		payloads = append(payloads, b.String())
	}
	for i, p := range payloads[1:] {
		if p != payloads[0] {
			t.Errorf("the events changed with the language:\n%s\n%s", payloads[0], payloads[i+1])
		}
	}
	for _, want := range []string{`"chat_name":"Teste"`, `"sender_name":"Teste"`, `"text":"Mensagem de teste do WhatsApp MCP"`} {
		if !strings.Contains(payloads[0], want) {
			t.Errorf("the test event lacks %s: %s", want, payloads[0])
		}
	}
}
