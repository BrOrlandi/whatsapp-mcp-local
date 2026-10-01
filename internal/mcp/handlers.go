package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/index"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/state"
)

// arguments is the shared decoding of every tool input.
type arguments struct {
	Search     string   `json:"search"`
	Query      string   `json:"query"`
	ChatJID    string   `json:"chat_jid"`
	GroupJID   string   `json:"group_jid"`
	MessageID  string   `json:"message_id"`
	To         string   `json:"to"`
	Text       string   `json:"text"`
	Type       string   `json:"type"`
	URL        string   `json:"url"`
	Caption    string   `json:"caption"`
	Filename   string   `json:"filename"`
	Since      string   `json:"since"`
	Until      string   `json:"until"`
	Order      string   `json:"order"`
	Limit      int      `json:"limit"`
	Count      int      `json:"count"`
	Rounds     int      `json:"rounds"`
	Chats      int      `json:"chats"`
	Emoji      *string  `json:"emoji"`
	Confirm    bool     `json:"confirm"`
	Live       bool     `json:"live"`
	Numbers    []string `json:"numbers"`
	Full       bool     `json:"full"`
	Latitude   *float64 `json:"latitude"`
	Longitude  *float64 `json:"longitude"`
	Name       string   `json:"name"`
	Address    string   `json:"address"`
	Question   string   `json:"question"`
	Options    []string `json:"options"`
	MaxAnswers int      `json:"max_answers"`
	Action     string   `json:"action"`
	Language   string   `json:"language"`
	Refresh    bool     `json:"refresh"`
	APIKey     string   `json:"api_key"`
	Remove     bool     `json:"remove"`
	Link       bool     `json:"link"`
	Engine     string   `json:"engine"`
	Model      string   `json:"model"`

	MaxSilenceHours float64 `json:"max_silence_hours"`
}

const (
	readTimeout = 60 * time.Second
	sendTimeout = 2 * time.Minute
	liveTimeout = 90 * time.Second
)

func (s *Server) call(ctx context.Context, params callParams) map[string]any {
	var a arguments
	if len(params.Arguments) > 0 && string(params.Arguments) != "null" {
		if err := json.Unmarshal(params.Arguments, &a); err != nil {
			return toolError("could not read the tool arguments: %v", err)
		}
	}
	handlers := map[string]func(context.Context, arguments) map[string]any{
		"health":                s.health,
		"whatsapp_status":       s.status,
		"list_chats":            s.listChats,
		"get_chat_messages":     s.chatMessages,
		"search_messages":       s.searchMessages,
		"list_contacts":         s.listContacts,
		"list_groups":           s.listGroups,
		"get_group":             s.getGroup,
		"send_text_message":     s.sendText,
		"send_media_message":    s.sendMedia,
		"download_media":        s.downloadMedia,
		"transcribe_audio":      s.transcribeAudio,
		"save_transcript":       s.saveTranscript,
		"set_transcription_key": s.setTranscriptionKey,
		"sync_history":          s.syncHistory,
		"delete_message":        s.deleteMessage,
		"edit_message":          s.editMessage,
		"react_to_message":      s.react,
		"check_numbers":         s.checkNumbers,
		"get_profile_picture":   s.profilePicture,
		"send_location":         s.sendLocation,
		"send_poll":             s.sendPoll,
		"get_poll_results":      s.pollResults,
		"organise_chat":         s.organiseChat,
	}
	handler, ok := handlers[params.Name]
	if !ok {
		return toolError("unknown tool %q", params.Name)
	}
	return handler(ctx, a)
}

func limit(n, fallback int) int {
	if n <= 0 {
		return fallback
	}
	return min(n, 500)
}

func parseTime(value, name string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if _, err := time.Parse(time.RFC3339, value); err == nil {
		return value, nil
	}
	if _, err := time.Parse("2006-01-02", value); err == nil {
		return value, nil
	}
	return "", fmt.Errorf("%s must be an RFC 3339 time such as 2026-09-01T00:00:00Z", name)
}

// ---- reading ----

