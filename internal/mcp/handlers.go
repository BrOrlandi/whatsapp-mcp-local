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

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/index"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
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
	Link       bool     `json:"link"`
	Model      string   `json:"model"`

	MaxSilenceHours float64 `json:"max_silence_hours"`

	ReplyTo  string   `json:"reply_to"`
	Mentions []string `json:"mentions"`
	DryRun   bool     `json:"dry_run"`
	ForMe    bool     `json:"for_me"`
	Typing   *bool    `json:"typing"`
	Audio    bool     `json:"audio"`
	Receipts *bool    `json:"receipts"`

	Fields          []string `json:"fields"`
	MaxContentChars int      `json:"max_content_chars"`
	CountOnly       bool     `json:"count_only"`
	Before          *int     `json:"before"`
	After           *int     `json:"after"`
	GroupBy         string   `json:"group_by"`
	Direction       string   `json:"direction"`
	MediaType       string   `json:"media_type"`
	ExcludeGroups   bool     `json:"exclude_groups"`

	IncludeGroups        bool     `json:"include_groups"`
	IncludeGroupMentions bool     `json:"include_group_mentions"`
	IncludeMuted         *bool    `json:"include_muted"`
	IncludeArchived      bool     `json:"include_archived"`
	IncludeHandled       bool     `json:"include_handled"`
	IgnoreClosing        *bool    `json:"ignore_closing"`
	MinAgeHours          float64  `json:"min_age_hours"`
	OnlyUnanswered       bool     `json:"only_unanswered"`
	PerChat              int      `json:"per_chat"`
	Note                 string   `json:"note"`
	Clear                bool     `json:"clear"`
	Participants         []string `json:"participants"`
	Description          *string  `json:"description"`
	Reset                bool     `json:"reset"`

	OlderThan   int `json:"older_than_days"`
	MinMegabyte int `json:"min_megabytes"`
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
	handler, ok := s.handlers()[params.Name]
	if !ok {
		return toolError("unknown tool %q", params.Name)
	}
	return handler(ctx, a)
}

// handlers maps each tool to the method that answers it.
func (s *Server) handlers() map[string]func(context.Context, arguments) map[string]any {
	return map[string]func(context.Context, arguments) map[string]any{
		"health":              s.health,
		"whatsapp_status":     s.status,
		"list_chats":          s.listChats,
		"get_chat_messages":   s.chatMessages,
		"search_messages":     s.searchMessages,
		"list_contacts":       s.listContacts,
		"list_groups":         s.listGroups,
		"get_group":           s.getGroup,
		"send_text_message":   s.sendText,
		"send_media_message":  s.sendMedia,
		"download_media":      s.downloadMedia,
		"transcribe_audio":    s.transcribeAudio,
		"save_transcript":     s.saveTranscript,
		"sync_history":        s.syncHistory,
		"delete_message":      s.deleteMessage,
		"edit_message":        s.editMessage,
		"react_to_message":    s.react,
		"check_numbers":       s.checkNumbers,
		"get_profile_picture": s.profilePicture,
		"send_location":       s.sendLocation,
		"send_poll":           s.sendPoll,
		"get_poll_results":    s.pollResults,
		"organise_chat":       s.organiseChat,

		"forward_message": s.forwardMessage,
		"mark_chat_read":  s.markChatRead,
		"send_typing":     s.sendTyping,

		"get_message_context": s.messageContext,
		"message_stats":       s.messageStats,
		"export_messages":     s.exportMessages,

		"list_unread":     s.listUnread,
		"list_unanswered": s.listUnanswered,
		"list_mentions":   s.listMentions,
		"mark_handled":    s.markHandled,
		"snooze_chat":     s.snoozeChat,

		"manage_group_participants": s.manageParticipants,
		"update_group":              s.updateGroup,
		"get_group_invite_link":     s.groupInviteLink,
		"leave_group":               s.leaveGroup,

		"media_stats": s.mediaStats,
		"purge_media": s.purgeMedia,
	}
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
	// TextTruncated says max_content_chars cut the text or the transcript.
	TextTruncated bool `json:"text_truncated,omitempty"`
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
	sh, err := newShaping(a, chatFields)
	if err != nil {
		return toolError("%v", err)
	}
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
	var rows any = chats
	if len(sh.fields) > 0 {
		picked := make([]map[string]any, 0, len(chats))
		for _, c := range chats {
			picked = append(picked, sh.pick(c))
		}
		rows = picked
	}
	return textResult(map[string]any{"chats": rows, "count": len(chats), "history_since": coverage.Oldest}, false)
}

