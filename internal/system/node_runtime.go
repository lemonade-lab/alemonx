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
	"strconv"
	"strings"
	"time"

	"alemonx/internal/resources"
	"alemonx/internal/systemnetwork"
)

const (
	nodeIndexURL        = "https://nodejs.org/dist/index.json"
	nodeDownloadLimit   = 180 << 20
	nodeDownloadTimeout = 12 * time.Minute
	nvmVersion          = "v0.40.7"
)

var (
	userCacheDir = os.UserCacheDir
	userHomeDir  = os.UserHomeDir
	nodeLookPath = exec.LookPath
)

type nodeRelease struct {
	Version string          `json:"version"`
	LTS     json.RawMessage `json:"lts"`
}

// NVMNodeStatus describes Node.js versions managed by the user's NVM when it
// is available, otherwise by ALemonX's isolated fallback installation.
type NVMNodeStatus struct {
	Available            bool     `json:"available"`
	Versions             []string `json:"versions"`
	ActiveVersion        string   `json:"activeVersion,omitempty"`
	RecommendedVersion   string   `json:"recommendedVersion"`
	RecommendedInstalled bool     `json:"recommendedInstalled"`
	LatestVersion        string   `json:"latestVersion,omitempty"`
	LatestInstalled      bool     `json:"latestInstalled"`
}

// NVMStatus returns locally managed versions and the active Node.js runtime.
// ActiveVersion is populated exclusively from a real `node --version` call;
// managed defaults and downloaded versions are never treated as current use.
func NVMStatus() NVMNodeStatus {
	status := NVMNodeStatus{Versions: []string{}, RecommendedVersion: "v" + MinimumNodeVersion}
	directory, err := preferredNVMDirectory()
	if err == nil {
		if entries, readErr := os.ReadDir(filepath.Join(directory, "versions", "node")); readErr == nil {
			status.Available = true
			for _, entry := range entries {
				if entry.IsDir() && isNodeVersion(entry.Name()) {
					bin := filepath.Join(directory, "versions", "node", entry.Name(), "bin", "node")
					if info, statErr := os.Stat(bin); statErr == nil && !info.IsDir() {
						status.Versions = append(status.Versions, entry.Name())
					}
				}
			}
			sortNodeVersions(status.Versions)
		}
	}
	status.RecommendedInstalled = containsNodeVersion(status.Versions, MinimumNodeVersion)
	status.ActiveVersion = systemNodeVersion()
	return status
}

func systemNodeVersion() string {
	path, err := nodeLookPath("node")
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil || ctx.Err() != nil {
		return ""
	}
	version := normalizeNodeVersion(strings.TrimSpace(string(output)))
	if version == "" {
		return ""
	}
	return "v" + version
}

// InstallNVMNodeVersion installs an explicit semver through NVM and makes it
// the workbench default. The input is intentionally limited to a concrete
// Node version so it cannot be interpreted as an NVM option.
func InstallNVMNodeVersion(ctx context.Context, version string) (string, error) {
	version = normalizeNodeVersion(version)
	if version == "" {
		return "", errors.New("Node.js 版本格式无效，请使用例如 22.22.3")
	}
	directory, _, err := preferredOrEnsureNVM()
	if err != nil {
		return "", err
	}
	if err := runNVM(ctx, directory, "install", version); err != nil {
		return "", err
	}
	return "已下载 Node.js v" + version + "。", nil
}

// UseNVMNodeVersion changes the NVM default and updates this process PATH so
// subsequent workbench commands use the selected runtime immediately.
func UseNVMNodeVersion(ctx context.Context, version string) (string, error) {
	version = normalizeNodeVersion(version)
	if version == "" {
		return "", errors.New("Node.js 版本格式无效")
	}
	directory, err := preferredNVMDirectory()
	if err != nil {
		return "", err
	}
	if !containsNodeVersion(NVMStatus().Versions, version) {
		return "", errors.New("该 Node.js 版本尚未通过 NVM 安装")
	}
	return useNVMNodeVersion(ctx, directory, version)
}

func useNVMNodeVersion(ctx context.Context, directory, version string) (string, error) {
	if err := runNVM(ctx, directory, "alias", "default", version); err != nil {
		return "", err
	}
	if err := runNVM(ctx, directory, "use", version); err != nil {
		return "", err
	}
	bin := filepath.Join(directory, "versions", "node", "v"+version, "bin")
	if info, err := os.Stat(filepath.Join(bin, "node")); err != nil || info.IsDir() {
		return "", errors.New("NVM 未返回可用的 Node.js 运行时")
	}
	prependCommandPath(bin)
	RefreshCommandEnvironment("node", "npm", "npx")
	return "已切换工作台 Node.js 至 v" + version + "。", nil
}

