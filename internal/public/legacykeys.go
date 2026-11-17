package public

import (
	"bufio"
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
)

// LegacyKeysFile, in the data folder, is where the 2.0.0-beta test versions
// kept the API keys of their remote mode: the SHA-256 (hex) of each key, one
// per line, with an optional name after a space and a comment after #
// ("created <time> tool <name> unused"). A server that moves from one of
// those versions keeps its keys: each line becomes a key in state.db, so the
// tools that already hold one go on working at the same address.
const LegacyKeysFile = "api-keys"

// legacyKeyMin is the shortest key those versions accepted.
const legacyKeyMin = 32

// ImportLegacyKeys adds the keys of path that state.db does not have yet,
// and returns how many it added and how many the file holds. The file is left as it is: a key already
// there, revoked ones included, is never added again, so this runs at every
// start. A key those versions still counted as unused keeps its date, and
// expires as it would have.
func ImportLegacyKeys(ctx context.Context, st *state.State, path string, now time.Time) (added, found int, err error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	lines := bufio.NewScanner(f)
	for lines.Scan() {
		body, comment, _ := strings.Cut(lines.Text(), "#")
		fields := strings.Fields(body)
		if len(fields) == 0 {
			continue
		}
		hash := strings.ToLower(fields[0])
		if !isSHA256Hex(hash) {
			continue
		}
		found++
		name := strings.Join(fields[1:], " ")
		if r := []rune(name); len(r) > 60 {
			name = string(r[:60])
		}
		if name == "" {
			name = "API key"
		}
		created, unused := time.Time{}, false
		if c := strings.Fields(comment); len(c) >= 2 && c[0] == "created" {
			created, _ = time.Parse(time.RFC3339, c[1])
			unused = !created.IsZero() && c[len(c)-1] == "unused"
		}
		if created.IsZero() {
			created = now
		}
		k := state.APIKey{ID: "k_" + hash[:12], Name: name, CreatedAt: created.UTC().Truncate(time.Second)}
		used := now
		if unused {
			used = time.Time{}
		}
		ok, err := st.ImportAPIKey(ctx, k, hash, used)
		if err != nil {
			return added, found, err
		}
		if ok {
			added++
		}
	}
	return added, found, lines.Err()
}

func isSHA256Hex(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
