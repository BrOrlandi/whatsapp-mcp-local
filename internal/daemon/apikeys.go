package daemon

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/panel"
)

// APIKeysFile, in the data folder, holds the SHA-256 (hex) of each API key
// that may use the MCP by URL, one per line, with an optional name after a
// space; # starts a comment. No file, or no key in it, and the MCP is not
// served by URL. The keys themselves are never kept: they are long random
// secrets, shown once when made.
const APIKeysFile = "api-keys"

// unusedFor is how long a key made here waits for its first use. A key
// nobody used by then stops working and leaves the file, so a key made and
// forgotten does not stay valid. Lines written by hand never expire.
const unusedFor = 24 * time.Hour

// apiKeys keeps the API keys for the panel and the command line: it makes
// one, shown once, lists them by name and revokes one. Lines written by
// hand stay as they are.
type apiKeys struct {
	path string
	now  func() time.Time
	mu   sync.Mutex
}

// apiKeyID is how a key is named in lists and revoked: the start of its
// hash, which says nothing about the key.
func apiKeyID(hash string) string { return hash[:min(len(hash), 12)] }

func (k *apiKeys) read() ([]string, error) {
	raw, err := os.ReadFile(k.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimRight(string(raw), "\n"), "\n"), nil
}

func (k *apiKeys) write(lines []string) error {
	tmp := k.path + ".new"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, k.path)
}

// keyLine is a line of the file:
// "<hash> <name> # created <time> tool <agent> unused". "unused" goes at the
// key's first use; lines with no date (written by hand, or before keys
// expired) are taken as used.
type keyLine struct {
	hash   string
	key    panel.APIKey
	unused bool
}

func parseKeyLine(line string) (keyLine, bool) {
	body, comment, _ := strings.Cut(line, "#")
	fields := strings.Fields(body)
	if len(fields) == 0 {
		return keyLine{}, false
	}
	l := keyLine{hash: strings.ToLower(fields[0])}
	l.key = panel.APIKey{ID: apiKeyID(l.hash), Name: strings.Join(fields[1:], " ")}
	if c := strings.Fields(comment); len(c) >= 2 && c[0] == "created" {
		l.key.Created, _ = time.Parse(time.RFC3339, c[1])
		for i := 2; i < len(c); i++ {
			switch {
			case c[i] == "tool" && i+1 < len(c):
				i++
				l.key.Tool = c[i]
			case c[i] == "unused":
				l.unused = !l.key.Created.IsZero()
			}
		}
	}
	l.key.Used = !l.unused
	if l.unused {
		l.key.Expires = l.key.Created.Add(unusedFor)
	}
	return l, true
}

// expired reports whether the key was never used within unusedFor.
func (l keyLine) expired(now time.Time) bool {
	return l.unused && now.Sub(l.key.Created) >= unusedFor
}

// current reads the file without the expired keys, and drops them from it.
func (k *apiKeys) current() ([]string, error) {
	lines, err := k.read()
	if err != nil {
		return nil, err
	}
	now := k.now()
	kept := lines[:0:0]
	for _, line := range lines {
		if l, ok := parseKeyLine(line); ok && l.expired(now) {
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) < len(lines) {
		if err := k.write(kept); err != nil {
			return nil, err
		}
	}
	return kept, nil
}

// Valid reports whether key is one of the API keys, and whether there is
// any (none: the MCP by URL is off). The file is read on every call, so
// removing a line revokes a key at once. A key's first use is written down,
// so it no longer expires.
func (k *apiKeys) Valid(key string) (valid, on bool, err error) {
	sum := sha256.Sum256([]byte(key))
	want := hex.EncodeToString(sum[:])
	k.mu.Lock()
	defer k.mu.Unlock()
	lines, err := k.current()
	if err != nil {
		return false, true, err
	}
	match := -1
	for i, line := range lines {
		if l, ok := parseKeyLine(line); ok {
			on = true
			if subtle.ConstantTimeCompare([]byte(l.hash), []byte(want)) == 1 {
				match = i
			}
		}
	}
	if match < 0 || len(key) < 32 {
		return false, on, nil
	}
	if l, _ := parseKeyLine(lines[match]); l.unused {
		lines[match] = strings.TrimSuffix(strings.TrimRight(lines[match], " \t\r"), " unused")
		// The key works even if this cannot be written; it is tried
		// again on the next request.
		_ = k.write(lines)
	}
	return true, on, nil
}

func (k *apiKeys) List() ([]panel.APIKey, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	lines, err := k.current()
	if err != nil {
		return nil, err
	}
	out := []panel.APIKey{}
	for _, line := range lines {
		if l, ok := parseKeyLine(line); ok {
			out = append(out, l.key)
		}
	}
	return out, nil
}

// Add makes a key and keeps only its hash. The key is returned once, for
// the person to hand to the agent; it is not kept anywhere.
func (k *apiKeys) Add(name, tool string) (string, panel.APIKey, error) {
	name = strings.Join(strings.Fields(strings.NewReplacer("#", " ").Replace(name)), " ")
	if runes := []rune(name); len(runes) > 64 {
		name = string(runes[:64])
	}
	if name == "" {
		return "", panel.APIKey{}, errors.New(i18n.T("dê um nome à chave, como o do agente que vai usá-la"))
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", panel.APIKey{}, err
	}
	secret := "wamcp_" + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(secret))
	hash := hex.EncodeToString(sum[:])
	created := k.now().UTC().Truncate(time.Second)
	k.mu.Lock()
	defer k.mu.Unlock()
	lines, err := k.current()
	if err != nil {
		return "", panel.APIKey{}, err
	}
	line := hash + " " + name + " # created " + created.Format(time.RFC3339)
	if tool = strings.Map(func(r rune) rune {
		if r == '-' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, tool); tool != "" {
		line += " tool " + tool
	}
	lines = append(lines, line+" unused")
	if err := k.write(lines); err != nil {
		return "", panel.APIKey{}, err
	}
	return secret, panel.APIKey{ID: apiKeyID(hash), Name: name, Created: created, Tool: tool, Expires: created.Add(unusedFor)}, nil
}

// Revoke removes a key: the next request with it is refused.
func (k *apiKeys) Revoke(id string) error {
	id = strings.ToLower(strings.TrimSpace(id))
	k.mu.Lock()
	defer k.mu.Unlock()
	lines, err := k.read()
	if err != nil {
		return err
	}
	kept, found := lines[:0:0], false
	for _, l := range lines {
		if key, ok := parseKeyLine(l); ok && len(id) == 12 && subtle.ConstantTimeCompare([]byte(key.key.ID), []byte(id)) == 1 {
			found = true
			continue
		}
		kept = append(kept, l)
	}
	if !found {
		return panel.ErrNoAPIKey
	}
	return k.write(kept)
}
