//go:build !windows

package webhook

import "syscall"

// errRefused is how the system says nothing listens at an address.
var errRefused error = syscall.ECONNREFUSED
