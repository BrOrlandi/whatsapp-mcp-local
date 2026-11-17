//go:build integration

package integration

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// The messaging step sends real messages from the test account to WMCP_TO
// (wacli refuses to send to the account itself) and deletes them for
// everyone at the end. Every text carries a fresh tag, to find it again.

const media = "/tmp/whatsapp-mcp-integration"

func sentID(v any) string {
	for _, p := range []string{"id", "message_id", "result.id", "result.message_id", "sent.id", "message.id", "data.id"} {
		if s := str(v, p); s != "" {
			return s
		}
	}
	return ""
}

func testMessaging(t *testing.T, a *app, r *receiver) {
	to := os.Getenv("WMCP_TO")
	if to == "" {
		t.Skip("WMCP_TO is not set: no conversation to send the test messages to")
	}
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	tag := "it" + hex.EncodeToString(b)
	label := "[teste de integração " + tag + "]"

	var sent []string
	var chat string
	t.Cleanup(func() {
		for _, id := range sent {
			if v, isErr := a.tryCall("delete_message", obj{"message_id": id, "chat_jid": chat, "confirm": true}); isErr {
				t.Errorf("could not delete %s: %s", id, clipAny(v))
			}
		}
	})
	send := func(tool string, args obj) string {
		t.Helper()
		v := a.call(tool, args)
		id := sentID(v)
		if id == "" {
			t.Fatalf("%s returned no message id: %s", tool, clipAny(v))
		}
		sent = append(sent, id)
		// The conversation as the reading tools know it, not the LID
		// WhatsApp may answer with.
		if got := str(v, "chat_jid"); got != chat {
			t.Errorf("%s answered chat_jid %q, want %q", tool, got, chat)
		}
		return id
	}

	a.hook(t, r, "/msg", obj{"events": []string{"message", "reaction", "receipt"}, "include_own": true})

	// Dry run: the draft and its recipient, nothing sent.
	dry := a.call("send_text_message", obj{"to": to, "text": label, "dry_run": true})
	chat = str(dry, "recipient.jid")
	if get(dry, "sent") != false || chat == "" {
		t.Fatalf("dry run = %s", clipAny(dry))
	}

	a.call("send_typing", obj{"to": to})
	a.call("send_typing", obj{"to": to, "typing": false})

	textID := send("send_text_message", obj{"to": to, "text": label + " texto"})
	d, ok := r.wait(t, "/msg", 60*time.Second, "the message webhook (include_own, sent by a tool)", func(d delivery) bool {
		return d.event == "message" && strings.Contains(str(d.body, "message.text"), tag)
	})
	if ok && (get(d.body, "message.from_me") != true || str(d.body, "message.id") != textID) {
		t.Errorf("message webhook = %s", clipAny(d.body))
	}

	eventually(t, 60*time.Second, "the message in search_messages", func() bool {
		return strings.Contains(mustJSON(a.call("search_messages", obj{"query": tag, "limit": 5})), textID)
	})
	if !strings.Contains(mustJSON(a.call("get_chat_messages", obj{"chat_jid": chat, "limit": 10})), textID) {
		t.Error("get_chat_messages does not show the message sent")
	}
	a.call("get_message_context", obj{"message_id": textID, "chat_jid": chat, "before": 1, "after": 1})

	replyID := send("send_text_message", obj{"to": to, "text": label + " resposta", "reply_to": textID})
	eventually(t, 60*time.Second, "the reply quoting the first message", func() bool {
		msgs := firstList(a.call("get_chat_messages", obj{"chat_jid": chat, "limit": 10, "fields": []string{"id", "quoted_message_id"}}), "messages")
		for _, m := range msgs {
			if str(m, "id") == replyID {
				return str(m, "quoted_message_id") == textID
			}
		}
		return false
	})

	a.call("react_to_message", obj{"message_id": textID, "chat_jid": chat, "emoji": "👍"})
	r.wait(t, "/msg", 60*time.Second, "the reaction webhook", func(d delivery) bool {
		return d.event == "reaction" && str(d.body, "message.reaction.to") == textID
	})
	a.call("react_to_message", obj{"message_id": textID, "chat_jid": chat, "emoji": ""})

	a.call("edit_message", obj{"message_id": textID, "chat_jid": chat, "text": label + " texto editado"})
	eventually(t, 60*time.Second, "the edited text", func() bool {
		return strings.Contains(mustJSON(a.call("search_messages", obj{"query": "editado " + tag, "limit": 5})), textID) ||
			strings.Contains(mustJSON(a.call("get_chat_messages", obj{"chat_jid": chat, "limit": 10})), "texto editado")
	})

	// The other side's phone confirms delivery.
	r.wait(t, "/msg", 2*time.Minute, "a delivery receipt", func(d delivery) bool {
		return d.event == "receipt" && strings.Contains(mustJSON(d.body), textID)
	})

	send("send_location", obj{"to": to, "latitude": -21.1767, "longitude": -47.8208, "name": label, "address": "Ribeirão Preto"})

	pollID := send("send_poll", obj{"to": to, "question": label + " enquete", "options": []string{"sim", "não"}})
	a.call("get_poll_results", obj{"message_id": pollID, "chat_jid": chat})

	fwd := a.call("forward_message", obj{"message_id": textID, "chat_jid": chat, "to": to, "dry_run": true})
	if get(fwd, "sent") == true {
		t.Errorf("forward dry run sent: %s", clipAny(fwd))
	}
	send("forward_message", obj{"message_id": textID, "chat_jid": chat, "to": to})

	testMedia(t, a, r, to, chat, tag, label, send)

	a.call("sync_history", obj{"chat_jid": chat, "count": 5})
	a.call("mark_chat_read", obj{"chat_jid": chat, "receipts": false})

	// Deleting is two-step: without confirm nothing changes.
	pre := a.call("delete_message", obj{"message_id": textID, "chat_jid": chat})
	if !strings.Contains(mustJSON(pre), "confirm") && get(pre, "preview") != true {
		t.Errorf("delete preview = %s", clipAny(pre))
	}
	if strings.Contains(mustJSON(a.call("get_chat_messages", obj{"chat_jid": chat, "limit": 20, "fields": []string{"id", "revoked"}})), `"revoked":true,"id":"`+textID) {
		t.Error("the preview deleted the message")
	}
}

