package webhook

import "golang.org/x/sys/windows"

// errRefused is how the system says nothing listens at an address: on
// Windows a connection refused comes back as WSAECONNREFUSED, which is not
// syscall.ECONNREFUSED.
var errRefused error = windows.WSAECONNREFUSED
