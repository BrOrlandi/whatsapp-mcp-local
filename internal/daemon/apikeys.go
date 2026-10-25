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

// apiKeyValid reports whether key is one of the API keys, and whether there
// is any (none: the MCP by URL is off). The file is read
// on every call, so removing a line revokes a key at once.
func apiKeyValid(path, key string) (valid, on bool, err error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, true, err
	}
	sum := sha256.Sum256([]byte(key))
	want := hex.EncodeToString(sum[:])
	found := 0
	for _, line := range strings.Split(string(raw), "\n") {
		line, _, _ = strings.Cut(line, "#")
		fields := strings.Fields(line)
		if len(fields) > 0 {
			on = true
			found |= subtle.ConstantTimeCompare([]byte(strings.ToLower(fields[0])), []byte(want))
		}
	}
	return found == 1 && len(key) >= 32, on, nil
}

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

// parseKeyLine reads "<hash> <name> # created <time>".
func parseKeyLine(line string) (panel.APIKey, bool) {
	body, comment, _ := strings.Cut(line, "#")
	fields := strings.Fields(body)
	if len(fields) == 0 {
		return panel.APIKey{}, false
	}
	key := panel.APIKey{ID: apiKeyID(strings.ToLower(fields[0])), Name: strings.Join(fields[1:], " ")}
	if at, ok := strings.CutPrefix(strings.TrimSpace(comment), "created "); ok {
		key.Created, _ = time.Parse(time.RFC3339, strings.TrimSpace(at))
	}
	return key, true
}

func (k *apiKeys) List() ([]panel.APIKey, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	lines, err := k.read()
	if err != nil {
		return nil, err
	}
	out := []panel.APIKey{}
	for _, l := range lines {
		if key, ok := parseKeyLine(l); ok {
			out = append(out, key)
		}
	}
	return out, nil
}

// Add makes a key and keeps only its hash. The key is returned once, for
// the person to hand to the agent; it is not kept anywhere.
func (k *apiKeys) Add(name string) (string, panel.APIKey, error) {
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
	lines, err := k.read()
	if err != nil {
		return "", panel.APIKey{}, err
	}
	lines = append(lines, hash+" "+name+" # created "+created.Format(time.RFC3339))
	if err := k.write(lines); err != nil {
		return "", panel.APIKey{}, err
	}
	return secret, panel.APIKey{ID: apiKeyID(hash), Name: name, Created: created}, nil
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
		if key, ok := parseKeyLine(l); ok && len(id) == 12 && subtle.ConstantTimeCompare([]byte(key.ID), []byte(id)) == 1 {
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