// testMedia sends an image, a document and a voice note made on the test
// Mac, downloads one back and transcribes the audio.
func testMedia(t *testing.T, a *app, r *receiver, to, chat, tag, label string, send func(string, obj) string) {
	prep := `set -e; mkdir -p ` + media + `; cd ` + media + `
sips -s format png "/Applications/WhatsApp MCP.app/Contents/Resources/icon.icns" --out image.png >/dev/null
sips -Z 256 image.png >/dev/null
printf 'teste de integracao do WhatsApp MCP\n' > documento.txt
say -o audio.aiff "Teste de integração do WhatsApp MCP. Um, dois, três."
afconvert -f m4af -d aac audio.aiff audio.m4a; rm -f audio.aiff; ls`
	if out, err := remote(t, a.e, prep, nil); err != nil {
		t.Fatalf("preparing the media on the test Mac: %v %s", err, out)
	}

	imgID := send("send_media_message", obj{"to": to, "type": "image", "url": media + "/image.png", "caption": label + " imagem"})
	r.wait(t, "/msg", 30*time.Second, "the image webhook", func(d delivery) bool {
		return d.event == "message" && str(d.body, "message.id") == imgID && str(d.body, "message.media.type") == "image"
	})
	send("send_media_message", obj{"to": to, "type": "document", "url": media + "/documento.txt", "filename": "teste.txt", "caption": label})
	audioID := send("send_media_message", obj{"to": to, "type": "audio", "url": media + "/audio.m4a"})

	dl := a.call("download_media", obj{"message_id": imgID, "chat_jid": chat, "link": true})
	link := ""
	for _, p := range []string{"url", "link", "download_url"} {
		if link = str(dl, p); link != "" {
			break
		}
	}
	if link == "" {
		t.Errorf("download_media link = %s", clipAny(dl))
	} else {
		// The link names the app's own port on the test Mac; fetch it there.
		out, err := remote(t, a.e, "curl -s -o /dev/null -w '%{http_code} %{size_download}' '"+link+"'", nil)
		if err != nil || !strings.HasPrefix(out, "200 ") || strings.HasSuffix(out, " 0") {
			t.Errorf("fetching the download link: %s %v", out, err)
		}
	}
	if p := str(dl, "path"); p != "" {
		if out, _ := remote(t, a.e, "test -s '"+p+"' && echo yes", nil); !strings.Contains(out, "yes") {
			t.Errorf("downloaded file %s missing", p)
		}
	}

	st := a.call("whatsapp_status", nil)
	ready := get(st, "transcription.local.ready") == true
	v, isErr := a.tryCall("transcribe_audio", obj{"message_id": audioID, "chat_jid": chat, "language": "pt"})
	switch {
	case ready && isErr:
		t.Errorf("transcribe_audio failed with transcription installed: %s", clipAny(v))
	case ready && !strings.Contains(strings.ToLower(mustJSON(v)), "integra"):
		t.Errorf("transcript does not match the audio: %s", clipAny(v))
	case !ready:
		t.Logf("transcription not installed on the test Mac; transcribe_audio answered %s", clipAny(v))
	}
	a.call("save_transcript", obj{"message_id": audioID, "chat_jid": chat, "text": "transcrição revisada " + tag, "language": "pt"})
	eventually(t, 30*time.Second, "the saved transcript in search", func() bool {
		return strings.Contains(mustJSON(a.call("search_messages", obj{"query": "revisada " + tag, "limit": 5})), audioID)
	})
	a.call("media_stats", nil)
	a.call("purge_media", obj{"chat_jid": chat})
	remote(t, a.e, "rm -rf "+media, nil)
}

