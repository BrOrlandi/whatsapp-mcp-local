//go:build !darwin

package main

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/platform"
)

// Outside macOS, the device's private key for remote access is a file in
// the data folder, readable only by its owner. (A secret service, such as
// the Windows Credential Manager or libsecret, is the next step.)
func deviceKeyPath(account string) (string, error) {
	dir, err := platform.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "remote-device-"+filepath.Base(account)+".key"), nil
}

func saveDeviceKey(account string, key []byte) error {
	path, err := deviceKeyPath(account)
	if err != nil {
		return err
	}
	return os.WriteFile(path, key, 0o600)
}

func loadDeviceKey(account string) ([]byte, error) {
	path, err := deviceKeyPath(account)
	if err != nil {
		return nil, err
	}
	key, err := os.ReadFile(path)
	if len(key) == 0 && err == nil {
		err = errors.New("the device's key is empty")
	}
	return key, err
}

func deleteDeviceKey(account string) error {
	path, err := deviceKeyPath(account)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
