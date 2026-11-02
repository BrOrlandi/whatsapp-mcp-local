//go:build !darwin && !windows

package panel

import "context"

// There is no Claude Desktop for Linux to close and open.
const desktopControl = false

func desktopRunning() bool                  { return false }
func quitDesktop(ctx context.Context) error { return errDesktopStillOpen }
func desktopLaunchTarget() string           { return "" }
func openDesktop(string) error              { return nil }