// wacliMessage is a message as `wacli --json messages list|search` prints it.
type wacliMessage struct {
	ChatJID       string
	ChatName      string
	MsgID         string
	SenderJID     string
	SenderName    string
	Timestamp     time.Time
	FromMe        bool
	Text          string
	DisplayText   string
	QuotedMsgID   string `json:"quoted_msg_id"`
	IsForwarded   bool
	ReactionToID  string
	ReactionEmoji string
	MediaType     string
	MediaCaption  string
	Filename      string
	MimeType      string
	Starred       bool
	Edited        bool
	Revoked       bool
	Snippet       string
}

type message struct {
	ID         string            `json:"id"`
	ChatJID    string            `json:"chat_jid"`
	ChatName   string            `json:"chat_name,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
	FromMe     bool              `json:"from_me"`
	SenderJID  string            `json:"sender_jid,omitempty"`
	SenderName string            `json:"sender_name,omitempty"`
	Text       string            `json:"text,omitempty"`
	MediaType  string            `json:"media_type,omitempty"`
	MimeType   string            `json:"mime_type,omitempty"`
	Filename   string            `json:"filename,omitempty"`
	QuotedID   string            `json:"quoted_message_id,omitempty"`
	ReactionTo string            `json:"reaction_to,omitempty"`
	Reaction   string            `json:"reaction,omitempty"`
	Forwarded  bool              `json:"forwarded,omitempty"`
	Edited     bool              `json:"edited,omitempty"`
	Revoked    bool              `json:"revoked,omitempty"`
	Starred    bool              `json:"starred,omitempty"`
	Snippet    string            `json:"snippet,omitempty"`
	Transcript *state.Transcript `json:"transcript,omitempty"`
}

func convert(m wacliMessage) message {
	text := m.Text
	if m.MediaType != "" && m.MediaCaption != "" {
		text = m.MediaCaption
	} else if m.MediaType != "" && strings.HasPrefix(text, "[") {
		text = ""
	}
	if text == "" && m.MediaType == "" {
		text = m.DisplayText
	}
	return message{ID: m.MsgID, ChatJID: m.ChatJID, ChatName: m.ChatName, Timestamp: m.Timestamp.UTC(), FromMe: m.FromMe,
		SenderJID: m.SenderJID, SenderName: m.SenderName, Text: text, MediaType: m.MediaType, MimeType: m.MimeType,
		Filename: m.Filename, QuotedID: m.QuotedMsgID, ReactionTo: m.ReactionToID, Reaction: m.ReactionEmoji,
		Forwarded: m.IsForwarded, Edited: m.Edited, Revoked: m.Revoked, Starred: m.Starred, Snippet: m.Snippet}
}

// name fills in who wrote a message and where, from the address book, when
// wacli stored an identifier or nothing.
func (s *Server) name(ctx context.Context, m *message) {
	if m.ChatName == "" || index.IsIdentifier(m.ChatName) {
		m.ChatName = s.index.Names.Name(ctx, m.ChatJID)
	}
	if m.FromMe {
		m.SenderName = ""
		return
	}
	if known := s.index.Names.Known(ctx, m.SenderJID); known != "" {
		m.SenderName = known
	} else if m.SenderName == "" || index.IsIdentifier(m.SenderName) {
		m.SenderName = s.index.Names.Name(ctx, m.SenderJID)
	}
}

func (s *Server) attachTranscripts(ctx context.Context, msgs []message) {
	for i := range msgs {
		if msgs[i].MediaType != "audio" {
			continue
		}
		if t, err := s.state.Transcript(ctx, msgs[i].ChatJID, msgs[i].ID); err == nil {
			msgs[i].Transcript = &t
		}
	}
}

func (s *Server) listChats(ctx context.Context, a arguments) map[string]any {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	args := []string{"--read-only", "chats", "list", "--limit", strconv.Itoa(limit(a.Limit, 50))}
	if q := strings.TrimSpace(a.Search); q != "" {
		args = append(args, "--query", q)
	}
	var chats []map[string]any
	if err := s.cli.Decode(ctx, &chats, args...); err != nil {
		return toolError("%v", err)
	}
	if chats == nil {
		chats = []map[string]any{}
	}
	for _, c := range chats {
		jid, _ := c["jid"].(string)
		if name, _ := c["name"].(string); jid != "" && (name == "" || index.IsIdentifier(name)) {
			c["name"] = s.index.Names.Name(ctx, jid)
		}
	}
	coverage, _ := s.index.Coverage(ctx)
	return textResult(map[string]any{"chats": chats, "count": len(chats), "history_since": coverage.Oldest}, false)
}

func (s *Server) chatMessages(ctx context.Context, a arguments) map[string]any {
	if strings.TrimSpace(a.ChatJID) == "" {
		return toolError("chat_jid is required")
	}
	since, err := parseTime(a.Since, "since")
	if err != nil {
		return toolError("%v", err)
	}
	until, err := parseTime(a.Until, "until")
	if err != nil {
		return toolError("%v", err)
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	args := []string{"--read-only", "messages", "list", "--chat", a.ChatJID, "--limit", strconv.Itoa(limit(a.Limit, 100))}
	if since != "" {
		args = append(args, "--after", since)
	}
	if until != "" {
		args = append(args, "--before", until)
	}
	if a.Order == "oldest" {
		args = append(args, "--asc")
	}
	var out struct {
		Messages []wacliMessage `json:"messages"`
	}
	if err := s.cli.Decode(ctx, &out, args...); err != nil {
		return toolError("%v", err)
	}
	msgs := make([]message, 0, len(out.Messages))
	for _, m := range out.Messages {
		msg := convert(m)
		s.name(ctx, &msg)
		msgs = append(msgs, msg)
	}
	s.attachTranscripts(ctx, msgs)
	result := map[string]any{"chat_jid": a.ChatJID, "chat_name": s.index.Names.Name(ctx, a.ChatJID), "messages": msgs, "count": len(msgs), "untrusted_content": UntrustedContent}
	if oldest, err := s.index.ChatOldest(ctx, a.ChatJID); err == nil {
		result["history_since"] = oldest
		if oldest != nil && since != "" {
			if t, err := time.Parse(time.RFC3339, since); err == nil && t.Before(*oldest) {
				result["note"] = "the period starts before the oldest message this machine holds for this chat; sync_history can ask the phone for older ones"
			}
		}
	}
	if gaps := s.gapsWithin(ctx, since, until); len(gaps) > 0 {
		result["unknown_windows"] = gaps
		result["unknown_windows_note"] = "no conversation at all received a message in these windows, which is what a stopped sync looks like; silence inside them is unknown, not empty"
	}
	return textResult(result, false)
}

// gapsWithin returns the store-wide silent windows that overlap a period.
func (s *Server) gapsWithin(ctx context.Context, since, until string) []index.Gap {
	gaps, err := s.index.Gaps(ctx, gapThreshold, 90*24*time.Hour, 50)
	if err != nil {
		return nil
	}
	from, _ := time.Parse(time.RFC3339, since)
	to, errTo := time.Parse(time.RFC3339, until)
	if errTo != nil {
		to = time.Now()
	}
	var out []index.Gap
	for _, g := range gaps {
		if g.Until.After(from) && g.From.Before(to) {
			out = append(out, g)
		}
	}
	return out
}

// gapThreshold is how long the whole account can be silent before it counts
// as a hole: longer than an ordinary night.
const gapThreshold = 10 * time.Hour

func (s *Server) searchMessages(ctx context.Context, a arguments) map[string]any {
	q := strings.TrimSpace(a.Query)
	if q == "" {
		return toolError("query is required")
	}
	since, err := parseTime(a.Since, "since")
	if err != nil {
		return toolError("%v", err)
	}
	until, err := parseTime(a.Until, "until")
	if err != nil {
		return toolError("%v", err)
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	n := limit(a.Limit, 50)
	args := []string{"--read-only", "messages", "search", q, "--limit", strconv.Itoa(n)}
	if a.ChatJID != "" {
		args = append(args, "--chat", a.ChatJID)
	}
	if since != "" {
		args = append(args, "--after", since)
	}
	if until != "" {
		args = append(args, "--before", until)
	}
	var out struct {
		Messages []wacliMessage `json:"messages"`
		FTS      bool           `json:"fts"`
	}
	if err := s.cli.Decode(ctx, &out, args...); err != nil {
		return toolError("%v", err)
	}
	msgs := make([]message, 0, len(out.Messages))
	seen := map[string]bool{}
	for _, m := range out.Messages {
		msg := convert(m)
		s.name(ctx, &msg)
		msgs = append(msgs, msg)
		seen[m.ChatJID+"/"+m.MsgID] = true
	}
	// What was said in a voice note is searchable once it is transcribed.
	if ts, err := s.state.SearchTranscripts(ctx, q, a.ChatJID, n); err == nil {
		for _, t := range ts {
			if seen[t.ChatJID+"/"+t.MessageID] {
				continue
			}
			m, err := s.index.MessageByID(ctx, t.MessageID, t.ChatJID)
			if err != nil {
				continue
			}
			msgs = append(msgs, message{ID: m.ID, ChatJID: m.ChatJID, ChatName: m.ChatName, Timestamp: m.Timestamp, FromMe: m.FromMe,
				SenderJID: m.SenderJID, MediaType: m.MediaType})
		}
	}
	s.attachTranscripts(ctx, msgs)
	coverage, _ := s.index.Coverage(ctx)
	return textResult(map[string]any{"query": q, "messages": msgs, "count": len(msgs), "full_text": out.FTS,
		"history_since": coverage.Oldest, "untrusted_content": UntrustedContent}, false)
}

func (s *Server) listContacts(ctx context.Context, a arguments) map[string]any {
	// The phone's address book lives in the session; wacli's own contact rows
	// are mostly nameless until it copies it over.
	contacts := s.index.Names.Contacts(ctx, a.Search, limit(a.Limit, 100))
	if contacts == nil {
		contacts = []index.Contact{}
	}
	return textResult(map[string]any{"contacts": contacts, "count": len(contacts)}, false)
}

func (s *Server) listGroups(ctx context.Context, a arguments) map[string]any {
	groups, err := s.index.Groups(ctx, a.Search, limit(a.Limit, 100))
	if err != nil {
		return toolError("%v", err)
	}
	if groups == nil {
		groups = []index.Group{}
	}
	return textResult(map[string]any{"groups": groups, "count": len(groups)}, false)
}

func (s *Server) getGroup(ctx context.Context, a arguments) map[string]any {
	jid := strings.TrimSpace(a.GroupJID)
	if !strings.HasSuffix(jid, "@g.us") {
		return toolError("group_jid must be a group JID ending in @g.us")
	}
	if a.Live {
		ctx, cancel := context.WithTimeout(ctx, liveTimeout)
		err := s.supervisor.Exclusive(ctx, "group info", func(ctx context.Context) error {
			_, err := s.cli.Run(ctx, "groups", "info", "--jid", jid)
			return err
		})
		cancel()
		if err != nil {
			return toolError("could not refresh the group from WhatsApp: %v", err)
		}
	}
	g, err := s.index.Group(ctx, jid, a.Search)
	if err != nil {
		return toolError("%v", err)
	}
	return textResult(g, false)
}

// ---- resolving a message ----

func (s *Server) message(ctx context.Context, a arguments) (index.Message, map[string]any) {
	id := strings.TrimSpace(a.MessageID)
	if id == "" {
		return index.Message{}, toolError("message_id is required")
	}
	m, err := s.index.MessageByID(ctx, id, strings.TrimSpace(a.ChatJID))
	if errors.Is(err, index.ErrNotFound) {
		return m, toolError("no indexed message has id %s; the reading tools return the ids this machine knows", id)
	}
	if err != nil {
		return m, toolError("%v", err)
	}
	return m, nil
}

// ---- sending ----

func (s *Server) delegated(ctx context.Context, args ...string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	var out map[string]any
	err := s.supervisor.Delegated(ctx, func(ctx context.Context) error {
		out = nil
		return s.cli.Decode(ctx, &out, args...)
	})
	return out, err
}

func (s *Server) exclusive(ctx context.Context, reason string, v any, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, liveTimeout)
	defer cancel()
	return s.supervisor.Exclusive(ctx, reason, func(ctx context.Context) error {
		return s.cli.Decode(ctx, v, args...)
	})
}

func sendResult(out map[string]any, err error) map[string]any {
	if err != nil {
		return toolError("%v", err)
	}
	if out == nil {
		out = map[string]any{}
	}
	if _, ok := out["id"]; ok {
		out["message_id"] = out["id"]
		delete(out, "id")
	}
	out["note"] = "accepted by WhatsApp; that is not yet confirmation of delivery"
	return textResult(out, false)
}

func (s *Server) sendText(ctx context.Context, a arguments) map[string]any {
	if strings.TrimSpace(a.To) == "" || a.Text == "" {
		return toolError("to and text are required")
	}
	return sendResult(s.delegated(ctx, "send", "text", "--to", a.To, "--message", a.Text))
}

const maxMedia = 100 << 20

func (s *Server) sendMedia(ctx context.Context, a arguments) map[string]any {
	if strings.TrimSpace(a.To) == "" || strings.TrimSpace(a.URL) == "" {
		return toolError("to and url are required")
	}
	as := map[string]string{"image": "image", "video": "video", "audio": "audio", "document": "document"}[a.Type]
	if as == "" {
		return toolError("type must be image, video, audio or document")
	}
	path, cleanup, err := s.fetchFile(ctx, a.URL, a.Filename)
	if err != nil {
		return toolError("%v", err)
	}
	defer cleanup()
	args := []string{"send", "file", "--to", a.To, "--file", path, "--as", as}
	if a.Caption != "" {
		args = append(args, "--caption", a.Caption)
	}
	if a.Filename != "" {
		args = append(args, "--filename", a.Filename)
	}
	return sendResult(s.delegated(ctx, args...))
}

// fetchFile resolves a send's source to a local file: a path on this machine
// is used as it is, a URL is downloaded to a temporary file.
func (s *Server) fetchFile(ctx context.Context, source, filename string) (string, func(), error) {
	nothing := func() {}
	source = strings.TrimSpace(source)
	if p, ok := strings.CutPrefix(source, "file://"); ok {
		source = p
	}
	if filepath.IsAbs(source) {
		info, err := os.Stat(source)
		if err != nil {
			return "", nothing, fmt.Errorf("cannot read %s: %v", source, err)
		}
		if info.IsDir() || info.Size() > maxMedia {
			return "", nothing, fmt.Errorf("%s is not a file of at most 100 MiB", source)
		}
		return source, nothing, nil
	}
	if !strings.HasPrefix(source, "http://") && !strings.HasPrefix(source, "https://") {
		return "", nothing, errors.New("url must be an http(s) URL or an absolute path on this machine")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return "", nothing, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nothing, fmt.Errorf("could not download %s: %v", source, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", nothing, fmt.Errorf("downloading %s answered %s", source, resp.Status)
	}
	name := filename
	if name == "" {
		name = filepath.Base(req.URL.Path)
	}
	if name == "" || name == "/" || name == "." {
		name = "file"
	}
	dir, err := os.MkdirTemp("", "whatsapp-mcp-send-")
	if err != nil {
		return "", nothing, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, filepath.Base(name))
	f, err := os.Create(path)
	if err != nil {
		cleanup()
		return "", nothing, err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxMedia+1))
	f.Close()
	if err != nil {
		cleanup()
		return "", nothing, err
	}
	if n > maxMedia {
		cleanup()
		return "", nothing, errors.New("the file is larger than WhatsApp's 100 MiB limit")
	}
	return path, cleanup, nil
}

func (s *Server) sendLocation(ctx context.Context, a arguments) map[string]any {
	if strings.TrimSpace(a.To) == "" || a.Latitude == nil || a.Longitude == nil {
		return toolError("to, latitude and longitude are required")
	}
	args := []string{"send", "location", "--to", a.To,
		"--latitude", strconv.FormatFloat(*a.Latitude, 'f', -1, 64), "--longitude", strconv.FormatFloat(*a.Longitude, 'f', -1, 64)}
	// wacli sends the pin's name but has no address field, so the address
	// travels as the second half of the label.
	label := strings.TrimSpace(a.Name)
	if addr := strings.TrimSpace(a.Address); addr != "" {
		if label != "" {
			label += " — " + addr
		} else {
			label = addr
		}
	}
	if label != "" {
		args = append(args, "--name", label)
	}
	return sendResult(s.delegated(ctx, args...))
}

func (s *Server) sendPoll(ctx context.Context, a arguments) map[string]any {
	if strings.TrimSpace(a.To) == "" || strings.TrimSpace(a.Question) == "" {
		return toolError("to and question are required")
	}
	if len(a.Options) < 2 || len(a.Options) > 12 {
		return toolError("a poll needs two to twelve options")
	}
	args := []string{"send", "poll", "--to", a.To, "--question", a.Question}
	for _, o := range a.Options {
		args = append(args, "--option", o)
	}
	if a.MaxAnswers > 1 {
		args = append(args, "--multi", strconv.Itoa(min(a.MaxAnswers, len(a.Options))))
	}
	return sendResult(s.delegated(ctx, args...))
}

func (s *Server) editMessage(ctx context.Context, a arguments) map[string]any {
	if a.Text == "" {
		return toolError("text is required")
	}
	m, fail := s.message(ctx, a)
	if fail != nil {
		return fail
	}
	if !m.FromMe {
		return toolError("only the account's own messages can be edited; this one was sent by %s", m.SenderJID)
	}
	return sendResult(s.delegated(ctx, "messages", "edit", "--chat", m.ChatJID, "--id", m.ID, "--message", a.Text))
}

func (s *Server) react(ctx context.Context, a arguments) map[string]any {
	m, fail := s.message(ctx, a)
	if fail != nil {
		return fail
	}
	emoji := "👍"
	if a.Emoji != nil {
		emoji = *a.Emoji
	}
	args := []string{"send", "react", "--to", m.ChatJID, "--id", m.ID, "--reaction", emoji}
	if strings.HasSuffix(m.ChatJID, "@g.us") && m.SenderJID != "" {
		args = append(args, "--sender", m.SenderJID)
	}
	return sendResult(s.delegated(ctx, args...))
}

func (s *Server) deleteMessage(ctx context.Context, a arguments) map[string]any {
	m, fail := s.message(ctx, a)
	if fail != nil {
		return fail
	}
	if !m.FromMe {
		return toolError("only the account's own messages can be revoked; this one was sent by %s", m.SenderJID)
	}
	if !a.Confirm {
		return textResult(map[string]any{"preview": true, "would_delete": m,
			"next": "call delete_message again with confirm true to revoke it for everyone; this cannot be undone"}, false)
	}
	var out map[string]any
	if err := s.exclusive(ctx, "revoke message", &out, "messages", "revoke", "--chat", m.ChatJID, "--id", m.ID); err != nil {
		return toolError("%v", err)
	}
	return textResult(map[string]any{"deleted": true, "message_id": m.ID, "chat_jid": m.ChatJID}, false)
}

// ---- live lookups and chat state ----

func (s *Server) checkNumbers(ctx context.Context, a arguments) map[string]any {
	if len(a.Numbers) == 0 {
		return toolError("numbers is required")
	}
	if len(a.Numbers) > 50 {
		return toolError("check at most 50 numbers at a time")
	}
	var out any
	if err := s.exclusive(ctx, "check numbers", &out, append([]string{"contacts", "check"}, a.Numbers...)...); err != nil {
		return toolError("%v", err)
	}
	return textResult(map[string]any{"results": out, "note": "a number WhatsApp did not answer for is unknown, not confirmed absent"}, false)
}

func (s *Server) profilePicture(ctx context.Context, a arguments) map[string]any {
	if strings.TrimSpace(a.To) == "" {
		return toolError("to is required")
	}
	args := []string{"profile", "picture-info", "--jid", a.To}
	if !a.Full {
		args = append(args, "--preview")
	}
	var out any
	if err := s.exclusive(ctx, "profile picture", &out, args...); err != nil {
		return toolError("%v", err)
	}
	return textResult(out, false)
}

func (s *Server) organiseChat(ctx context.Context, a arguments) map[string]any {
	actions := map[string]bool{"archive": true, "unarchive": true, "pin": true, "unpin": true, "mute": true, "unmute": true}
	if !actions[a.Action] {
		return toolError("action must be one of archive, unarchive, pin, unpin, mute, unmute")
	}
	if strings.TrimSpace(a.ChatJID) == "" {
		return toolError("chat_jid is required")
	}
	var out any
	if err := s.exclusive(ctx, a.Action+" chat", &out, "chats", a.Action, "--chat", a.ChatJID); err != nil {
		return toolError("%v", err)
	}
	return textResult(map[string]any{"done": true, "action": a.Action, "chat_jid": a.ChatJID, "result": out}, false)
}

func (s *Server) pollResults(ctx context.Context, a arguments) map[string]any {
	m, fail := s.message(ctx, a)
	if fail != nil {
		return fail
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	var out any
	if err := s.cli.Decode(ctx, &out, "--read-only", "poll", "show", "--to", m.ChatJID, "--id", m.ID); err != nil {
		return toolError("%v", err)
	}
	return textResult(out, false)
}

// ---- media ----

type downloaded struct {
	Path      string `json:"path"`
	Bytes     int64  `json:"bytes"`
	MediaType string `json:"media_type"`
	MimeType  string `json:"mime_type"`
}

func (s *Server) fetchMedia(ctx context.Context, m index.Message) (downloaded, error) {
	var d downloaded
	if m.MediaType == "" {
		return d, fmt.Errorf("message %s carries no media", m.ID)
	}
	dir := filepath.Join(s.mediaDir, safeName(m.ChatJID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return d, err
	}
	target := filepath.Join(dir, safeName(m.ID)+extension(m))
	if info, err := os.Stat(target); err == nil && info.Size() > 0 {
		return downloaded{Path: target, Bytes: info.Size(), MediaType: m.MediaType, MimeType: m.MimeType}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	// Read-only download takes no store lock, so it runs while sync does.
	if err := s.cli.Decode(ctx, &d, "--read-only", "media", "download", "--chat", m.ChatJID, "--id", m.ID, "--output", target); err != nil {
		if strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "410") {
			return d, fmt.Errorf("WhatsApp's CDN no longer holds this file (%v); the phone may still have it, and `wacli media retry --chat %s` asks it to upload it again", err, m.ChatJID)
		}
		return d, err
	}
	if d.Path == "" {
		d.Path = target
	}
	return d, nil
}

func (s *Server) downloadMedia(ctx context.Context, a arguments) map[string]any {
	m, fail := s.message(ctx, a)
	if fail != nil {
		return fail
	}
	d, err := s.fetchMedia(ctx, m)
	if err != nil {
		return toolError("%v", err)
	}
	result := map[string]any{"message_id": m.ID, "chat_jid": m.ChatJID, "path": d.Path, "bytes": d.Bytes,
		"media_type": m.MediaType, "mime_type": m.MimeType, "filename": m.Filename}
	if a.Link {
		token := s.links.add(d.Path, m.MimeType, 10*time.Minute)
		url := s.baseURL + "/media/" + token
		result["url"] = url
		result["expires_in_seconds"] = 600
		result["curl"] = fmt.Sprintf("curl -sSf -o %q %q", filepath.Base(d.Path), url)
		return textResult(result, false)
	}
	if d.Bytes > 20<<20 {
		result["note"] = "the file is larger than 20 MiB, so it is not inlined; read it from path, or ask again with link true"
		return textResult(result, false)
	}
	body, err := os.ReadFile(d.Path)
	if err != nil {
		return toolError("%v", err)
	}
	meta, _ := json.MarshalIndent(result, "", "  ")
	content := []any{map[string]any{"type": "text", "text": string(meta)}}
	encoded := base64.StdEncoding.EncodeToString(body)
	switch {
	case m.MediaType == "image" || m.MediaType == "sticker":
		content = append(content, map[string]any{"type": "image", "data": encoded, "mimeType": orDefault(m.MimeType, "image/jpeg")})
	case m.MediaType == "audio":
		content = append(content, map[string]any{"type": "audio", "data": encoded, "mimeType": orDefault(m.MimeType, "audio/ogg")})
	default:
		content = append(content, map[string]any{"type": "resource", "resource": map[string]any{
			"uri": "file://" + d.Path, "mimeType": orDefault(m.MimeType, "application/octet-stream"), "blob": encoded}})
	}
	return map[string]any{"content": content, "isError": false}
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == 0 {
			return '_'
		}
		return r
	}, s)
}

func extension(m index.Message) string {
	if ext := filepath.Ext(m.Filename); ext != "" {
		return ext
	}
	switch strings.Split(m.MimeType, ";")[0] {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "audio/ogg":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	case "audio/mp4":
		return ".m4a"
	case "application/pdf":
		return ".pdf"
	}
	return ""
}
