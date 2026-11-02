package platform

import "golang.org/x/sys/windows"

// waitProcess blocks until the process ends, and says whether it could wait.
// Windows keeps a dead parent's id as the parent, so it has to be waited on.
func waitProcess(pid int) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	_, err = windows.WaitForSingleObject(h, windows.INFINITE)
	return err == nil
}
