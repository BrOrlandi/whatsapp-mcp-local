package daemon

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

const (
	testMae   = "5511900000001@s.whatsapp.net"
	testLucas = "5511900000002@s.whatsapp.net"
	testGroup = "120363000000000001@g.us"
)

// seedStore writes the part of wacli's store the tools read: two chats, Mãe
// waiting for an answer.
func seedStore(t *testing.T, store string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(store, "wacli.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().Unix()
	for _, q := range []string{
		`CREATE TABLE chats (jid TEXT PRIMARY KEY, kind TEXT NOT NULL, name TEXT, last_message_ts INTEGER, archived INTEGER NOT NULL DEFAULT 0,
			pinned INTEGER NOT NULL DEFAULT 0, muted_until INTEGER NOT NULL DEFAULT 0, unread INTEGER NOT NULL DEFAULT 0, unread_count INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE contacts (jid TEXT PRIMARY KEY, phone TEXT, push_name TEXT, full_name TEXT, first_name TEXT, business_name TEXT, system_name TEXT, updated_at INTEGER NOT NULL)`,
		`CREATE TABLE groups (jid TEXT PRIMARY KEY, name TEXT, owner_jid TEXT, created_ts INTEGER, is_parent INTEGER NOT NULL DEFAULT 0, linked_parent_jid TEXT, left_at INTEGER, updated_at INTEGER NOT NULL)`,
		`CREATE TABLE contact_aliases (jid TEXT PRIMARY KEY, alias TEXT NOT NULL, notes TEXT, updated_at INTEGER NOT NULL)`,
		`CREATE TABLE messages (rowid INTEGER PRIMARY KEY AUTOINCREMENT, chat_jid TEXT NOT NULL, chat_name TEXT, msg_id TEXT NOT NULL, sender_jid TEXT, sender_name TEXT,
			ts INTEGER NOT NULL, from_me INTEGER NOT NULL, text TEXT, display_text TEXT, quoted_msg_id TEXT, is_forwarded INTEGER NOT NULL DEFAULT 0,
			reaction_to_id TEXT, reaction_emoji TEXT, media_type TEXT, media_caption TEXT, filename TEXT, mime_type TEXT, direct_path TEXT,
			revoked INTEGER NOT NULL DEFAULT 0, deleted_at INTEGER, edited INTEGER NOT NULL DEFAULT 0, UNIQUE(chat_jid, msg_id))`,
		fmt.Sprintf(`INSERT INTO chats (jid, kind, name, last_message_ts, unread_count) VALUES ('%s', 'dm', 'Mãe', %d, 1), ('%s', 'dm', 'Lucas', %d, 0)`,
			testMae, now-60, testLucas, now-3600),
		fmt.Sprintf(`INSERT INTO contacts (jid, phone, full_name, updated_at) VALUES ('%s', '5511900000001', 'Mãe', 0), ('%s', '5511900000002', 'Lucas', 0)`,
			testMae, testLucas),
		fmt.Sprintf(`INSERT INTO messages (chat_jid, msg_id, sender_jid, ts, from_me, text) VALUES
			('%s', 'M1', '%s', %d, 0, 'Vem almoçar domingo?'),
			('%s', 'L1', '5511999999999@s.whatsapp.net', %d, 1, 'Te mandei o arquivo')`, testMae, testMae, now-60, testLucas, now-3600),
		fmt.Sprintf(`INSERT INTO messages (chat_jid, msg_id, sender_jid, ts, from_me, text, media_type, mime_type, filename) VALUES
			('%s', 'P1', '%s', %d, 0, '', 'image', 'image/png', 'foto.png'),
			('%s', 'D1', '%s', %d, 0, '', 'document', 'text/plain', 'lista.txt')`, testMae, testMae, now-120, testMae, testMae, now-110),
		fmt.Sprintf(`INSERT INTO messages (chat_jid, msg_id, sender_jid, ts, from_me, text) VALUES ('%s', 'G1', '%s', %d, 1, 'epa')`,
			testGroup, testGroup, now-7200),
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

func callTool(t *testing.T, port int, name string, args map[string]any) (map[string]any, bool) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	resp, err := http.Post("http://127.0.0.1:"+strconv.Itoa(port)+"/mcp", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Result struct {
			Content []struct{ Text string }
			IsError bool
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || len(out.Result.Content) == 0 {
		t.Fatalf("%s: unreadable answer: %v", name, err)
	}
	var result map[string]any
	_ = json.Unmarshal([]byte(out.Result.Content[0].Text), &result)
	return result, out.Result.IsError
}

func callContent(t *testing.T, port int, name string, args map[string]any) []map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	resp, err := http.Post("http://127.0.0.1:"+strconv.Itoa(port)+"/mcp", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Result struct{ Content []map[string]any }
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.Result.Content
}

func agentRequest(t *testing.T, port int, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var r *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	} else {
		r = bytes.NewReader([]byte("{}"))
	}
	req, _ := http.NewRequest(method, "http://127.0.0.1:"+strconv.Itoa(port)+path, r)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestNewToolsReachWacliTheRightWay(t *testing.T) {
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

	// Sync runs with the webhook relay.
	if log := argsLog(); !strings.Contains(log, "sync --follow") || !strings.Contains(log, "--webhook http://127.0.0.1:") {
		t.Errorf("sync was not pointed at the relay:\n%s", log)
	}

	// A reply with a mention goes through the running sync, quoting by id.
	if res, isErr := callTool(t, port, "send_text_message", map[string]any{"to": testMae, "text": "oi @5511900000002",
		"reply_to": "M1", "mentions": []string{"5511900000002"}}); isErr {
		t.Fatalf("send: %v", res)
	}
	if log := argsLog(); !strings.Contains(log, "--reply-to M1") || !strings.Contains(log, "--mention 5511900000002") {
		t.Errorf("reply and mention flags missing:\n%s", log)
	}
	// A reply to a message of another conversation is refused.
	if _, isErr := callTool(t, port, "send_text_message", map[string]any{"to": testLucas, "text": "oi", "reply_to": "M1"}); !isErr {
		t.Error("quoting another conversation's message should be refused")
	}

	// A dry run sends nothing and names the recipient.
	before := argsLog()
	res, isErr := callTool(t, port, "send_text_message", map[string]any{"to": "+55 11 90000-0001", "text": "rascunho", "dry_run": true})
	if isErr || res["sent"] != false || res["recipient"].(map[string]any)["name"] != "Mãe" {
		t.Errorf("dry run: %v", res)
	}
	if argsLog() != before {
		t.Error("a dry run ran wacli")
	}

	// A sticker goes through the running sync as one; only a WebP is one.
	webp := filepath.Join(root, "fig.webp")
	_ = os.WriteFile(webp, []byte("RIFF\x10\x00\x00\x00WEBPVP8 "), 0o600)
	if res, isErr := callTool(t, port, "send_media_message", map[string]any{"to": testMae, "type": "sticker", "url": webp}); isErr {
		t.Fatalf("sticker: %v", res)
	}
	if log := argsLog(); !strings.Contains(log, "send sticker --to "+testMae+" --file "+webp) {
		t.Errorf("sticker not sent as one:\n%s", log)
	}
	notWebP := filepath.Join(root, "foto.png")
	_ = os.WriteFile(notWebP, []byte("\x89PNG\r\n\x1a\n0000"), 0o600)
	if _, isErr := callTool(t, port, "send_media_message", map[string]any{"to": testMae, "type": "sticker", "url": notWebP}); !isErr {
		t.Error("a PNG should be refused as a sticker")
	}
	if _, isErr := callTool(t, port, "send_media_message", map[string]any{"to": testMae, "type": "sticker", "url": webp, "caption": "oi"}); !isErr {
		t.Error("a sticker with a caption should be refused")
	}

	// Forwarding and deleting for me need the store: sync pauses.
	if res, isErr := callTool(t, port, "forward_message", map[string]any{"message_id": "M1", "to": testLucas}); isErr {
		t.Fatalf("forward: %v", res)
	}
	if res, _ := callTool(t, port, "delete_message", map[string]any{"message_id": "M1", "for_me": true}); res["preview"] != true {
		t.Errorf("delete for me should preview first: %v", res)
	}
	if res, isErr := callTool(t, port, "delete_message", map[string]any{"message_id": "M1", "for_me": true, "confirm": true}); isErr {
		t.Fatalf("delete for me: %v", res)
	}
	if log := argsLog(); !strings.Contains(log, "messages forward --chat "+testMae+" --id M1 --to "+testLucas) ||
		!strings.Contains(log, "messages delete --chat "+testMae+" --id M1 --for-me") {
		t.Errorf("forward or delete for me missing:\n%s", log)
	}

	// A reaction to a received DM message names its sender, or WhatsApp
	// takes it for a reaction to one of the account's own messages; a
	// reaction to an own message names none.
	if res, isErr := callTool(t, port, "react_to_message", map[string]any{"message_id": "M1", "emoji": "🧡"}); isErr {
		t.Fatalf("react: %v", res)
	}
	if res, isErr := callTool(t, port, "react_to_message", map[string]any{"message_id": "L1", "emoji": "👍"}); isErr {
		t.Fatalf("react to own: %v", res)
	}
	if log := argsLog(); !strings.Contains(log, "send react --to "+testMae+" --id M1 --reaction 🧡 --sender "+testMae) ||
		!strings.Contains(log, "send react --to "+testLucas+" --id L1 --reaction 👍\n") {
		t.Errorf("reaction sender wrong:\n%s", log)
	}

	// In a group, a reaction to an own message names the account, which the
	// index leaves out or records as the group itself.
	if res, isErr := callTool(t, port, "react_to_message", map[string]any{"message_id": "G1", "emoji": "🧡"}); isErr {
		t.Fatalf("react in group: %v", res)
	}
	if !strings.Contains(argsLog(), "send react --to "+testGroup+" --id G1 --reaction 🧡 --sender 5511912345678@s.whatsapp.net") {
		t.Errorf("own group message reacted without the account:\n%s", argsLog())
	}

	// Chat state goes through sync; a wacli whose sync refuses it (the fake
	// still treats archive as needing the store) falls back to a pause.
	if res, isErr := callTool(t, port, "mark_chat_read", map[string]any{"chat_jid": testMae}); isErr {
		t.Fatalf("mark read: %v", res)
	}
	if res, isErr := callTool(t, port, "organise_chat", map[string]any{"chat_jid": testMae, "action": "archive"}); isErr {
		t.Fatalf("archive: %v", res)
	}
	if !strings.Contains(argsLog(), "chats mark-read --chat "+testMae+" --receipts") {
		t.Errorf("mark read without receipts:\n%s", argsLog())
	}

	// Triage reads the store: Mãe waits, Lucas does not.
	res, _ = callTool(t, port, "list_unanswered", nil)
	chats, _ := res["chats"].([]any)
	if len(chats) != 1 || chats[0].(map[string]any)["chat_jid"] != testMae {
		t.Errorf("unanswered: %v", res)
	}
	if res, isErr := callTool(t, port, "mark_handled", map[string]any{"chat_jid": testMae}); isErr {
		t.Fatalf("mark handled: %v", res)
	}
	if res, _ := callTool(t, port, "list_unanswered", nil); len(res["chats"].([]any)) != 0 {
		t.Errorf("a handled chat should leave the list: %v", res)
	}
	if res, _ := callTool(t, port, "message_stats", map[string]any{"group_by": "chat"}); res["total"] != float64(5) {
		t.Errorf("stats: %v", res)
	}
	res, isErr = callTool(t, port, "export_messages", nil)
	if isErr || res["count"] != float64(5) {
		t.Fatalf("export: %v", res)
	}
	if b, err := os.ReadFile(res["path"].(string)); err != nil || strings.Count(string(b), "\n") != 5 {
		t.Errorf("export file: %q %v", b, err)
	}

	// download_media hands over the file as WhatsApp delivered it: a picture
	// as the very same image, anything else as a file.
	media := filepath.Join(root, "data", "media", testMae)
	_ = os.MkdirAll(media, 0o700)
	img := image.NewRGBA(image.Rect(0, 0, 3000, 1000))
	var png bytes.Buffer
	_ = pngEncode(&png, img)
	_ = os.WriteFile(filepath.Join(media, "P1.png"), png.Bytes(), 0o600)
	_ = os.WriteFile(filepath.Join(media, "D1.txt"), []byte("arroz\nfeijão\n"), 0o600)
	blocks := callContent(t, port, "download_media", map[string]any{"message_id": "P1"})
	if len(blocks) != 2 || blocks[1]["type"] != "image" || blocks[1]["data"] != base64.StdEncoding.EncodeToString(png.Bytes()) {
		t.Errorf("download_media of a picture should return the original image: %.300v", blocks)
	}
	blocks = callContent(t, port, "download_media", map[string]any{"message_id": "D1"})
	if len(blocks) != 2 || blocks[1]["type"] != "resource" {
		t.Errorf("download_media of a document should return it as a file: %.300v", blocks)
	}
	if res, _ := callTool(t, port, "download_media", map[string]any{"message_id": "D1", "link": true}); !strings.Contains(fmt.Sprint(res["url"]), "/media/") {
		t.Errorf("download_media with link: %v", res)
	}
	if res, _ := callTool(t, port, "media_stats", nil); res["inventory"].(map[string]any)["files"] != float64(2) {
		t.Errorf("media_stats: %v", res)
	}
	if res, _ := callTool(t, port, "purge_media", map[string]any{"chat_jid": testMae}); res["would_delete_files"] != float64(2) {
		t.Errorf("purge preview: %v", res)
	}
	if res, _ := callTool(t, port, "purge_media", map[string]any{"chat_jid": testMae, "confirm": true}); res["deleted_files"] != float64(2) {
		t.Errorf("purge: %v", res)
	}

	// Webhooks are configured through the API.
	code, list := agentRequest(t, port, "GET", "/api/webhooks", nil)
	if code != 200 || list["available"] != true {
		t.Fatalf("GET /api/webhooks = %d %v", code, list)
	}
	code, created := agentRequest(t, port, "POST", "/api/webhooks", map[string]any{"url": "http://127.0.0.1:9/hook", "events": []string{"message"}})
	if code != 200 || created["secret"] == "" {
		t.Fatalf("POST /api/webhooks = %d %v", code, created)
	}
	id := created["webhook"].(map[string]any)["id"].(string)
	if code, _ := agentRequest(t, port, "POST", "/api/webhooks", map[string]any{"url": "nope"}); code != http.StatusConflict {
		t.Errorf("a bad url answered %d", code)
	}
	if code, out := agentRequest(t, port, "PATCH", "/api/webhooks/"+id, map[string]any{"enabled": false}); code != 200 || out["enabled"] != false {
		t.Errorf("PATCH = %d %v", code, out)
	}
	if code, _ := agentRequest(t, port, "DELETE", "/api/webhooks/"+id, nil); code != 200 {
		t.Errorf("DELETE = %d", code)
	}
	if code, _ := agentRequest(t, port, "GET", "/api/webhooks/"+id, nil); code != http.StatusConflict {
		t.Errorf("a deleted webhook answered %d", code)
	}
	if code, inv := agentRequest(t, port, "GET", "/api/media", nil); code != 200 || inv["files"] != float64(0) {
		t.Errorf("GET /api/media = %d %v", code, inv)
	}
	if code, out := agentRequest(t, port, "POST", "/api/media/purge", map[string]any{"what": "exports"}); code != 200 || out["deleted_files"] != float64(1) {
		t.Errorf("purging the export = %d %v", code, out)
	}

	// The documentation shows each kind of delivery, from the real types.
	if code, body, _ := get(port, "/webhooks/documentacao"); code != 200 || !strings.Contains(body, "X-WhatsApp-MCP-Signature") ||
		!strings.Contains(body, "&#34;event&#34;: &#34;reaction&#34;") || strings.Contains(body, "can't evaluate") {
		t.Errorf("the webhook documentation answered %d", code)
	}

	// Both are on the settings page, in the command line's panel too.
	if code, body, _ := get(port, "/configuracoes"); code != 200 || !strings.Contains(body, `id="webhooks"`) || !strings.Contains(body, `id="arquivos"`) {
		t.Errorf("the settings page lacks the webhook or the files card (%d)", code)
	}
}

var pngEncode = png.Encode
