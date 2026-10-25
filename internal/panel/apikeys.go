package panel

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
)

// API keys let an agent outside this computer use the MCP by the server's
// address (internal/remote). The panel makes, lists and revokes them; the
// daemon keeps only their hashes.

// APIKey is a key as it is listed: its id and name, never the key.
type APIKey struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Created time.Time `json:"created_at,omitzero"`
}

// APIKeys keeps the keys; the daemon provides it.
type APIKeys interface {
	List() ([]APIKey, error)
	// Add makes a key named name and returns it, once.
	Add(name string) (string, APIKey, error)
	Revoke(id string) error
}

// ErrNoAPIKey means no key has that id.
var ErrNoAPIKey = errors.New("no API key has that id")

func (p *Panel) registerAPIKeys(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/chaves", p.api(p.apiKeysList))
	mux.HandleFunc("POST /api/chaves", p.api(p.apiKeysAdd))
	mux.HandleFunc("DELETE /api/chaves/{id}", p.api(p.apiKeysRevoke))
}

func (p *Panel) apiKeysList(r *http.Request) (any, error) {
	if p.APIKeys == nil {
		return nil, userError{i18n.T("as chaves de API não estão disponíveis aqui")}
	}
	keys, err := p.APIKeys.List()
	if err != nil {
		return nil, err
	}
	return map[string]any{"keys": keys}, nil
}

func (p *Panel) apiKeysAdd(r *http.Request) (any, error) {
	if p.APIKeys == nil {
		return nil, userError{i18n.T("as chaves de API não estão disponíveis aqui")}
	}
	var body struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	secret, key, err := p.APIKeys.Add(body.Name)
	if err != nil {
		return nil, userError{err.Error()}
	}
	return map[string]any{"key": secret, "id": key.ID, "name": key.Name}, nil
}

func (p *Panel) apiKeysRevoke(r *http.Request) (any, error) {
	if p.APIKeys == nil {
		return nil, userError{i18n.T("as chaves de API não estão disponíveis aqui")}
	}
	err := p.APIKeys.Revoke(strings.TrimSpace(r.PathValue("id")))
	if errors.Is(err, ErrNoAPIKey) {
		return nil, userError{i18n.T("nenhuma chave tem esse id")}
	}
	if err != nil {
		return nil, err
	}
	return map[string]bool{"revoked": true}, nil
}
