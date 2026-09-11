//go:build windows

// Package processutil contains cross-cutting process-launch helpers that do
// not depend on any product subsystem.
package processutil

import (
	"os/exec"
	"syscall"
)

// HideWindow prevents a background child process from creating a visible
// console window on Windows.
func HideWindow(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
