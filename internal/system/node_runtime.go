package system

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"alemonx/internal/systemnetwork"
)

const (
	nodeIndexURL        = "https://nodejs.org/dist/index.json"
	nodeDownloadLimit   = 180 << 20
	nodeDownloadTimeout = 12 * time.Minute
)

var userCacheDir = os.UserCacheDir

type nodeRelease struct {
	Version string          `json:"version"`
	LTS     json.RawMessage `json:"lts"`
}

// InstallManagedNode downloads a verified Node.js LTS package through the
// workbench network manager. This is intentionally independent from npm,
// Homebrew, WinGet and the host package manager's own mirror configuration.
func InstallManagedNode(ctx context.Context) (string, error) {
	version, err := latestNodeLTS(ctx)
	if err != nil {
		return "", err
	}
	asset, err := nodeAssetName(version)
	if err != nil {
		return "", err
	}
	base := "https://nodejs.org/dist/" + version + "/"
	checksum, err := nodeChecksum(ctx, base+"SHASUMS256.txt", asset)
	if err != nil {
		return "", err
	}
	archive, err := cachedNodeArchive(ctx, base+asset, asset, checksum)
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		output, runErr := exec.CommandContext(ctx, "msiexec.exe", "/i", archive, "/qn", "/norestart").CombinedOutput()
		if runErr != nil {
			return "", fmt.Errorf("Node.js 安装程序执行失败：%s", limitedNodeOutput(output, runErr))
		}
		RefreshCommandEnvironment("node", "npm", "npx")
		return "已通过工作台镜像下载并安装 Node.js LTS。请重新检查环境确认版本。", nil
	}
	bin, err := installNodeArchive(ctx, archive, version)
	if err != nil {
		return "", err
	}
	RefreshCommandEnvironment("node", "npm", "npx")
	return "已通过工作台镜像下载并安装 Node.js LTS（" + bin + "）。请重新检查环境确认版本。", nil
}

func latestNodeLTS(ctx context.Context) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, nodeIndexURL, nil)
	if err != nil {
		return "", err
	}
	response, err := systemnetwork.DefaultClient(20 * time.Second).Do(request)
	if err != nil {
		return "", fmt.Errorf("读取 Node.js 版本索引失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("读取 Node.js 版本索引失败：%s", response.Status)
	}
	var releases []nodeRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&releases); err != nil {
		return "", fmt.Errorf("Node.js 版本索引格式无效：%w", err)
	}
	for _, release := range releases {
		if strings.HasPrefix(release.Version, "v") && len(release.LTS) > 0 && string(release.LTS) != "false" && string(release.LTS) != "null" {
			return release.Version, nil
		}
	}
	return "", errors.New("Node.js 下载源未提供可用的 LTS 版本")
}

