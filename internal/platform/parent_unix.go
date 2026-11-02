//go:build !windows

package platform

// waitProcess cannot wait on a process that is not a child here; the
// parent is noticed gone when this process is handed to another.
func waitProcess(int) bool { return false }
