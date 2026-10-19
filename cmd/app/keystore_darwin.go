package main

import (
	"errors"

	"github.com/keybase/go-keychain"
)

// The device's private key for remote access lives in the login keychain,
// for this Mac only: never synced to iCloud, never in a file.
const keychainService = "com.brorlandi.whatsapp-mcp.remote"

func saveDeviceKey(account string, key []byte) error {
	_ = keychain.DeleteGenericPasswordItem(keychainService, account)
	item := keychain.NewGenericPassword(keychainService, account, "WhatsApp MCP remote access", key, "")
	item.SetSynchronizable(keychain.SynchronizableNo)
	item.SetAccessible(keychain.AccessibleAfterFirstUnlockThisDeviceOnly)
	return keychain.AddItem(item)
}

func loadDeviceKey(account string) ([]byte, error) {
	key, err := keychain.GetGenericPassword(keychainService, account, "", "")
	if err != nil {
		return nil, err
	}
	if len(key) == 0 {
		return nil, errors.New("the device's key is not in the keychain")
	}
	return key, nil
}

func deleteDeviceKey(account string) error {
	err := keychain.DeleteGenericPasswordItem(keychainService, account)
	if errors.Is(err, keychain.ErrorItemNotFound) {
		return nil
	}
	return err
}