func runNVM(ctx context.Context, directory string, args ...string) error {
	shell, err := exec.LookPath("bash")
	if err != nil {
		return errors.New("未检测到 Bash，无法运行 NVM")
	}
	command := "nvm"
	for _, arg := range args {
		command += " '" + arg + "'"
	}
	script := "set -eu\nexport NVM_DIR=\"$1\"\n. \"$NVM_DIR/nvm.sh\"\n" + command
	output, runErr := exec.CommandContext(ctx, shell, "-c", script, "alemonx-nvm", directory).CombinedOutput()
	if runErr != nil {
		return fmt.Errorf("NVM 执行 %s 失败：%s", strings.Join(args, " "), limitedNodeOutput(output, runErr))
	}
	return nil
}

func normalizeNodeVersion(version string) string {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return ""
	}
	for _, part := range parts {
		if part == "" {
			return ""
		}
		for _, char := range part {
			if char < '0' || char > '9' {
				return ""
			}
		}
	}
	return version
}

func isNodeVersion(version string) bool { return normalizeNodeVersion(version) != "" }

func containsNodeVersion(versions []string, target string) bool {
	for _, version := range versions {
		if version == "v"+target {
			return true
		}
	}
	return false
}

func sortNodeVersions(versions []string) {
	sort.Slice(versions, func(i, j int) bool {
		left, leftOK := parseNodeVersion(versions[i])
		right, rightOK := parseNodeVersion(versions[j])
		if leftOK && rightOK {
			for index := range left {
				if left[index] != right[index] {
					return left[index] > right[index]
				}
			}
		}
		return versions[i] > versions[j]
	})
}

// InstallNodeWithNVM keeps Node.js upgrades scoped to the current user. On
// POSIX hosts it materializes the embedded official NVM shell implementation
// into ALemonX's cache (without editing shell profiles), then uses it to
// install and select the current Node.js 22 LTS release.
// Windows uses the separate nvm-windows implementation when it is available.
func InstallNodeWithNVM(ctx context.Context) (string, error) {
	if runtime.GOOS == "windows" {
		return installNodeWithNVMWindows(ctx)
	}
	directory, installedNVM, err := ensureNVM()
	if err != nil {
		return "", err
	}
	bin, version, err := installNVMNodeLTS(ctx, directory)
	if err != nil {
		return "", err
	}
	prependCommandPath(bin)
	RefreshCommandEnvironment("node", "npm", "npx")
	prefix := "已通过 NVM 安装并启用 Node.js 22 LTS " + version + "。"
	if installedNVM {
		prefix = "已安装 NVM，并通过 NVM 安装并启用 Node.js 22 LTS " + version + "。"
	}
	return prefix + " 请重新检查环境确认版本。", nil
}

// nvmDirectory is the workbench-owned fallback location. It intentionally
// remains separate from a user's existing NVM installation.
func nvmDirectory() (string, error) {
	base, err := nodeRuntimeBase()
	if err != nil {
		return "", fmt.Errorf("无法确定 NVM 安装目录：%w", err)
	}
	return filepath.Join(filepath.Dir(base), "nvm", nvmVersion), nil
}

func preferredNVMDirectory() (string, error) {
	if directory := localNVMDirectory(); directory != "" {
		return directory, nil
	}
	return nvmDirectory()
}

func localNVMDirectory() string {
	candidates := []string{}
	if directory := strings.TrimSpace(os.Getenv("NVM_DIR")); directory != "" && filepath.IsAbs(directory) {
		candidates = append(candidates, directory)
	}
	if home, err := userHomeDir(); err == nil && home != "" {
		candidates = append(candidates, filepath.Join(home, ".nvm"))
	}
	for _, directory := range candidates {
		if info, err := os.Stat(filepath.Join(directory, "nvm.sh")); err == nil && !info.IsDir() {
			return directory
		}
	}
	return ""
}

func preferredOrEnsureNVM() (string, bool, error) {
	if directory := localNVMDirectory(); directory != "" {
		return directory, false, nil
	}
	return ensureNVM()
}

