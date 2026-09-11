//go:build !windows

package system

import "os/exec"

// HideWindow is a no-op on platforms that do not create a console for child
// processes.
func HideWindow(_ *exec.Cmd) {}
