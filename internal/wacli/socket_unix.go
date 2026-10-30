//go:build !windows

package wacli

func sendSocketReady(storeDir string) bool {
	return unixSocketReady(storeDir)
}