func (s *Server) chatMessages(ctx context.Context, a arguments) map[string]any {
	if strings.TrimSpace(a.ChatJID) == "" {
		return toolError("chat_jid is required")
	}
	sh, err := newShaping(a, messageFields)
	if err != nil {
		return toolError("%v", err)
	}
	if a.CountOnly {
		return s.countChatMessages(ctx, a)
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
	result := map[string]any{"chat_jid": a.ChatJID, "chat_name": s.index.Names.Name(ctx, a.ChatJID), "messages": sh.messages(msgs), "count": len(msgs), "untrusted_content": UntrustedContent}
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
	sh, err := newShaping(a, messageFields)
	if err != nil {
		return toolError("%v", err)
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
	return textResult(map[string]any{"query": q, "messages": sh.messages(msgs), "count": len(msgs), "full_text": out.FTS,
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
	args := []string{"send", "text", "--to", a.To, "--message", a.Text}
	draft := map[string]any{"text": a.Text}
	reply, fail := s.replyArgs(ctx, a.ReplyTo, a.To)
	if fail != nil {
		return fail
	}
	if reply != nil {
		args = append(args, reply.args...)
		draft["reply_to"] = reply.quoted
	}
	mentions, err := mentionArgs(a.Text, a.Mentions)
	if err != nil {
		return toolError("%v", err)
	}
	if len(mentions) > 0 {
		args = append(args, mentions...)
		draft["mentions"] = a.Mentions
	}
	if a.DryRun {
		return s.dryRun(ctx, a.To, draft)
	}
	return sendResult(s.delegated(ctx, args...))
}

const maxMedia = 100 << 20

func (s *Server) sendMedia(ctx context.Context, a arguments) map[string]any {
	if strings.TrimSpace(a.To) == "" || strings.TrimSpace(a.URL) == "" {
		return toolError("to and url are required")
	}
	as := map[string]string{"image": "image", "video": "video", "audio": "audio", "document": "document", "sticker": "sticker"}[a.Type]
	if as == "" {
		return toolError("type must be image, video, audio, document or sticker")
	}
	sticker := as == "sticker"
	if sticker && (a.Caption != "" || a.Filename != "") {
		return toolError("a sticker carries no caption or file name")
	}
	reply, fail := s.replyArgs(ctx, a.ReplyTo, a.To)
	if fail != nil {
		return fail
	}
	if a.DryRun {
		// A dry run checks the source without downloading it.
		draft := map[string]any{"type": as, "file": a.URL, "caption": a.Caption, "filename": a.Filename}
		if reply != nil {
			draft["reply_to"] = reply.quoted
		}
		if src := strings.TrimPrefix(strings.TrimSpace(a.URL), "file://"); filepath.IsAbs(src) {
			info, err := os.Stat(src)
			if err != nil || info.IsDir() || info.Size() > maxMedia {
				return toolError("%s is not a readable file of at most 100 MiB", src)
			}
			if sticker && !isWebP(src) {
				return toolError("a sticker must be a WebP image, and %s is not one", src)
			}
			draft["bytes"] = info.Size()
		}
		return s.dryRun(ctx, a.To, draft)
	}
	path, cleanup, err := s.fetchFile(ctx, a.URL, a.Filename)
	if err != nil {
		return toolError("%v", err)
	}
	defer cleanup()
	if sticker {
		if !isWebP(path) {
			return toolError("a sticker must be a WebP image, and %s is not one", a.URL)
		}
		args := []string{"send", "sticker", "--to", a.To, "--file", path}
		if reply != nil {
			args = append(args, reply.args...)
		}
		return sendResult(s.delegated(ctx, args...))
	}
	args := []string{"send", "file", "--to", a.To, "--file", path, "--as", as}
	if a.Caption != "" {
		args = append(args, "--caption", a.Caption)
	}
	if a.Filename != "" {
		args = append(args, "--filename", a.Filename)
	}
	if reply != nil {
		args = append(args, reply.args...)
	}
	return sendResult(s.delegated(ctx, args...))
}

// isWebP reports whether a file starts like a WebP image, the only format
// WhatsApp shows as a sticker.
func isWebP(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 12)
	if _, err := io.ReadFull(f, head); err != nil {
		return false
	}
	return string(head[:4]) == "RIFF" && string(head[8:]) == "WEBP"
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
	// Without a sender, whatsmeow keys the reaction as if the target were the
	// account's own message: in a DM a reaction to a received message then
	// points at nothing, and WhatsApp accepts it without showing it.
	sender := m.SenderJID
	if sender == "" && !m.FromMe && !strings.HasSuffix(m.ChatJID, "@g.us") {
		sender = m.ChatJID
	}
	if sender != "" && (!m.FromMe || strings.HasSuffix(m.ChatJID, "@g.us")) {
		args = append(args, "--sender", sender)
	}
	return sendResult(s.delegated(ctx, args...))
}

func (s *Server) deleteMessage(ctx context.Context, a arguments) map[string]any {
	m, fail := s.message(ctx, a)
	if fail != nil {
		return fail
	}
	if a.ForMe {
		if !a.Confirm {
			return textResult(map[string]any{"preview": true, "for_me": true, "would_delete": m,
				"next": "call delete_message again with for_me and confirm true to remove it from this account's devices; the other side keeps it, and this cannot be undone"}, false)
		}
		var out map[string]any
		if err := s.exclusive(ctx, "delete message for me", &out, "messages", "delete", "--chat", m.ChatJID, "--id", m.ID, "--for-me"); err != nil {
			return toolError("%v", err)
		}
		return textResult(map[string]any{"deleted": true, "for_me": true, "message_id": m.ID, "chat_jid": m.ChatJID}, false)
	}
	if !m.FromMe {
		return toolError("only the account's own messages can be deleted for everyone; this one was sent by %s. for_me true removes it from this account only", m.SenderJID)
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
	out, err := s.delegatedOrExclusive(ctx, a.Action+" chat", "chats", a.Action, "--chat", a.ChatJID)
	if err != nil {
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
			return d, fmt.Errorf("WhatsApp's servers no longer hold this file (%v); the phone may still have it, where the user can open or forward it", err)
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
		url := s.base() + "/media/" + token
		result["url"] = url
		result["expires_in_seconds"] = 600
		result["curl"] = fmt.Sprintf("curl -sSf -o %q %q", filepath.Base(d.Path), url)
		return textResult(result, false)
	}
	if d.Bytes > 20<<20 {
		// Too large to travel inside a tool result: hand out a link instead.
		token := s.links.add(d.Path, m.MimeType, 10*time.Minute)
		url := s.base() + "/media/" + token
		result["url"], result["expires_in_seconds"] = url, 600
		result["curl"] = fmt.Sprintf("curl -sSf -o %q %q", filepath.Base(d.Path), url)
		result["note"] = "the file is larger than 20 MiB, so it is not inlined: read it from path, or download it from url"
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
