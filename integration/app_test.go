//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestApp runs the whole suite in order: what it sends in one step is read
// back in the next, and the restart comes last.
func TestApp(t *testing.T) {
	e := loadEnv(t)
	a := newApp(t, e)
	hooks := startReceiver(t, e)

	t.Run("http", func(t *testing.T) { testHTTP(t, newApp(t, e)) })
	t.Run("mcp_protocol", func(t *testing.T) { testProtocol(t, newApp(t, e)) })
	t.Run("read_tools", func(t *testing.T) { testReadTools(t, newApp(t, e)) })
	t.Run("webhooks_api", func(t *testing.T) { testWebhooksAPI(t, newApp(t, e), hooks) })
	t.Run("messaging", func(t *testing.T) { testMessaging(t, newApp(t, e), hooks) })
	t.Run("transcription", func(t *testing.T) { testTranscription(t, newApp(t, e)) })
	t.Run("local_state_tools", func(t *testing.T) { testLocalState(t, newApp(t, e)) })
	t.Run("groups", func(t *testing.T) { testGroups(t, newApp(t, e)) })
	t.Run("media_api", func(t *testing.T) { testMediaAPI(t, newApp(t, e)) })
	t.Run("bridge", func(t *testing.T) { testBridge(t, e) })
	t.Run("restart_online_event", func(t *testing.T) { testRestart(t, newApp(t, e), hooks) })
	_ = a
}

func testHTTP(t *testing.T, a *app) {
	code, body := a.do(http.MethodGet, "/healthz", nil)
	if code != 200 || strings.TrimSpace(string(body)) != "ok" {
		t.Fatalf("/healthz = %d %q", code, body)
	}

	st := a.json(http.MethodGet, "/api/status", nil, 200)
	if v := str(st, "version"); a.e.version != "" && v != a.e.version {
		t.Errorf("version = %q, want %q (the build under test is not the one running)", v, a.e.version)
	}
	if next := str(st, "next"); next != "ready" {
		t.Fatalf("api/status next = %q, want ready: %s", next, clipAny(st))
	}
	if get(st, "whatsapp.connected") != true {
		t.Errorf("whatsapp not connected: %s", clipAny(get(st, "whatsapp")))
	}

	// /health answers 503 when a check fails. A test account gets few
	// messages, so "receiving" may fail; every other check must pass.
	code, body = a.do(http.MethodGet, "/health", nil)
	var h any
	if err := json.Unmarshal(body, &h); err != nil || (code != 200 && code != 503) {
		t.Fatalf("/health = %d %s", code, clip(body))
	}
	for _, c := range list(h, "checks") {
		if str(c, "status") == "fail" && str(c, "name") != "receiving" {
			t.Errorf("/health check %s fails: %s", str(c, "name"), clipAny(c))
		}
	}

	for _, p := range []string{"/", "/status", "/whatsapp", "/conectar", "/configuracoes", "/funcoes",
		"/receitas", "/ajuda", "/webhooks/documentacao", "/assets/i18n.js", "/favicon.svg"} {
		code, body := a.do(http.MethodGet, p, nil)
		if code != 200 {
			t.Errorf("GET %s = %d: %s", p, code, clip(body))
			continue
		}
		if strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".svg") {
			continue
		}
		page := string(body)
		if !strings.Contains(strings.ToLower(page), "<html") {
			t.Errorf("GET %s is not a page", p)
		}
		if strings.Contains(page, "{{") || strings.Contains(page, "%!") {
			t.Errorf("GET %s has a template or format left over", p)
		}
	}
	for _, p := range []string{"/api/state", "/api/chats", "/api/media", "/api/webhooks"} {
		a.json(http.MethodGet, p, nil, 200)
	}
	if code, body := a.do(http.MethodGet, "/diagnostico/logs.txt", nil); code != 200 || len(body) == 0 {
		t.Errorf("support logs = %d, %d bytes", code, len(body))
	}

	// A foreign origin is refused, the guard every action relies on.
	req, _ := http.NewRequest(http.MethodPost, a.e.base+"/api/webhooks", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://evil.example")
	if res, err := a.http.Do(req); err == nil {
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("cross-origin POST = %d, want 403", res.StatusCode)
		}
	}
}

// expectedTools is every tool the MCP offers. A tool added or removed must be
// added or removed here, and tested below.
var expectedTools = []string{
	"health", "whatsapp_status", "list_chats", "get_chat_messages", "search_messages", "list_contacts",
	"list_groups", "get_group", "send_text_message", "send_media_message", "download_media",
	"transcribe_audio", "save_transcript", "sync_history", "delete_message", "edit_message",
	"react_to_message", "check_numbers", "get_profile_picture", "send_location", "send_poll",
	"get_poll_results", "organise_chat", "forward_message", "mark_chat_read", "send_typing",
	"get_message_context", "message_stats", "export_messages", "list_unread", "list_unanswered",
	"list_mentions", "mark_handled", "snooze_chat", "manage_group_participants", "update_group",
	"get_group_invite_link", "leave_group", "media_stats", "purge_media",
}

