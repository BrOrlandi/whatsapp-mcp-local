//go:build !linux && !windows

package platform

import "syscall"

func setParentDeathSignal(*syscall.SysProcAttr) {}
