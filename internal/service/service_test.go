package service

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A service file carrying the token is readable by its owner only, also
// when it replaces one that was readable by everyone; one without a secret
// stays as it was.
func TestAServiceFileWithASecretIsPrivate(t *testing.T) {
	if !hasSecret(map[string]string{"WHATSAPP_MCP_TOKEN": "x"}) || hasSecret(map[string]string{"WACLI_BIN": "/b", "WHATSAPP_MCP_DATA": "/d", "WHATSAPP_MCP_LANG": "pt-BR", "WHATSAPP_MCP_PORT": "47821"}) {
		t.Fatal("which environments carry a secret")
	}
	if runtime.GOOS == "windows" {
		t.Skip("no service file on Windows")
	}
	path := filepath.Join(t.TempDir(), "whatsapp-mcp.service")
	if err := writeServiceFile(path, []byte("no secret"), false); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o644 {
		t.Fatalf("a unit without a secret: %v", fi.Mode().Perm())
	}
	if err := writeServiceFile(path, []byte("Environment=\"WHATSAPP_MCP_TOKEN=x\""), true); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("a unit with the token: %v", fi.Mode().Perm())
	}
}