func testProtocol(t *testing.T, a *app) {
	init, err := a.mcp("initialize", obj{"protocolVersion": "2025-06-18", "capabilities": obj{},
		"clientInfo": obj{"name": "integration", "version": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if str(init, "serverInfo.name") == "" || str(init, "protocolVersion") == "" {
		t.Errorf("initialize = %s", clipAny(init))
	}
	if str(init, "instructions") == "" {
		t.Error("initialize carries no instructions")
	}
	if _, err := a.mcp("ping", obj{}); err != nil {
		t.Errorf("ping: %v", err)
	}
	res, err := a.mcp("tools/list", obj{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, tool := range list(res, "tools") {
		got = append(got, str(tool, "name"))
		if str(tool, "description") == "" || get(tool, "inputSchema") == nil {
			t.Errorf("tool %s has no description or schema", str(tool, "name"))
		}
	}
	want := slices.Clone(expectedTools)
	sort.Strings(got)
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Errorf("tools/list differs from the suite:\n got  %v\n want %v", got, want)
	}
}

func testReadTools(t *testing.T, a *app) {
	h := a.call("health", obj{"max_silence_hours": 100000})
	if s := str(h, "status"); s != "ok" && s != "warn" {
		t.Errorf("health = %s", clipAny(h))
	}
	st := a.call("whatsapp_status", nil)
	if str(st, "sync.state") != "connected" {
		t.Errorf("sync state = %q", str(st, "sync.state"))
	}
	if a.e.version != "" && str(st, "gateway.version") != a.e.version {
		t.Errorf("gateway.version = %q", str(st, "gateway.version"))
	}

	chats := firstList(a.call("list_chats", obj{"limit": 5}), "chats")
	if len(chats) == 0 {
		t.Fatal("list_chats returned no chat")
	}
	trimmed := firstList(a.call("list_chats", obj{"limit": 2, "fields": []string{"jid"}}), "chats")
	if len(trimmed) > 0 && len(trimmed[0].(map[string]any)) != 1 {
		t.Errorf("fields did not trim list_chats: %s", clipAny(trimmed[0]))
	}

	// The most recent chat can hold only deleted messages (tests write to the
	// account's own chat or to WMCP_TO and delete it all for everyone): it
	// lists, but has nothing to read. The first recent chat with messages is
	// read instead.
	var chat string
	var msgs []any
	for _, c := range chats {
		chat = str(c, "jid")
		if msgs = firstList(a.call("get_chat_messages", obj{"chat_jid": chat, "limit": 5}), "messages"); len(msgs) > 0 {
			break
		}
	}
	if len(msgs) == 0 {
		t.Errorf("get_chat_messages returned nothing for the %d most recent chats", len(chats))
	}
	cnt := a.call("get_chat_messages", obj{"chat_jid": chat, "count_only": true})
	if get(cnt, "count") == nil {
		t.Errorf("count_only = %s", clipAny(cnt))
	}
	if len(msgs) > 0 {
		id := str(msgs[0], "id")
		ctx := a.call("get_message_context", obj{"message_id": id, "chat_jid": chat, "before": 2, "after": 2})
		if clipAny(ctx) == "null" {
			t.Error("get_message_context returned nothing")
		}
	}

	a.call("search_messages", obj{"query": "a", "limit": 3})
	a.call("list_contacts", obj{"limit": 5})
	a.call("message_stats", obj{"group_by": "month", "limit": 3})
	a.call("message_stats", obj{"group_by": "chat", "limit": 3, "exclude_groups": true})
	a.call("list_unread", obj{"limit": 5})
	a.call("list_unanswered", obj{"limit": 5})
	a.call("list_mentions", obj{"limit": 5})
	a.call("media_stats", nil)

	exp := a.call("export_messages", obj{"since": time.Now().AddDate(0, 0, -30).Format("2006-01-02")})
	if p := str(exp, "path"); p == "" {
		t.Errorf("export_messages = %s", clipAny(exp))
	} else if out, err := remote(t, a.e, "test -f '"+p+"' && echo yes; rm -f '"+p+"'", nil); err != nil || !strings.Contains(out, "yes") {
		t.Errorf("export file %s missing on the test Mac: %s %v", p, out, err)
	}

	phone := digits(str(st, "account.phone"))
	if phone == "" {
		t.Fatalf("whatsapp_status has no account phone: %s", clipAny(st))
	}
	nums := a.call("check_numbers", obj{"numbers": []string{"+" + phone}})
	if r := list(nums, "results"); len(r) != 1 || get(r[0], "registered") != true {
		t.Errorf("check_numbers(own number) = %s", clipAny(nums))
	}
	if v, isErr := a.tryCall("get_profile_picture", obj{"to": phone}); isErr && !strings.Contains(strings.ToLower(clipAny(v)), "no profile picture") {
		t.Errorf("get_profile_picture = %s", clipAny(v))
	}
}

func testMediaAPI(t *testing.T, a *app) {
	m := a.json(http.MethodGet, "/api/media", nil, 200)
	days := 0
	if d, ok := get(m, "retention_days").(float64); ok {
		days = int(d)
	}
	// Setting a retention deletes what is older at once: ten years keeps all.
	if r := a.json(http.MethodPost, "/api/media/retention", obj{"days": 3650}, 200); get(r, "retention_days") != float64(3650) {
		t.Errorf("retention = %s", clipAny(r))
	}
	a.json(http.MethodPost, "/api/media/retention", obj{"days": days}, 200)
	a.call("purge_media", obj{"older_than_days": 3650})
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