func nvmDefaultVersion(directory string, versions []string) string {
	data, err := os.ReadFile(filepath.Join(directory, "alias", "default"))
	if err != nil {
		return ""
	}
	candidate := strings.TrimSpace(string(data))
	if normalized := normalizeNodeVersion(candidate); normalized != "" {
		candidate = "v" + normalized
	}
	for _, version := range versions {
		if candidate == version {
			return version
		}
	}
	if major, err := strconv.Atoi(strings.TrimPrefix(candidate, "v")); err == nil && major >= 0 {
		for _, version := range versions {
			parsed, ok := parseNodeVersion(version)
			if ok && parsed[0] == major {
				return version
			}
		}
	}
	return ""
}

func ensureNVM() (string, bool, error) {
	directory, err := nvmDirectory()
	if err != nil {
		return "", false, err
	}
	installed, err := resources.MaterializeNVM(directory)
	if err != nil {
		return "", false, err
	}
	return directory, installed, nil
}

const nvmInstallNode22Script = `set -eu
export NVM_DIR="$1"
. "$NVM_DIR/nvm.sh"
nvm install 22
nvm alias default 22 >/dev/null
nvm use default >/dev/null
printf '__ALX_NODE_BIN__=%s\n' "$(dirname "$(command -v node)")"
printf '__ALX_NODE_VERSION__=%s\n' "$(node --version)"`

func installNVMNodeLTS(ctx context.Context, directory string) (string, string, error) {
	shell, err := exec.LookPath("bash")
	if err != nil {
		return "", "", errors.New("未检测到 Bash，无法运行 NVM")
	}
	output, runErr := exec.CommandContext(ctx, shell, "-c", nvmInstallNode22Script, "alemonx-nvm", directory).CombinedOutput()
	if runErr != nil {
		return "", "", fmt.Errorf("NVM 安装 Node.js 22 LTS 失败：%s", limitedNodeOutput(output, runErr))
	}
	bin, version := "", ""
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if value, ok := strings.CutPrefix(line, "__ALX_NODE_BIN__="); ok {
			bin = value
		}
		if value, ok := strings.CutPrefix(line, "__ALX_NODE_VERSION__="); ok {
			version = value
		}
	}
	if bin == "" || version == "" || !filepath.IsAbs(bin) {
		return "", "", errors.New("NVM 未返回有效的 Node.js 安装路径")
	}
	if relative, relErr := filepath.Rel(directory, bin); relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", errors.New("NVM 返回的 Node.js 安装路径无效")
	}
	if info, statErr := os.Stat(filepath.Join(bin, "node")); statErr != nil || info.IsDir() {
		return "", "", errors.New("NVM 未安装可用的 Node.js")
	}
	return bin, version, nil
}

func installNodeWithNVMWindows(ctx context.Context) (string, error) {
	nvm, err := ResolveCommand("nvm")
	if err != nil {
		manager, managerErr := hostPackageManager()
		if managerErr != nil {
			return "", errors.New("未检测到 NVM for Windows，且无法找到可用的包管理器进行安装")
		}
		packages := []string(nil)
		switch manager {
		case "winget":
			packages = []string{"CoreyButler.NVMforWindows"}
		case "choco":
			packages = []string{"nvm"}
		default:
			return "", fmt.Errorf("未检测到 NVM for Windows；当前包管理器 %s 不支持自动安装", manager)
		}
		output, runErr := runPackageCommand(ctx, manager, installArguments(manager, packages))
		if runErr != nil {
			return "", fmt.Errorf("安装 NVM for Windows 失败：%s", limitedNodeOutput([]byte(output), runErr))
		}
		RefreshCommandEnvironment("nvm")
		nvm, err = ResolveCommand("nvm")
		if err != nil {
			return "", errors.New("NVM for Windows 已安装，但当前服务未找到 nvm；请重新启动工作台后重试")
		}
	}
	version, err := node22LTS(ctx)
	if err != nil {
		return "", err
	}
	for _, args := range [][]string{{"install", version}, {"use", version}} {
		output, runErr := exec.CommandContext(ctx, nvm, args...).CombinedOutput()
		if runErr != nil {
			return "", fmt.Errorf("NVM 执行 %s 失败：%s", strings.Join(args, " "), limitedNodeOutput(output, runErr))
		}
	}
	RefreshCommandEnvironment("node", "npm", "npx")
	return "已通过 NVM for Windows 安装并启用 Node.js 22 LTS " + version + "。请重新检查环境确认版本。", nil
}

