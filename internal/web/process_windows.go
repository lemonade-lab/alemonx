//go:build windows

package web

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"alemonx/internal/robot"
)

// configureManagedProcess hides the console window that Windows would otherwise
// open for the supervised dev/app process group.
func configureManagedProcess(command *exec.Cmd) {
	robot.HideWindow(command)
}

func interruptManagedProcess(command *exec.Cmd) error {
	return command.Process.Signal(os.Interrupt)
}

func forceStopManagedProcess(command *exec.Cmd) error {
	return command.Process.Kill()
}

// processGroupID returns 0 on Windows; process-group signalling is not used.
func processGroupID(command *exec.Cmd) int {
	return 0
}

func processGroupAlive(_ int) bool { return false }

func killProcessGroup(_ int) {}

// portListenerPIDs returns the PIDs of TCP listeners bound to port on any
// interface by parsing netstat output. The caller still knows the port is
// occupied from the bind probe even when netstat is unavailable.
func portListenerPIDs(port int) []int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "netstat", "-ano").Output()
	if err != nil {
		return nil
	}
	needle := ":" + strconv.Itoa(port)
	seen := map[int]bool{}
	var pids []int
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.EqualFold(fields[0], "tcp") {
			continue
		}
		if !strings.Contains(fields[1], needle) || !strings.Contains(strings.ToUpper(fields[3]), "LISTEN") {
			continue
		}
		pid, parseErr := strconv.Atoi(fields[len(fields)-1])
		if parseErr != nil || pid <= 0 || seen[pid] {
			continue
		}
		seen[pid] = true
		pids = append(pids, pid)
	}
	return pids
}

// processDescription returns the image name of a Windows process.
func processDescription(pid int) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/FO", "CSV", "/NH").Output()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(output))
	if line == "" {
		return ""
	}
	parts := strings.Split(line, ",")
	if len(parts) >= 2 {
		return strings.Trim(parts[0], `"`)
	}
	return line
}

// processWorkingDirectory is not reported on Windows without extra tooling.
func processWorkingDirectory(_ int) string { return "" }

// processPGID is always 0 on Windows; process-group signalling is not used.
func processPGID(_ int) int { return 0 }
