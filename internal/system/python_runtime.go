package system

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"alemonx/internal/systemnetwork"
)

// PythonRuntimeStatus separates managed Python installations from the Python
// command currently used by the service.
type PythonRuntimeStatus struct {
	Available     bool     `json:"available"`
	Fixed         bool     `json:"fixed,omitempty"`
	Versions      []string `json:"versions"`
	ActiveVersion string   `json:"activeVersion,omitempty"`
}

func PythonRuntimeStatusForHost() PythonRuntimeStatus {
	status := PythonRuntimeStatus{Versions: []string{}, ActiveVersion: pythonCommandVersion()}
	if InContainer() {
		status.Fixed = true
		return status
	}
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
	if err := downloadPyenv(ctx, root); err != nil {
		return "", err
	}
	return root, nil
}

const pyenvArchiveURL = "https://github.com/pyenv/pyenv/archive/refs/heads/master.tar.gz"

// downloadPyenv keeps the bootstrap archive inside AlemonX's controlled
// networking. In particular it observes the GitHub mirror selected in system
// settings instead of relying on the terminal's Git configuration.
func downloadPyenv(ctx context.Context, root string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, pyenvArchiveURL, nil)
	if err != nil {
		return err
	}
	response, err := systemnetwork.DefaultClient(2 * time.Minute).Do(request)
	if err != nil {
		return fmt.Errorf("下载 Python 版本管理组件失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("下载 Python 版本管理组件失败：服务器返回 %s", response.Status)
	}
	staging, err := os.MkdirTemp(filepath.Dir(root), ".pyenv-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := extractPyenvArchive(response.Body, staging); err != nil {
		return fmt.Errorf("准备 Python 版本管理失败：%w", err)
	}
	if _, err := os.Stat(filepath.Join(staging, "bin", "pyenv")); err != nil {
		return errors.New("准备 Python 版本管理失败：下载内容不完整")
	}
	if err := os.Rename(staging, root); err != nil {
		return fmt.Errorf("准备 Python 版本管理失败：%w", err)
	}
	return nil
}

func extractPyenvArchive(source io.Reader, destination string) error {
	gzipReader, err := gzip.NewReader(source)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	archive := tar.NewReader(gzipReader)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.Clean(header.Name), "/")
		if len(parts) < 2 || parts[0] == "." || parts[0] == ".." {
			continue
		}
		relative := filepath.Join(parts[1:]...)
		if relative == "." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return errors.New("下载内容包含无效路径")
		}
		target := filepath.Join(destination, relative)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, archive)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
}

func runPyenv(ctx context.Context, root string, args ...string) error {
	command := exec.CommandContext(ctx, filepath.Join(root, "bin", "pyenv"), args...)
	environment := append(os.Environ(), "PYENV_ROOT="+root, "PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	environment = append(environment, systemnetwork.PythonBuildEnvironment()...)
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Python 版本操作失败：%s", strings.TrimSpace(string(output)))
	}
	return nil
}

func InstallPythonVersion(ctx context.Context, version string) (string, error) {
	if InContainer() {
		return "", errors.New("Docker 环境中的 Python 由镜像固定，不能在容器内安装版本。请更新镜像后重新创建容器。")
	}
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
	if InContainer() {
		return "", errors.New("Docker 环境中的 Python 由镜像固定，不能在容器内切换版本。请更新镜像后重新创建容器。")
	}
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
