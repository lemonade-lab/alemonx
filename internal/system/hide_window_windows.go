//go:build windows

package system

import (
	"os/exec"

	"alemonx/internal/processutil"
)

// HideWindow prevents a background child process from creating a visible
// console window on Windows. Call it before Start, Run, Output, or
// CombinedOutput when a command is launched by the workbench rather than a
// user-facing terminal.
func HideWindow(command *exec.Cmd) {
	processutil.HideWindow(command)
}
