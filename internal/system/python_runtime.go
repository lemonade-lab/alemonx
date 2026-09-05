package system

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// PythonRuntimeStatus separates managed Python installations from the Python
// command currently used by the service.
type PythonRuntimeStatus struct {
	Available     bool     `json:"available"`
	Versions      []string `json:"versions"`
	ActiveVersion string   `json:"activeVersion,omitempty"`
}

func PythonRuntimeStatusForHost() PythonRuntimeStatus {
	status := PythonRuntimeStatus{Versions: []string{}, ActiveVersion: pythonCommandVersion()}
	root := pythonRoot()
	if root == "" {
		return status
	}
	entries, err := os.ReadDir(filepath.Join(root, "versions"))
	if err != nil {
		return status
	}
	status.Available = true
	for _, entry := range entries {
		if entry.IsDir() && pythonVersion(entry.Name()) {
			status.Versions = append(status.Versions, entry.Name())
		}
	}
	sort.Slice(status.Versions, func(i, j int) bool { return status.Versions[i] > status.Versions[j] })
	return status
}

func pythonCommandVersion() string {
	path, err := exec.LookPath("python3")
	if err != nil {
		return ""
	}
	output, err := exec.Command(path, "--version").Output()
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(output))
	if len(fields) != 2 || fields[0] != "Python" || !pythonVersion(fields[1]) {
		return ""
	}
	return fields[1]
}

func pythonVersion(version string) bool {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, char := range part {
			if char < '0' || char > '9' {
				return false
			}
		}
	}
	return true
}

func pythonRoot() string {
	root := strings.TrimSpace(os.Getenv("PYENV_ROOT"))
	if root == "" {
		home, err := userHomeDir()
		if err != nil {
			return ""
		}
		root = filepath.Join(home, ".pyenv")
	}
	if info, err := os.Stat(filepath.Join(root, "bin", "pyenv")); err == nil && !info.IsDir() {
		return root
	}
	return ""
}

func ensurePythonRoot(ctx context.Context) (string, error) {
	if root := pythonRoot(); root != "" {
		return root, nil
	}
	home, err := userHomeDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(home, ".pyenv")
	git, err := exec.LookPath("git")
	if err != nil {
		return "", errors.New("未检测到 Git，无法准备 Python 版本管理")
	}
	output, err := exec.CommandContext(ctx, git, "clone", "--depth=1", "https://github.com/pyenv/pyenv.git", root).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("准备 Python 版本管理失败：%s", strings.TrimSpace(string(output)))
	}
	return root, nil
}

func runPyenv(ctx context.Context, root string, args ...string) error {
	command := exec.CommandContext(ctx, filepath.Join(root, "bin", "pyenv"), args...)
	command.Env = append(os.Environ(), "PYENV_ROOT="+root, "PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Python 版本操作失败：%s", strings.TrimSpace(string(output)))
	}
	return nil
}

func InstallPythonVersion(ctx context.Context, version string) (string, error) {
	if !pythonVersion(version) || strings.Count(version, ".") != 2 {
		return "", errors.New("Python 版本格式无效，请使用例如 3.12.10")
	}
	root, err := ensurePythonRoot(ctx)
	if err != nil {
		return "", err
	}
	if err := runPyenv(ctx, root, "install", "--skip-existing", version); err != nil {
		return "", err
	}
	return "已下载 Python " + version + "。", nil
}

func UsePythonVersion(ctx context.Context, version string) (string, error) {
	if !pythonVersion(version) {
		return "", errors.New("Python 版本格式无效")
	}
	root := pythonRoot()
	if root == "" {
		return "", errors.New("尚未安装可切换的 Python 版本")
	}
	if _, err := os.Stat(filepath.Join(root, "versions", version, "bin", "python3")); err != nil {
		return "", errors.New("该 Python 版本尚未下载")
	}
	if err := runPyenv(ctx, root, "global", version); err != nil {
		return "", err
	}
	if err := runPyenv(ctx, root, "rehash"); err != nil {
		return "", err
	}
	prependCommandPath(filepath.Join(root, "shims"))
	return "已切换工作台 Python 至 " + version + "。", nil
}