func nodeAssetName(version string) (string, error) {
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[runtime.GOARCH]
	if arch == "" {
		return "", fmt.Errorf("Node.js 托管安装暂不支持 %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	prefix := "node-" + version + "-"
	switch runtime.GOOS {
	case "linux":
		return prefix + "linux-" + arch + ".tar.xz", nil
	case "darwin":
		return prefix + "darwin-" + arch + ".tar.gz", nil
	case "windows":
		return prefix + arch + ".msi", nil
	default:
		return "", fmt.Errorf("Node.js 托管安装暂不支持 %s", runtime.GOOS)
	}
}

func nodeChecksum(ctx context.Context, raw, asset string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return "", err
	}
	response, err := systemnetwork.DefaultClient(20 * time.Second).Do(request)
	if err != nil {
		return "", fmt.Errorf("读取 Node.js 校验文件失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("读取 Node.js 校验文件失败：%s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimPrefix(fields[len(fields)-1], "*") == asset && len(fields[0]) == 64 {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", errors.New("Node.js 校验文件中缺少当前平台安装包")
}

func cachedNodeArchive(ctx context.Context, raw, asset, checksum string) (string, error) {
	base, err := nodeRuntimeBase()
	if err != nil {
		return "", err
	}
	directory := filepath.Join(base, "archives")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(directory, filepath.Base(asset))
	if nodeFileChecksum(path, checksum) {
		return path, nil
	}
	response, err := systemnetwork.DefaultClient(nodeDownloadTimeout).Get(raw)
	if err != nil {
		return "", fmt.Errorf("下载 Node.js 安装包失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载 Node.js 安装包失败：%s", response.Status)
	}
	partial := path + ".part"
	file, err := os.OpenFile(partial, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(file, io.LimitReader(response.Body, nodeDownloadLimit+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(partial)
		if copyErr != nil {
			return "", copyErr
		}
		return "", closeErr
	}
	info, statErr := os.Stat(partial)
	if statErr != nil || info.Size() > nodeDownloadLimit || !nodeFileChecksum(partial, checksum) {
		_ = os.Remove(partial)
		return "", errors.New("Node.js 安装包校验失败")
	}
	if err := os.Rename(partial, path); err != nil {
		return "", err
	}
	return path, nil
}

func installNodeArchive(ctx context.Context, archive, version string) (string, error) {
	base, err := nodeRuntimeBase()
	if err != nil {
		return "", err
	}
	name := "node-" + version + "-" + runtime.GOOS + "-" + map[string]string{"amd64": "x64", "arm64": "arm64"}[runtime.GOARCH]
	target := filepath.Join(base, "installed", name)
	bin := filepath.Join(target, "bin")
	if info, err := os.Stat(filepath.Join(bin, "node")); err == nil && !info.IsDir() {
		return bin, nil
	}
	if err := os.MkdirAll(filepath.Join(base, "installed"), 0o700); err != nil {
		return "", err
	}
	staging, err := os.MkdirTemp(filepath.Join(base, "installed"), ".extract-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)
	output, runErr := exec.CommandContext(ctx, "tar", "-xf", archive, "-C", staging).CombinedOutput()
	if runErr != nil {
		return "", fmt.Errorf("解压 Node.js 安装包失败：%s", limitedNodeOutput(output, runErr))
	}
	entries, err := os.ReadDir(staging)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		return "", errors.New("Node.js 安装包结构无效")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", err
	}
	_ = os.RemoveAll(target)
	if err := os.Rename(filepath.Join(staging, entries[0].Name()), target); err != nil {
		return "", err
	}
	return bin, nil
}

// ManagedNodeBin returns the newest verified Node runtime installed by the
// workbench so checks and child project commands can use it after a restart.
func ManagedNodeBin() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	base, err := nodeRuntimeBase()
	if err != nil {
		return ""
	}
	entries, err := os.ReadDir(filepath.Join(base, "installed"))
	if err != nil {
		return ""
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() > entries[j].Name() })
	for _, entry := range entries {
		bin := filepath.Join(base, "installed", entry.Name(), "bin")
		if info, err := os.Stat(filepath.Join(bin, "node")); err == nil && !info.IsDir() {
			return bin
		}
	}
	return ""
}

// ManagedNodeCommand resolves the executable for a workbench-installed Node
// runtime. Callers may fall back to exec.LookPath when it returns an empty
// string, preserving a user's system or version-manager installation.
func ManagedNodeCommand(name string) string {
	if name != "node" && name != "npm" && name != "npx" {
		return ""
	}
	bin := ManagedNodeBin()
	if bin == "" {
		return ""
	}
	path := filepath.Join(bin, name)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path
	}
	return ""
}

func nodeRuntimeBase() (string, error) {
	base, err := userCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "alemonx", "environments", "node"), nil
}

func nodeFileChecksum(path, wanted string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return false
	}
	return strings.EqualFold(fmt.Sprintf("%x", digest.Sum(nil)), wanted)
}

func limitedNodeOutput(output []byte, runErr error) string {
	text := strings.TrimSpace(string(output))
	if text == "" {
		text = runErr.Error()
	}
	if len(text) > 500 {
		text = text[:500] + "…"
	}
	return text
}
