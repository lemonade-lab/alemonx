package main

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureWorkbenchProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}
