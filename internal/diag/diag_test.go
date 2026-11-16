package diag

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testAnon() *Anonymizer { return NewAnonymizer([]byte("chave-de-teste-123456"), "/Users/fulano") }

func TestTextHidesWhatIdentifiesAPerson(t *testing.T) {
	a := testAnon()
	in := "could not read message 3EB0C431D5A2F1E9B7 in 120363025246125486@g.us from 5511987654321:12@s.whatsapp.net " +
		"and 198765432109876@lid, ligue +55 11 98765-4321 ou 5511987654321, fulano@example.com, " +
		"https://hooks.example.com/abc?x=1, http://127.0.0.1:47821/mcp, /Users/fulano/Library/x, /Users/outro/y"
	out := a.Text(in)
	for _, leak := range []string{"3EB0C431D5A2F1E9B7", "120363025246125486", "5511987654321", "198765432109876",
		"98765-4321", "fulano@example.com", "hooks.example.com", "/Users/fulano", "/Users/outro"} {
		if strings.Contains(out, leak) {
			t.Errorf("%q leaked into %q", leak, out)
		}
	}
	for _, kept := range []string{"could not read message", "group#", "user#", ":12", "lid#", "num#", "email#", "url#",
		"http://127.0.0.1:47821/mcp", "~/Library/x", "/Users/<user>/y"} {
		if !strings.Contains(out, kept) {
			t.Errorf("%q missing from %q", kept, out)
		}
	}
	// The same value always gets the same code, the plain and formatted
	// phone number included.
	if a.Text("5511987654321") != a.Text("+55 11 98765-4321") {
		t.Errorf("phone forms got different codes: %q %q", a.Text("5511987654321"), a.Text("+55 11 98765-4321"))
	}
	if a.Text("5511987654321@s.whatsapp.net") != a.Text("5511987654321@s.whatsapp.net") {
		t.Error("codes are not stable")
	}
	other := NewAnonymizer([]byte("outra-chave-qualquer-99"), "")
	if other.Text("5511987654321@s.whatsapp.net") == a.Text("5511987654321@s.whatsapp.net") {
		t.Error("codes do not depend on the key")
	}
}

func TestValueOmitsTextAndNames(t *testing.T) {
	a := testAnon()
	got := a.Value("data", map[string]any{
		"text": "oi, tudo bem?", "push_name": "Maria", "chat_jid": "5511987654321@s.whatsapp.net",
		"msg_id": "ABCDEF", "count": 3.0, "nested": map[string]any{"caption": "foto", "ok": true},
	}).(map[string]any)
	if got["text"] != "<omitted>" || got["push_name"] != "<omitted>" {
		t.Errorf("text kept: %v", got)
	}
	if s := got["chat_jid"].(string); !strings.HasPrefix(s, "user#") {
		t.Errorf("chat_jid = %q", s)
	}
	if s := got["msg_id"].(string); !strings.HasPrefix(s, "msg#") {
		t.Errorf("msg_id = %q", s)
	}
	if got["count"] != 3.0 {
		t.Errorf("count = %v", got["count"])
	}
	if n := got["nested"].(map[string]any); n["caption"] != "<omitted>" || n["ok"] != true {
		t.Errorf("nested = %v", n)
	}
}

func TestLogWritesExportsAndPrunes(t *testing.T) {
	dir := t.TempDir()
	var mu sync.Mutex
	clock := time.Date(2026, 11, 16, 10, 0, 0, 0, time.Local)
	advance := func(d time.Duration) { mu.Lock(); clock = clock.Add(d); mu.Unlock() }
	l, err := open(filepath.Join(dir, "diag"), dir, func() time.Time { mu.Lock(); defer mu.Unlock(); return clock })
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	// An old file is pruned when the day changes.
	old := filepath.Join(l.dir, filePrefix+"2026-11-01"+fileSuffix)
	if err := os.WriteFile(old, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	l.Event("wacli", "warning", Fields{"code": "undecryptable_message", "chat_jid": "5511987654321@s.whatsapp.net", "text": "segredo"})
	advance(2 * time.Hour)
	l.Count("presence")
	l.Count("presence")
	l.Event("mcp", "tool_call", Fields{"tool": "list_chats", "ms": 12})
	advance(24 * time.Hour)
	l.Event("sync", "state", Fields{"from": "connected", "to": "reconnecting"})
	time.Sleep(50 * time.Millisecond) // the prune runs on its own

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old file kept: %v", err)
	}

	var buf bytes.Buffer
	n, err := l.WriteSince(&buf, time.Date(2026, 11, 16, 11, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "undecryptable_message") {
		t.Errorf("line before since exported:\n%s", out)
	}
	for _, want := range []string{"mcp tool_call ms=12 tool=list_chats", "sync state from=connected to=reconnecting", "app counts", "presence=2"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q missing:\n%s", want, out)
		}
	}
	if n != 3 {
		t.Errorf("n = %d, want 3:\n%s", n, out)
	}

	day, _ := os.ReadFile(filepath.Join(l.dir, filePrefix+"2026-11-16"+fileSuffix))
	if strings.Contains(string(day), "segredo") || strings.Contains(string(day), "5511987654321") {
		t.Errorf("file holds plain data:\n%s", day)
	}
}

func TestNilLogIsQuiet(t *testing.T) {
	var l *Log
	l.Event("x", "y", nil)
	l.Count("z")
	l.FlushCounts()
	l.Close()
	if n, err := l.WriteSince(&bytes.Buffer{}, time.Now()); n != 0 || err != nil {
		t.Fatal(n, err)
	}
}
