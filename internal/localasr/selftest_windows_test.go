//go:build windows

package localasr

import (
	"os"
	"path/filepath"
	"testing"
)

// The real test on Windows: a program that starts and answers -h passes
// (this test binary prints its flags and exits 0), and one that cannot be
// started is unavailable. A clean Windows without the Visual C++ runtime is
// checked by hand, with the published whisper-cli.exe (docs/transcricao.md).
func TestSelfTestOnWindows(t *testing.T) {
	if !selfTestOn {
		t.Fatal("the engine test is off on Windows")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if reason, known := runSelfTest(self); reason != "" || !known {
		t.Errorf("a program that starts: %q, %v", reason, known)
	}
	missing := filepath.Join(t.TempDir(), "whisper-cli.exe")
	if err := os.WriteFile(missing, []byte("not a program"), 0o755); err != nil {
		t.Fatal(err)
	}
	if reason, known := runSelfTest(missing); reason == "" || !known {
		t.Errorf("a file that is not a program: %q, %v", reason, known)
	}
}
