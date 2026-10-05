package platform

import "syscall"

// setParentDeathSignal makes the kernel send SIGTERM to the child when the app
// dies, however it dies: wacli then closes its store as on any stop.
func setParentDeathSignal(attr *syscall.SysProcAttr) {
	attr.Pdeathsig = syscall.SIGTERM
}
