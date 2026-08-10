//go:build !windows

package web

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// configureManagedProcess makes the command and all of its descendants a
// private process group. This lets Stop terminate Yarn and the process it
// spawned, instead of leaving the robot orphaned after only Yarn exits.
func configureManagedProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func interruptManagedProcess(command *exec.Cmd) error {
	return syscall.Kill(-command.Process.Pid, syscall.SIGINT)
}

func forceStopManagedProcess(command *exec.Cmd) error {
	return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
}

// processGroupID returns the process-group id for a configured command. The
// command was started with Setpgid, so the group equals its own pid.
func processGroupID(command *exec.Cmd) int {
	if command.Process == nil {
		return 0
	}
	return command.Process.Pid
}

// processGroupAlive reports whether a process group still exists.
func processGroupAlive(pgid int) bool {
	err := syscall.Kill(-pgid, 0)
	return err == nil || err == syscall.EPERM
}

// killProcessGroup forcibly terminates every member of a process group.
func killProcessGroup(pgid int) {
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}

// portListenerPIDs returns the PIDs of TCP listeners bound to port on any
// interface. lsof is a standard macOS/Linux tool; when it is unavailable the
// caller still knows the port is occupied from the bind probe.
func portListenerPIDs(port int) []int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "lsof", "-ti", "tcp:"+strconv.Itoa(port)).Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		pid, parseErr := strconv.Atoi(strings.TrimSpace(line))
		if parseErr == nil && pid > 0 {
			pids = append(pids, pid)
		}
	}
	return pids
}

// processDescription returns a short command-line description of a process.
func processDescription(pid int) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// processWorkingDirectory returns the process's current working directory, or
// "" when it cannot be read. The value is used to recognise a listener that
// belongs to the robot directory being started.
func processWorkingDirectory(pid int) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(output), "\n") {
		if len(line) > 1 && line[0] == 'n' {
			return strings.TrimSpace(line[1:])
		}
	}
	return ""
}

// processPGID returns the process-group id of a process (0 when unknown).
func processPGID(pid int) int {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return 0
	}
	return pgid
}
