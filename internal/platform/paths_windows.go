package platform

import (
	"os"
	"path/filepath"
)

func dataDir() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		dir, err := os.UserCacheDir() // %LOCALAPPDATA% by another name
		if err != nil {
			return "", err
		}
		base = dir
	}
	return filepath.Join(base, AppName), nil
}

func logDir() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "logs"), nil
}
