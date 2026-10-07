package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/index"
)

// ---- compact reads ----

// messageFields are the keys a message can be trimmed to with fields.
var messageFields = []string{"id", "chat_jid", "chat_name", "timestamp", "from_me", "sender_jid", "sender_name", "text",
	"media_type", "mime_type", "filename", "quoted_message_id", "reaction_to", "reaction", "forwarded", "edited", "revoked",
	"starred", "snippet", "transcript"}

// chatFields are the keys a chat of list_chats can be trimmed to.
var chatFields = []string{"jid", "kind", "name", "last_message_ts", "archived", "pinned", "muted_until", "unread", "unread_count"}

// shaping is how a bulk read is trimmed to what the caller needs: only some
// fields, and long texts cut. A trimmed page costs a fraction of the context.
type shaping struct {
	fields   []string
	maxChars int
}

func newShaping(a arguments, allowed []string) (shaping, error) {
	for _, f := range a.Fields {
		if !slices.Contains(allowed, f) {
			return shaping{}, fmt.Errorf("unknown field %q; the fields are %s", f, strings.Join(allowed, ", "))
		}
	}
	if a.MaxContentChars < 0 {
		return shaping{}, fmt.Errorf("max_content_chars must be positive")
	}
	return shaping{fields: a.Fields, maxChars: a.MaxContentChars}, nil
}

// cut shortens long texts in place, marking each message it cut.
func (sh shaping) cut(msgs []message) {
	if sh.maxChars <= 0 {
		return
	}
	for i := range msgs {
		if t := clip(msgs[i].Text, sh.maxChars); t != msgs[i].Text {
			msgs[i].Text, msgs[i].TextTruncated = t, true
		}
		if tr := msgs[i].Transcript; tr != nil {
			copied := *tr
			if t := clip(copied.Text, sh.maxChars); t != copied.Text {
				copied.Text, copied.Raw = t, ""
				msgs[i].Transcript, msgs[i].TextTruncated = &copied, true
			}
		}
	}
}

// messages returns the messages as they go out: cut, then trimmed to fields.
func (sh shaping) messages(msgs []message) any {
	sh.cut(msgs)
	if len(sh.fields) == 0 {
		return msgs
	}
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, sh.pick(m))
	}
	return out
}

func (sh shaping) message(m message) any {
	one := []message{m}
	sh.cut(one)
	if len(sh.fields) == 0 {
		return one[0]
	}
	return sh.pick(one[0])
}

// pick keeps the chosen keys of a value, and the truncation mark, which says
// the text is not whole.
func (sh shaping) pick(v any) map[string]any {
	body, _ := json.Marshal(v)
	var full map[string]any
	_ = json.Unmarshal(body, &full)
	out := map[string]any{}
	for _, f := range append(sh.fields, "text_truncated") {
		if val, ok := full[f]; ok {
			out[f] = val
		}
	}
	return out
}

// ---- filters ----

// parseWhen reads a moment: RFC 3339, or a date, taken as midnight here.
func parseWhen(value, name string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", value, time.Local); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("%s must be an RFC 3339 time such as 2026-09-01T00:00:00Z, or a date such as 2026-09-01", name)
}

func (s *Server) filter(a arguments) (index.Filter, error) {
	f := index.Filter{ChatJID: strings.TrimSpace(a.ChatJID), ExcludeGroups: a.ExcludeGroups, MediaType: strings.TrimSpace(a.MediaType)}
	var err error
	if f.Since, err = parseWhen(a.Since, "since"); err != nil {
		return f, err
	}
	if f.Until, err = parseWhen(a.Until, "until"); err != nil {
		return f, err
	}
	switch a.Direction {
	case "", "all":
	case "in", "received":
		f.Direction = "in"
	case "out", "sent":
		f.Direction = "out"
	default:
		return f, fmt.Errorf("direction must be in or out")
	}
	return f, nil
}

func rowMessage(r index.Row) message {
	return message{ID: r.ID, ChatJID: r.ChatJID, ChatName: r.ChatName, Timestamp: r.Timestamp, FromMe: r.FromMe, SenderJID: r.SenderJID,
		SenderName: r.SenderName, Text: r.Text, MediaType: r.MediaType, MimeType: r.MimeType, Filename: r.Filename, QuotedID: r.QuotedID,
		ReactionTo: r.ReactionTo, Reaction: r.Reaction, Forwarded: r.Forwarded, Edited: r.Edited, Revoked: r.Revoked}
}

// ---- tools ----

func (s *Server) messageContext(ctx context.Context, a arguments) map[string]any {
	sh, err := newShaping(a, messageFields)
	if err != nil {
		return toolError("%v", err)
	}
	m, fail := s.message(ctx, a)
	if fail != nil {
		return fail
	}
	around := func(v *int) int {
		if v == nil {
			return 5
		}
		return min(max(*v, 0), 50)
	}
	before, after := around(a.Before), around(a.After)
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	var out []wacliMessage
	if err := s.cli.Decode(ctx, &out, "--read-only", "messages", "context", "--chat", m.ChatJID, "--id", m.ID,
		"--before", strconv.Itoa(before), "--after", strconv.Itoa(after)); err != nil {
		return toolError("%v", err)
	}
	msgs := make([]message, 0, len(out))
	for _, w := range out {
		msg := convert(w)
		s.name(ctx, &msg)
		msgs = append(msgs, msg)
	}
	s.attachTranscripts(ctx, msgs)
	at := slices.IndexFunc(msgs, func(x message) bool { return x.ID == m.ID })
	if at < 0 {
		return toolError("wacli returned the conversation around %s without the message itself", m.ID)
	}
	return textResult(map[string]any{"chat_jid": m.ChatJID, "chat_name": s.index.Names.Name(ctx, m.ChatJID),
		"before": sh.messages(msgs[:at]), "message": sh.message(msgs[at]), "after": sh.messages(msgs[at+1:]),
		"untrusted_content": UntrustedContent}, false)
}

