package robot

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"alemonx/internal/system"
)

var runtimeVersionString = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.+-]+)?$`)

type observedRuntimeBinary struct {
	info    os.FileInfo
	version string
}

// Cache by executable identity so polling does not start a new version query
// on every PM2 refresh. A replaced executable invalidates the previous value.
var observedRuntimeVersions = struct {
	sync.Mutex
	paths map[string]observedRuntimeBinary
}{paths: map[string]observedRuntimeBinary{}}

func observePM2Runtime(process *PM2Process) {
	if process.PID <= 0 || (process.Status != "online" && process.Status != "launching") {
		return
	}
	path := processImagePath(process.PID)
	if path == "" {
		return
	}
	base := strings.ToLower(filepath.Base(path))
	base = strings.TrimSuffix(strings.TrimSuffix(base, " (deleted)"), ".exe")
	if base != "node" && base != "bun" {
		return
	}
	process.Runtime = base
	// PM2's node_version comes from the running process. Bun may report
	// its Node compatibility version there, so only use it for actual Node.
	if base == "node" && runtimeVersionString.MatchString(process.nodeVersion) {
		process.RuntimeVersion = strings.TrimPrefix(process.nodeVersion, "v")
		return
	}
	info, err := os.Stat(path)
	if err != nil || process.Uptime <= 0 || info.ModTime().After(time.UnixMilli(process.Uptime)) {
		return
	}
	process.RuntimeVersion = observedRuntimeVersion(base, path, info)
}

func observedRuntimeVersion(name, path string, info os.FileInfo) string {
	observedRuntimeVersions.Lock()
	defer observedRuntimeVersions.Unlock()
	if cached, ok := observedRuntimeVersions.paths[path]; ok &&
		os.SameFile(cached.info, info) && cached.info.ModTime().Equal(info.ModTime()) && cached.info.Size() == info.Size() {
		return cached.version
	}
	// Query only a runtime the host has already selected or exposed on
	// PATH. A custom process can have any filename and is never executed
	// merely because its executable is called node or bun.
	selected, _ := system.ResolveCommand(name)
	onPATH, _ := exec.LookPath(name)
	knownRuntime := ""
	for _, candidate := range []string{selected, onPATH} {
		if candidateInfo, err := os.Stat(candidate); err == nil && os.SameFile(candidateInfo, info) {
			knownRuntime = candidate
			break
		}
	}
	if knownRuntime == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, knownRuntime, "--version")
	HideWindow(command)
	output, err := command.Output()
	version := strings.TrimSpace(string(output))
	after, statErr := os.Stat(knownRuntime)
	if err != nil || statErr != nil || !os.SameFile(info, after) || !info.ModTime().Equal(after.ModTime()) ||
		!runtimeVersionString.MatchString(version) {
		return ""
	}
	version = strings.TrimPrefix(version, "v")
	observedRuntimeVersions.paths[path] = observedRuntimeBinary{info: info, version: version}
	return version
}