// InstallManagedNode downloads a verified Node.js LTS package through the
// workbench network manager. This is intentionally independent from npm,
// Homebrew, WinGet and the host package manager's own mirror configuration.
func InstallManagedNode(ctx context.Context) (string, error) {
	version, err := node22LTS(ctx)
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

// node22LTS returns the current stable release from the supported Node.js 22
// line. This is intentionally distinct from latestNodeLTS: newer LTS majors
// are shown as informational updates, but never become the default runtime.
func node22LTS(ctx context.Context) (string, error) {
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
		if strings.HasPrefix(release.Version, "v22.") && len(release.LTS) > 0 && string(release.LTS) != "false" && string(release.LTS) != "null" {
			return release.Version, nil
		}
	}
	return "", errors.New("Node.js 下载源未提供可用的 22 LTS 版本")
}

// LatestNodeLTS returns the newest LTS release advertised by the configured
// Node.js index. It is exported for read-only UI recommendations.
func LatestNodeLTS(ctx context.Context) (string, error) {
	return latestNodeLTS(ctx)
}

func nodeAssetName(version string) (string, error) {
	arch := nodeArchitecture(runtime.GOARCH)
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

func nodeArchitecture(architecture string) string {
	return map[string]string{
		"amd64":   "x64",
		"arm64":   "arm64",
		"386":     "x86",
		"arm":     "armv7l",
		"ppc64le": "ppc64le",
		"s390x":   "s390x",
		"riscv64": "riscv64",
	}[architecture]
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
	name := "node-" + version + "-" + runtime.GOOS + "-" + nodeArchitecture(runtime.GOARCH)
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

// NVMNodeBin returns the newest Node runtime installed by nvm. nvm itself is
// shell-scoped, so resolving the selected binary directory lets the desktop
// service and its child processes use it without loading a shell profile.
func NVMNodeBin() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	directory, err := preferredNVMDirectory()
	if err != nil {
		return ""
	}
	versions := make([]string, 0)
	entries, err := os.ReadDir(filepath.Join(directory, "versions", "node"))
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() && isNodeVersion(entry.Name()) {
			versions = append(versions, entry.Name())
		}
	}
	// This default controls which NVM binary workbench operations select. It
	// is deliberately separate from NVMStatus.ActiveVersion, which reports
	// only the result of the real `node --version` command.
	if active := nvmDefaultVersion(directory, versions); active != "" {
		bin := filepath.Join(directory, "versions", "node", active, "bin")
		if info, statErr := os.Stat(filepath.Join(bin, "node")); statErr == nil && !info.IsDir() {
			return bin
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		left, leftOK := parseNodeVersion(entries[i].Name())
		right, rightOK := parseNodeVersion(entries[j].Name())
		if leftOK && rightOK {
			for index := range left {
				if left[index] != right[index] {
					return left[index] > right[index]
				}
			}
		}
		return entries[i].Name() > entries[j].Name()
	})
	for _, entry := range entries {
		bin := filepath.Join(directory, "versions", "node", entry.Name(), "bin")
		if info, err := os.Stat(filepath.Join(bin, "node")); err == nil && !info.IsDir() {
			return bin
		}
	}
	return ""
}

func nvmNodeCommand(name string) string {
	if name != "node" && name != "npm" && name != "npx" {
		return ""
	}
	bin := NVMNodeBin()
	if bin == "" {
		return ""
	}
	path := filepath.Join(bin, name)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path
	}
	return ""
}

func prependCommandPath(directory string) {
	if directory == "" {
		return
	}
	entries := filepath.SplitList(os.Getenv("PATH"))
	merged := make([]string, 0, len(entries)+1)
	seen := map[string]bool{}
	for _, entry := range append([]string{directory}, entries...) {
		if entry == "" {
			continue
		}
		// A previous Node runtime directory must not keep winning after the
		// user switches versions. Preserve unrelated PATH entries unchanged.
		if entry != directory && isNodeRuntimeBin(entry) {
			continue
		}
		key := filepath.Clean(entry)
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if !seen[key] {
			seen[key] = true
			merged = append(merged, entry)
		}
	}
	_ = os.Setenv("PATH", strings.Join(merged, string(os.PathListSeparator)))
}

func isNodeRuntimeBin(directory string) bool {
	clean := filepath.ToSlash(filepath.Clean(directory))
	return strings.Contains(clean, "/versions/node/v") && strings.HasSuffix(clean, "/bin")
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
