package index

import (
	"context"
	"database/sql"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// Names turns JIDs into what the WhatsApp app would show: a group's name, the
// name in the phone's address book, the person's own push name, or, when none
// is known, the phone number.
//
// wacli's index alone is a poor source. It stores a chat's JID as its name when
// it does not know better, and it copies the address book from the session
// only when told to, so most contacts arrive nameless. The session database,
// which whatsmeow keeps current from the phone's address book, knows them.
// It is read here read-only, for names alone; nothing else in it is touched.
type Names struct {
	index   *sql.DB
	session *sql.DB

	mu     sync.Mutex
	loaded time.Time
	byJID  map[string]string
	lidPN  map[string]string
	self   string
}

const namesTTL = 45 * time.Second

func openNames(index *sql.DB, sessionPath string) *Names {
	n := &Names{index: index}
	if sessionPath != "" {
		dsn := "file:" + (&url.URL{Path: sessionPath}).EscapedPath() + "?mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)"
		if db, err := sql.Open("sqlite", dsn); err == nil {
			db.SetMaxOpenConns(2)
			n.session = db
		}
	}
	return n
}

// isJIDLike reports whether a stored "name" is really an identifier: a JID, or
// a bare phone number, which wacli stores in the name column when it has none.
func isJIDLike(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || strings.Contains(s, "@") {
		return true
	}
	for _, r := range s {
		if (r < '0' || r > '9') && !strings.ContainsRune("+-() ", r) {
			return false
		}
	}
	return true
}

func (n *Names) refresh(ctx context.Context) {
	n.mu.Lock()
	fresh := time.Since(n.loaded) < namesTTL
	n.mu.Unlock()
	if fresh {
		return
	}
	byJID := map[string]string{}
	lidPN := map[string]string{}
	var self string
	set := func(jid, name string) {
		jid, name = strings.TrimSpace(jid), strings.TrimSpace(name)
		if jid == "" || isJIDLike(name) {
			return
		}
		if _, ok := byJID[jid]; !ok {
			byJID[jid] = name
		}
	}
	scan := func(db *sql.DB, query string) {
		if db == nil {
			return
		}
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var jid, name string
			if rows.Scan(&jid, &name) == nil {
				set(jid, name)
			}
		}
	}
	// Earlier sources win: a name the user chose beats one the phone gave, and
	// that beats what a sender called themselves.
	scan(n.index, `SELECT jid, alias FROM contact_aliases`)
	scan(n.index, `SELECT jid, COALESCE(name,'') FROM groups`)
	scan(n.index, `SELECT jid, COALESCE(system_name,'') FROM contacts`)
	scan(n.session, `SELECT their_jid, COALESCE(NULLIF(full_name,''), NULLIF(first_name,''), '') FROM whatsmeow_contacts`)
	scan(n.index, `SELECT jid, COALESCE(NULLIF(full_name,''), NULLIF(first_name,''), '') FROM contacts`)
	scan(n.index, `SELECT jid, COALESCE(name,'') FROM chats`)
	scan(n.index, `SELECT chat_jid, chat_name FROM (SELECT chat_jid, chat_name, MAX(ts) FROM messages
		WHERE chat_name IS NOT NULL AND chat_name != '' AND chat_name NOT LIKE '%@%' GROUP BY chat_jid)`)
	scan(n.session, `SELECT their_jid, COALESCE(NULLIF(push_name,''), NULLIF(business_name,''), '') FROM whatsmeow_contacts`)
	scan(n.index, `SELECT jid, COALESCE(NULLIF(push_name,''), NULLIF(business_name,''), '') FROM contacts`)
	scan(n.index, `SELECT sender_jid, sender_name FROM (SELECT sender_jid, sender_name, MAX(ts) FROM messages
		WHERE from_me = 0 AND sender_name IS NOT NULL AND sender_name != '' GROUP BY sender_jid)`)
	if n.session != nil {
		if rows, err := n.session.QueryContext(ctx, `SELECT lid, pn FROM whatsmeow_lid_map`); err == nil {
			for rows.Next() {
				var lid, pn string
				if rows.Scan(&lid, &pn) == nil {
					lidPN[lid] = pn
				}
			}
			rows.Close()
		}
		_ = n.session.QueryRowContext(ctx, `SELECT COALESCE(push_name,'') FROM whatsmeow_device LIMIT 1`).Scan(&self)
	}
	n.mu.Lock()
	n.byJID, n.lidPN, n.self, n.loaded = byJID, lidPN, strings.TrimSpace(self), time.Now()
	n.mu.Unlock()
}

// canonical turns a LID or device JID into the phone-number JID it stands for.
func (n *Names) canonical(jid string) string {
	jid = strings.TrimSpace(jid)
	user, server, ok := strings.Cut(jid, "@")
	if !ok {
		return jid
	}
	user, _, _ = strings.Cut(user, ":")
	if server == "lid" {
		if pn, ok := n.lidPN[user]; ok {
			return pn + "@s.whatsapp.net"
		}
	}
	return user + "@" + server
}

// Name is what to call a JID. It never returns an empty string: with no name
// known, a person is their phone number and anything else its identifier.
func (n *Names) Name(ctx context.Context, jid string) string {
	n.refresh(ctx)
	n.mu.Lock()
	defer n.mu.Unlock()
	if name, ok := n.byJID[jid]; ok {
		return name
	}
	c := n.canonical(jid)
	if name, ok := n.byJID[c]; ok {
		return name
	}
	return formatJID(c)
}

// Known is Name without the fallback: empty when no name is known.
func (n *Names) Known(ctx context.Context, jid string) string {
	n.refresh(ctx)
	n.mu.Lock()
	defer n.mu.Unlock()
	if name, ok := n.byJID[jid]; ok {
		return name
	}
	return n.byJID[n.canonical(jid)]
}

// Self is the paired account's own push name.
func (n *Names) Self(ctx context.Context) string {
	n.refresh(ctx)
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.self
}

func formatJID(jid string) string {
	user, server, _ := strings.Cut(jid, "@")
	switch server {
	case "s.whatsapp.net":
		if strings.HasPrefix(user, "55") && (len(user) == 12 || len(user) == 13) {
			local := user[4:]
			return "+55 (" + user[2:4] + ") " + local[:len(local)-4] + "-" + local[len(local)-4:]
		}
		return "+" + user
	case "g.us":
		return "Grupo sem nome"
	case "newsletter":
		return "Canal do WhatsApp"
	case "broadcast":
		return "Lista de transmissão"
	}
	return user
}

// IsIdentifier reports whether a stored name is really a JID or a bare number.
func IsIdentifier(s string) bool { return isJIDLike(s) }

// Contacts lists the people with a known name, from every source Names reads,
// optionally filtered by a name or number fragment.
func (n *Names) Contacts(ctx context.Context, search string, limit int) []Contact {
	n.refresh(ctx)
	n.mu.Lock()
	defer n.mu.Unlock()
	q := strings.ToLower(strings.TrimSpace(search))
	var out []Contact
	for jid, name := range n.byJID {
		user, server, _ := strings.Cut(jid, "@")
		if server != "s.whatsapp.net" {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(name), q) && !strings.Contains(user, strings.Trim(q, "+ ")) {
			continue
		}
		out = append(out, Contact{JID: jid, Phone: user, Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