func (s *Server) messageStats(ctx context.Context, a arguments) map[string]any {
	f, err := s.filter(a)
	if err != nil {
		return toolError("%v", err)
	}
	groupBy := a.GroupBy
	if groupBy == "" {
		groupBy = "chat"
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	buckets, total, groups, err := s.index.Stats(ctx, f, groupBy, limit(a.Limit, 50))
	if err != nil {
		return toolError("%v", err)
	}
	switch groupBy {
	case "chat":
		for i := range buckets {
			buckets[i].Name = s.index.Names.Name(ctx, buckets[i].Key)
		}
	case "sender":
		buckets = s.mergeSenders(ctx, buckets)
	}
	if buckets == nil {
		buckets = []index.Bucket{}
	}
	result := map[string]any{"group_by": groupBy, "buckets": buckets, "total": total, "groups": groups,
		"truncated": groups > len(buckets), "note": "counts messages, not reactions or deleted ones"}
	if groupBy == "day" || groupBy == "month" {
		zone, _ := time.Now().Zone()
		result["timezone"] = zone
	}
	return textResult(result, false)
}

// mergeSenders joins the buckets of one person counted under their phone
// number and their LID, and names each one.
func (s *Server) mergeSenders(ctx context.Context, buckets []index.Bucket) []index.Bucket {
	merged := map[string]*index.Bucket{}
	var order []string
	for _, b := range buckets {
		key := b.Key
		if key != "me" {
			key = s.index.Names.Canonical(ctx, key)
		}
		if m, ok := merged[key]; ok {
			m.Count += b.Count
			if b.First.Before(m.First) {
				m.First = b.First
			}
			if b.Last.After(m.Last) {
				m.Last = b.Last
			}
			continue
		}
		b.Key = key
		if key == "me" {
			b.Name = "the account itself"
		} else {
			b.Name = s.index.Names.Name(ctx, key)
		}
		merged[key] = &b
		order = append(order, key)
	}
	out := make([]index.Bucket, 0, len(order))
	for _, k := range order {
		out = append(out, *merged[k])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

func (s *Server) exportMessages(ctx context.Context, a arguments) map[string]any {
	f, err := s.filter(a)
	if err != nil {
		return toolError("%v", err)
	}
	if err := os.MkdirAll(s.exportDir, 0o700); err != nil {
		return toolError("%v", err)
	}
	scope := "todas"
	if f.ChatJID != "" {
		scope = safeName(strings.SplitN(f.ChatJID, "@", 2)[0])
	}
	path := filepath.Join(s.exportDir, fmt.Sprintf("whatsapp-%s-%s.ndjson", scope, time.Now().Format("20060102-150405")))
	partial := path + ".partial"
	file, err := os.OpenFile(partial, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return toolError("%v", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	w := bufio.NewWriterSize(file, 256<<10)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	var count int
	var first, last time.Time
	err = s.index.EachMessage(ctx, f, func(r index.Row) error {
		m := rowMessage(r)
		s.name(ctx, &m)
		if m.MediaType == "audio" {
			if t, err := s.state.Transcript(ctx, m.ChatJID, m.ID); err == nil {
				m.Transcript = &t
			}
		}
		if count == 0 {
			first = m.Timestamp
		}
		last = m.Timestamp
		count++
		return enc.Encode(m)
	})
	if err == nil {
		err = w.Flush()
	}
	if cerr := file.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(partial, path)
	}
	if err != nil {
		_ = os.Remove(partial)
		return toolError("the export failed: %v", err)
	}
	info, _ := os.Stat(path)
	result := map[string]any{"path": path, "format": "ndjson", "count": count, "bytes": info.Size(),
		"note": "one JSON message per line, oldest first, reactions and deleted messages left out; read the file from path rather than loading it whole"}
	if count > 0 {
		result["first_timestamp"], result["last_timestamp"] = first, last
	}
	return textResult(result, false)
}

// countChatMessages answers get_chat_messages with count_only: how many
// messages the conversation holds over the period, without reading them.
func (s *Server) countChatMessages(ctx context.Context, a arguments) map[string]any {
	f, err := s.filter(arguments{ChatJID: a.ChatJID, Since: a.Since, Until: a.Until})
	if err != nil {
		return toolError("%v", err)
	}
	n, err := s.index.CountMessages(ctx, f)
	if err != nil {
		return toolError("%v", err)
	}
	result := map[string]any{"chat_jid": f.ChatJID, "chat_name": s.index.Names.Name(ctx, f.ChatJID), "count": n,
		"note": "counts messages, not reactions or deleted ones"}
	if oldest, err := s.index.ChatOldest(ctx, f.ChatJID); err == nil && oldest != nil {
		result["history_since"] = oldest
	}
	return textResult(result, false)
}
