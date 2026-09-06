package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"alemonx/internal/resources"
)

// NodeRuntime is the single current Node.js execution context for ALemonX.
// Its version is always measured by running its exact Node binary.
type NodeRuntime struct {
	Path        string
	Bin         string
	Version     string
	Environment []string
}

func (runtime NodeRuntime) CommandPath(name string) (string, error) {
	if name == "node" {
		return runtime.Path, nil
	}
	if name == "npm" || name == "npx" || name == "corepack" {
		path := filepath.Join(runtime.Bin, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return path, nil
		}
		return "", exec.ErrNotFound
	}
	return exec.LookPath(name)
}

func CurrentNodeRuntime() (NodeRuntime, error) {
	path, err := nodeLookPath("node")
	if err != nil {
		return NodeRuntime{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil || ctx.Err() != nil {
		return NodeRuntime{}, fmt.Errorf("Node.js 无法执行：%w", err)
	}
	version := normalizeNodeVersion(strings.TrimSpace(string(output)))
	if version == "" {
		return NodeRuntime{}, fmt.Errorf("Node.js 版本输出无效：%s", strings.TrimSpace(string(output)))
	}
	bin := filepath.Dir(path)
	return NodeRuntime{Path: path, Bin: bin, Version: "v" + version, Environment: nodeRuntimeEnvironment(bin, os.Environ())}, nil
}

// ApplyNodeRuntime is the only function permitted to change ALemonX's Node
// PATH. It rolls back when the requested runtime cannot become node --version.
func ApplyNodeRuntime(bin string) (NodeRuntime, error) {
	previous := os.Getenv("PATH")
	_ = os.Setenv("PATH", nodeRuntimePath(bin, previous))
	runtime, err := CurrentNodeRuntime()
	if err != nil || filepath.Clean(runtime.Bin) != filepath.Clean(bin) {
		_ = os.Setenv("PATH", previous)
		if err != nil {
			return NodeRuntime{}, err
		}
		return NodeRuntime{}, fmt.Errorf("Node.js 切换验证失败：当前仍使用 %s", runtime.Path)
	}
	resources.SetNodeRuntime(runtime.Path, runtime.Environment)
	return runtime, nil
}

// ConfigureNodeRuntime makes embedded JavaScript tools follow the existing
// process runtime without changing PATH. It is used at service startup.
func ConfigureNodeRuntime() (NodeRuntime, error) {
	runtime, err := CurrentNodeRuntime()
	if err != nil {
		return NodeRuntime{}, err
	}
	resources.SetNodeRuntime(runtime.Path, runtime.Environment)
	return runtime, nil
}

func nodeRuntimeEnvironment(bin string, environment []string) []string {
	result := make([]string, 0, len(environment)+1)
	for _, item := range environment {
		if !strings.HasPrefix(item, "PATH=") {
			result = append(result, item)
		}
	}
	return append(result, "PATH="+nodeRuntimePath(bin, environmentPath(environment)))
}

func environmentPath(environment []string) string {
	for _, item := range environment {
		if strings.HasPrefix(item, "PATH=") {
			return strings.TrimPrefix(item, "PATH=")
		}
	}
	return ""
}

func nodeRuntimePath(bin, path string) string {
	entries := filepath.SplitList(path)
	filtered := make([]string, 0, len(entries)+1)
	for _, entry := range entries {
		if entry != "" && filepath.Clean(entry) != filepath.Clean(bin) && !isNodeRuntimeBin(entry) {
			filtered = append(filtered, entry)
		}
	}
	return strings.Join(append([]string{bin}, filtered...), string(os.PathListSeparator))
}
