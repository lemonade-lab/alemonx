package robot

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"alemonx/internal/system"
)

func TestObservePM2RuntimeUsesRunningExecutable(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("process image lookup unavailable")
	}
	for _, name := range []string{"node", "bun"} {
		t.Run(name, func(t *testing.T) {
			path, err := system.ResolveCommand(name)
			if err != nil {
				t.Skipf("%s is unavailable", name)
			}
			output, err := exec.Command(path, "--version").Output()
			if err != nil {
				t.Fatal(err)
			}
			expected := strings.TrimPrefix(strings.TrimSpace(string(output)), "v")
			child := exec.Command(path, "-e", "setInterval(() => {}, 1000)")
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
			process := PM2Process{PID: child.Process.Pid, Status: "online", Uptime: time.Now().UnixMilli()}
			if name == "bun" {
				process.nodeVersion = "24.0.0"
			}
			observePM2Runtime(&process)
			if process.Runtime != name || process.RuntimeVersion != expected {
				t.Fatalf("observed %s %s, want %s %s", process.Runtime, process.RuntimeVersion, name, expected)
			}
			process.Status, process.Runtime, process.RuntimeVersion = "stopped", "", ""
			observePM2Runtime(&process)
			if process.Runtime != "" || process.RuntimeVersion != "" {
				t.Fatal("stopped process must not report a running runtime")
			}
		})
	}
}
