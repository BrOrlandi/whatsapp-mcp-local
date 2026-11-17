package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// What 1.8.0 accepted as a recipient or a conversation is accepted again: a
// contact or group name, any JID as the reading tools return it, a phone
// number. Each tool passes it to wacli after its own flag, as 1.8.0 did
// (wacli resolves the name), and a draft resolves the name itself.
func TestNamesAndChatIDsAreAcceptedAsInV180(t *testing.T) {
	base := ""
	if runtime.GOOS != "windows" {
		base = "/tmp"
	}
	root, err := os.MkdirTemp(base, "daemon-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	store := filepath.Join(root, "s")
	_ = os.MkdirAll(store, 0o700)
	_ = os.WriteFile(filepath.Join(store, "AUTHED"), nil, 0o600)
	seedStore(t, store)
	port := freePort(t)
	d, err := Start(Config{Port: port, WacliBin: fakeWacli, StoreDir: store, DataDir: filepath.Join(root, "data")})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Stop()
	waitSync(t, d, "connected")
	argsLog := func() string { b, _ := os.ReadFile(filepath.Join(store, "args.log")); return string(b) }
	refused := func(name string, res map[string]any, isErr bool) {
		t.Helper()
		if isErr {
			if e, _ := res["error"].(string); strings.Contains(e, "must be a JID") || strings.Contains(e, "ending in @g.us") {
				t.Errorf("%s refused what 1.8.0 accepted: %v", name, res)
			}
		}
	}

	// Each tool, with a name, as 1.8.0 put it on wacli's line.
	cases := []struct {
		tool string
		args map[string]any
		line string
	}{
		{"send_text_message", map[string]any{"to": "Mãe", "text": "oi"}, "send text --to Mãe --message oi"},
		{"send_location", map[string]any{"to": "Mãe", "latitude": -23.5, "longitude": -46.6}, "send location --to Mãe"},
		{"send_poll", map[string]any{"to": "Mãe", "question": "Q?", "options": []string{"a", "b"}}, "send poll --to Mãe --question Q?"},
		{"get_profile_picture", map[string]any{"to": "Mãe"}, "profile picture-info --jid Mãe"},
		{"forward_message", map[string]any{"message_id": "M1", "to": "Lucas"}, "messages forward --chat " + testMae + " --id M1 --to Lucas"},
		{"send_typing", map[string]any{"to": "Mãe"}, "presence typing --to Mãe"},
		{"mark_chat_read", map[string]any{"chat_jid": "Mãe"}, "chats mark-read --chat Mãe"},
		{"organise_chat", map[string]any{"chat_jid": "Mãe", "action": "archive"}, "chats archive --chat Mãe"},
		{"get_chat_messages", map[string]any{"chat_jid": "Mãe"}, "messages list --chat Mãe"},
		{"search_messages", map[string]any{"query": "almoço", "chat_jid": "Mãe"}, "--chat Mãe"},
		{"manage_group_participants", map[string]any{"group_jid": testGroup, "action": "add", "participants": []string{"Mãe", "+55 11 90000-0002"}}, "groups participants add --jid " + testGroup + " --user Mãe --user +55 11 90000-0002"},
		{"check_numbers", map[string]any{"numbers": []string{"+55 11 90000-0001", "Mãe"}}, "contacts check -- +55 11 90000-0001 Mãe"},
		{"sync_history", map[string]any{"chat_jid": "Mãe"}, "history backfill --chat Mãe"},
	}
	for _, c := range cases {
		res, isErr := callTool(t, port, c.tool, c.args)
		refused(c.tool, res, isErr)
		deadline := time.Now().Add(5 * time.Second)
		for !strings.Contains(argsLog(), c.line) && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		if !strings.Contains(argsLog(), c.line) {
			t.Errorf("%s: wacli did not get %q (answer %v)", c.tool, c.line, res)
		}
	}

	// A draft resolves the name to the chat, as in 1.8.0.
	for _, tool := range []string{"send_text_message", "send_media_message"} {
		args := map[string]any{"to": "Mãe", "text": "oi", "url": "https://example.com/a.png", "type": "image",
			"dry_run": true}
		res, isErr := callTool(t, port, tool, args)
		refused(tool, res, isErr)
		r, _ := res["recipient"].(map[string]any)
		if r["jid"] != testMae {
			t.Errorf("%s draft to a name: %v", tool, res)
		}
	}

	res, isErr := callTool(t, port, "forward_message", map[string]any{"message_id": "M1", "to": "Lucas", "dry_run": true})
	refused("forward_message", res, isErr)
	if r, _ := res["recipient"].(map[string]any); r["jid"] != testLucas {
		t.Errorf("forward draft to a name: %v", res)
	}
	// A name matching several chats gives them all, nothing sent.
	res, isErr = callTool(t, port, "send_text_message", map[string]any{"to": "551190", "text": "oi", "dry_run": true})
	refused("send_text_message", res, isErr)
	if c, _ := res["recipient_candidates"].([]any); len(c) != 2 {
		t.Errorf("an ambiguous name: %v", res)
	}

	// Chat ids as the reading tools return them, and odd group ids, are
	// not refused.
	for _, jid := range []string{"123456789012345@lid", "status@broadcast", "120363000000000000@newsletter",
		"5511900000001:12@s.whatsapp.net", "5511900000001-1600000000@g.us"} {
		res, isErr := callTool(t, port, "get_chat_messages", map[string]any{"chat_jid": jid})
		refused("get_chat_messages "+jid, res, isErr)
	}
	res, isErr = callTool(t, port, "get_group", map[string]any{"group_jid": "Família 2024@g.us"})
	refused("get_group", res, isErr)
}