// testLocalState covers the tools that change only this computer or this
// account's own view of a chat, undoing each change.
func testLocalState(t *testing.T, a *app) {
	chat := ""
	if to := os.Getenv("WMCP_TO"); to != "" {
		chat = str(a.call("send_text_message", obj{"to": to, "text": "x", "dry_run": true}), "recipient.jid")
	}
	if chat == "" {
		chats := firstList(a.call("list_chats", obj{"limit": 1}), "chats")
		if len(chats) == 0 {
			t.Fatal("no chat")
		}
		chat = str(chats[0], "jid")
	}
	// WhatsApp's servers sometimes refuse a chat-state change with a 500
	// right after another one; a second try goes through.
	organise := func(action string) {
		t.Helper()
		for try := 0; ; try++ {
			v, isErr := a.tryCall("organise_chat", obj{"chat_jid": chat, "action": action})
			if !isErr {
				return
			}
			if try == 2 || !strings.Contains(mustJSON(v), "internal-server-error") {
				t.Fatalf("organise_chat %s: %s", action, clipAny(v))
			}
			time.Sleep(3 * time.Second)
		}
	}
	field := func(name string) any {
		for _, c := range firstList(a.call("list_chats", obj{"limit": 200}), "chats") {
			if str(c, "jid") == chat {
				return get(c, name)
			}
		}
		return nil
	}

	organise("pin")
	eventually(t, 30*time.Second, "the chat pinned", func() bool { return field("pinned") == true })
	organise("unpin")
	eventually(t, 30*time.Second, "the chat unpinned", func() bool { return field("pinned") == false })
	organise("mute")
	organise("unmute")
	organise("archive")
	organise("unarchive")
	eventually(t, 30*time.Second, "the chat unarchived", func() bool { return field("archived") == false })

	a.call("mark_handled", obj{"chat_jid": chat, "note": "integration"})
	a.call("mark_handled", obj{"chat_jid": chat, "clear": true})
	a.call("snooze_chat", obj{"chat_jid": chat, "until": time.Now().Add(24 * time.Hour).Format(time.RFC3339)})
	a.call("mark_handled", obj{"chat_jid": chat, "clear": true})
	a.call("list_unanswered", obj{"include_handled": true, "limit": 5})
}

// testGroups reads the groups, and with WMCP_GROUP (a group the test account
// administers, made for this) also changes it and puts it back.
func testGroups(t *testing.T, a *app) {
	groups := firstList(a.call("list_groups", obj{"limit": 5}), "groups")
	if len(groups) == 0 {
		t.Skip("the test account is in no group")
	}
	g := a.call("get_group", obj{"group_jid": str(groups[0], "jid")})
	if len(firstList(g, "participants", "group.participants")) == 0 {
		t.Errorf("get_group has no participants: %s", clipAny(g))
	}
	gid := a.e.group
	if gid == "" {
		t.Log("WMCP_GROUP not set: the group changes are skipped")
		return
	}
	live := a.call("get_group", obj{"group_jid": gid, "live": true})
	name := ""
	for _, p := range []string{"name", "group.name"} {
		if name = str(live, p); name != "" {
			break
		}
	}
	a.call("update_group", obj{"group_jid": gid, "description": "teste de integração " + time.Now().Format(time.RFC3339)})
	if name != "" {
		a.call("update_group", obj{"group_jid": gid, "name": name})
	}
	a.call("update_group", obj{"group_jid": gid, "description": ""})
	if link := a.call("get_group_invite_link", obj{"group_jid": gid}); !strings.Contains(mustJSON(link), "chat.whatsapp.com") {
		t.Errorf("invite link = %s", clipAny(link))
	}
	if v := a.call("get_group_invite_link", obj{"group_jid": gid, "reset": true}); strings.Contains(mustJSON(v), "chat.whatsapp.com/") && get(v, "reset") == true {
		t.Errorf("reset without confirm went ahead: %s", clipAny(v))
	}
	if v := a.call("leave_group", obj{"group_jid": gid}); get(v, "left") == true {
		t.Fatalf("leave_group without confirm left the group: %s", clipAny(v))
	}
}

var _ = http.MethodGet
