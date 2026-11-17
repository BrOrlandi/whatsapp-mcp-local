package public

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
)

// MinCost is the lowest bcrypt cost accepted for the panel's password.
const MinCost = 10

// HashCost is the cost `whatsapp-mcp password hash` uses.
const HashCost = 12

// PasswordFile is where `whatsapp-mcp password set` keeps the hash, in the
// data folder. WHATSAPP_MCP_PANEL_PASSWORD_HASH wins over it.
const PasswordFile = "panel-password"

// PasswordPath is the password file in a data folder.
func PasswordPath(dataDir string) string { return filepath.Join(dataDir, PasswordFile) }

// MinPasswordLength is the shortest password `password hash` accepts.
const MinPasswordLength = 10

// HashPassword is the bcrypt hash of a password, for the panel.
func HashPassword(password string) (string, error) {
	if len([]rune(password)) < MinPasswordLength {
		return "", errors.New(i18n.Tf("a senha precisa ter ao menos %d caracteres", MinPasswordLength))
	}
	if len(password) > 72 {
		return "", errors.New(i18n.T("a senha pode ter no máximo 72 bytes"))
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), HashCost)
	return string(h), err
}

// CheckHash refuses what is not a bcrypt hash ($2a$, $2b$ or $2y$) of cost
// MinCost or more: a password in plain text, a typo, a hash too cheap.
func CheckHash(hash string) error {
	hash = strings.TrimSpace(hash)
	if !strings.HasPrefix(hash, "$2a$") && !strings.HasPrefix(hash, "$2b$") && !strings.HasPrefix(hash, "$2y$") {
		return errors.New(i18n.T("a senha do painel precisa ser um hash bcrypt ($2a$, $2b$ ou $2y$); gere um com whatsapp-mcp password hash"))
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		return errors.New(i18n.T("o hash bcrypt da senha do painel está malformado; gere outro com whatsapp-mcp password hash"))
	}
	if cost < MinCost {
		return errors.New(i18n.Tf("o hash bcrypt da senha do painel tem custo %d; o mínimo é %d (gere outro com whatsapp-mcp password hash)", cost, MinCost))
	}
	return nil
}

// Password is where the panel's password hash comes from: the environment,
// fixed for the life of the process, or the file `password set` writes,
// read again when it changes, so a new password needs no restart.
type Password struct {
	env  string
	path string

	mu   sync.Mutex
	raw  string
	hash string
	err  error
}

// NewPassword reads the hash from env, when set, or else from the file at
// path.
func NewPassword(env, path string) *Password {
	return &Password{env: strings.TrimSpace(env), path: path}
}

// Current is the hash in force, or an error when there is none or it is not
// valid: then nobody signs in.
func (p *Password) Current() (string, error) {
	if p.env != "" {
		return p.env, CheckHash(p.env)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	// The file is small: it is read on every check, and compared with what
	// was read before (every bcrypt hash has the same size, and a quick
	// rewrite may keep the modification time).
	raw, err := os.ReadFile(p.path)
	if err != nil {
		return "", errNoPassword
	}
	if string(raw) == p.raw && (p.hash != "" || p.err != nil) {
		return p.hash, p.err
	}
	p.raw = string(raw)
	p.hash = strings.TrimSpace(p.raw)
	p.err = CheckHash(p.hash)
	if p.err != nil {
		p.hash = ""
	}
	return p.hash, p.err
}

var errNoPassword = errors.New("no panel password")

// fingerprint ties a session to the hash it was made with: a new password
// changes it, and every session made before stops counting.
func fingerprint(hash string) [32]byte { return sha256.Sum256([]byte("panel-session\x00" + hash)) }

// verify reports whether password matches hash. bcrypt's comparison takes
// the same time whatever the password.
func verify(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
