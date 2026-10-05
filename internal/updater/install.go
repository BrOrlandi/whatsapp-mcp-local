package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/platform"
)

// installDMG copies the app out of the new .dmg next to the running one,
// checks its signature (and that it is signed by the same developer), and
// swaps the two bundles. The running app keeps its open files; a helper
// opens the new one once it has quit.
func installDMG(ctx context.Context, dmg, bundle string) error {
	mount, err := os.MkdirTemp("", "wamcp-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(mount)
	if out, err := exec.CommandContext(ctx, "hdiutil", "attach", "-nobrowse", "-readonly", "-noautoopen", "-mountpoint", mount, dmg).CombinedOutput(); err != nil {
		return fmt.Errorf("não foi possível abrir a imagem da versão nova: %s", strings.TrimSpace(string(out)))
	}
	defer exec.Command("hdiutil", "detach", "-force", mount).Run()

	src := filepath.Join(mount, filepath.Base(bundle))
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("a imagem da versão nova não tem %s", filepath.Base(bundle))
	}
	next := bundle + ".new"
	old := bundle + ".old"
	os.RemoveAll(next)
	os.RemoveAll(old)
	if out, err := exec.CommandContext(ctx, "ditto", src, next).CombinedOutput(); err != nil {
		return fmt.Errorf("não foi possível copiar a versão nova: %s", strings.TrimSpace(string(out)))
	}
	if err := sameSigner(bundle, next); err != nil {
		os.RemoveAll(next)
		return err
	}
	if err := os.Rename(bundle, old); err != nil {
		os.RemoveAll(next)
		return fmt.Errorf("não foi possível trocar o app: %w", err)
	}
	if err := os.Rename(next, bundle); err != nil {
		_ = os.Rename(old, bundle)
		return fmt.Errorf("não foi possível trocar o app: %w", err)
	}
	return relaunch(`rm -rf "$OLD"; open "$TARGET"`, map[string]string{"OLD": old, "TARGET": bundle})
}

// sameSigner refuses a new bundle whose signature is broken, or made by a
// different developer than the running one.
func sameSigner(current, next string) error {
	if out, err := exec.Command("codesign", "--verify", "--deep", "--strict", next).CombinedOutput(); err != nil {
		return fmt.Errorf("a assinatura da versão nova não confere: %s", strings.TrimSpace(string(out)))
	}
	have, want := teamID(next), teamID(current)
	if want != "" && have != want {
		return fmt.Errorf("a versão nova foi assinada por outro desenvolvedor (%q, esperado %q)", have, want)
	}
	return nil
}

func teamID(bundle string) string {
	out, _ := exec.Command("codesign", "-dv", bundle).CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(line, "TeamIdentifier="); ok && v != "not set" {
			return v
		}
	}
	return ""
}

// installSetup runs the new installer silently. It waits for this app to
// quit, installs over it, and opens it again.
func installSetup(setup string) error {
	cmd := exec.Command(setup, "/S", "/UPDATE="+strconv.Itoa(os.Getpid()))
	return detach(cmd)
}

// installAppImage replaces the AppImage file, which the running app does not
// need once started, and opens it again after this one quits.
func installAppImage(downloaded, appImage string) error {
	next := appImage + ".new"
	if err := copyFile(downloaded, next, 0o755); err != nil {
		return err
	}
	if err := os.Rename(next, appImage); err != nil {
		os.Remove(next)
		return fmt.Errorf("não foi possível trocar o AppImage: %w", err)
	}
	return relaunch(`exec "$TARGET"`, map[string]string{"TARGET": appImage})
}

// relaunch starts a shell that waits for this process to end and then runs
// script, detached so it outlives the app. It is a variable so tests can
// stop short of starting anything.
var relaunch = func(script string, env map[string]string) error {
	wait := `while kill -0 "$PARENT" 2>/dev/null; do sleep 0.2; done; ` + script
	cmd := exec.Command("/bin/sh", "-c", wait)
	cmd.Env = append(os.Environ(), "PARENT="+strconv.Itoa(os.Getpid()))
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	return detach(cmd)
}

func detach(cmd *exec.Cmd) error {
	platform.Detach(cmd)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return errors.Join(err, os.Remove(dst))
	}
	return out.Close()
}
