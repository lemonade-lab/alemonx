package system

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"alemonx/internal/systemnetwork"
)

const bunReleaseIndex = "https://api.github.com/repos/oven-sh/bun/releases/latest"
const bunDownloadLimit = 128 << 20

var bunInstallMu sync.Mutex

func managedBunPath() string {
	cache, err := userCacheDir()
	if err != nil || !filepath.IsAbs(cache) {
		return ""
	}
	name := "bun"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(cache, "alemonx", "environments", "bun", "bin", name)
}

// BunEnvironment gives child shells access to the resolved Bun binary.
func BunEnvironment(environment []string) []string {
	bun, err := ResolveCommand("bun")
	if err != nil {
		return environment
	}
	bin := filepath.Dir(bun)
	path := environmentPath(environment)
	entries := filepath.SplitList(path)
	if len(entries) > 0 && filepath.Clean(entries[0]) == bin {
		return environment
	}
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, "PATH") {
			result = append(result, entry)
		} else if path == "" {
			_, path, _ = strings.Cut(entry, "=")
		}
	}
	if path != "" {
		path = string(os.PathListSeparator) + path
	}
	return append(result, "PATH="+bin+path)
}

func bunAssetName(platform, architecture string, musl bool) string {
	var arch string
	switch architecture {
	case "arm64":
		arch = "aarch64"
	case "amd64":
		arch = "x64"
	default:
		return ""
	}
	if platform != "linux" && platform != "darwin" && platform != "windows" {
		return ""
	}
	if platform == "linux" && musl {
		arch += "-musl"
	}
	if architecture == "amd64" {
		arch += "-baseline"
	}
	return "bun-" + platform + "-" + arch + ".zip"
}

func bunResponse(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("Bun 下载源返回 %s", response.Status)
	}
	return response, nil
}

type BunStatus struct {
	Check     Check  `json:"check"`
	Version   string `json:"version,omitempty"`
	Path      string `json:"path,omitempty"`
	Managed   bool   `json:"managed"`
	Platform  string `json:"platform"`
	Container bool   `json:"container"`
}

func CurrentBunStatus() BunStatus {
	report := NewChecker().CheckProjectRuntime("bun")
	status := BunStatus{Check: report.Checks[0], Platform: report.Platform, Container: report.Container}
	if path, err := ResolveCommand("bun"); err == nil {
		status.Path, status.Managed = path, path == managedBunPath()
		if status.Check.Status == "ready" {
			status.Version, _, _ = strings.Cut(status.Check.Detail, " · ")
		}
	}
	return status
}

func InstallManagedBun(ctx context.Context) (string, error) {
	return installManagedBun(ctx, false)
}

func UpdateManagedBun(ctx context.Context) (string, error) {
	return installManagedBun(ctx, true)
}

func installManagedBun(ctx context.Context, update bool) (string, error) {
	bunInstallMu.Lock()
	defer bunInstallMu.Unlock()

	target := managedBunPath()
	musl, _ := filepath.Glob("/lib/ld-musl-*.so.1")
	asset := bunAssetName(runtime.GOOS, runtime.GOARCH, len(musl) > 0)
	if target == "" || asset == "" {
		return "", fmt.Errorf("当前平台 %s/%s 无法托管安装 Bun，请使用官方安装说明", runtime.GOOS, runtime.GOARCH)
	}
	current, _ := ResolveCommand("bun")
	currentVersion := ""
	if current != "" {
		currentVersion, _ = checkedBunVersion(ctx, current)
		if currentVersion != "" && !update {
			return "已检测到 Bun " + currentVersion + "。", nil
		}
	}
	if update && current != target {
		return "", errors.New("这个 Bun 由外部工具安装，请使用原安装方式更新。")
	}

	client := systemnetwork.DefaultClient(4 * time.Minute)
	response, err := bunResponse(ctx, client, bunReleaseIndex)
	if err != nil {
		return "", fmt.Errorf("读取 Bun 稳定版失败：%w", err)
	}
	var release struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	err = json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&release)
	response.Body.Close()
	tag, ok := strings.CutPrefix(release.Tag, "bun-v")
	if _, stable := parseNodeVersion(tag); err != nil || !ok || !stable {
		return "", errors.New("Bun 稳定版信息无效")
	}
	if update && nodeVersionAtLeast(currentVersion, tag) {
		return "托管 Bun " + currentVersion + " 已是最新稳定版本。", nil
	}
	var checksum string
	for _, item := range release.Assets {
		if item.Name == asset {
			checksum, _ = strings.CutPrefix(item.Digest, "sha256:")
			break
		}
	}
	if raw, err := hex.DecodeString(checksum); err != nil || len(raw) != sha256.Size {
		return "", errors.New("Bun 发布版本缺少当前平台的校验值")
	}

	bin := filepath.Dir(target)
	if err := os.MkdirAll(bin, 0o700); err != nil {
		return "", err
	}
	staging, err := os.MkdirTemp(bin, ".install-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)
	url := "https://github.com/oven-sh/bun/releases/download/" + release.Tag + "/" + asset
	response, err = bunResponse(ctx, client, url)
	if err != nil {
		return "", fmt.Errorf("下载 Bun 失败：%w", err)
	}
	defer response.Body.Close()
	archive := filepath.Join(staging, asset)
	if err := writeBunFile(archive, response.Body, 0o600); err != nil {
		return "", err
	}
	if !nodeFileChecksum(archive, checksum) {
		return "", errors.New("Bun 安装包校验失败")
	}

	executable := filepath.Join(staging, filepath.Base(target))
	if err := extractBunExecutable(archive, strings.TrimSuffix(asset, ".zip")+"/"+filepath.Base(target), executable); err != nil {
		return "", err
	}
	version, err := checkedBunVersion(ctx, executable)
	if err != nil || version != tag {
		return "", errors.New("Bun 安装包无法运行或版本不匹配")
	}
	if err := os.Rename(executable, target); err != nil {
		return "", fmt.Errorf("保存 Bun 安装失败：%w", err)
	}
	if update {
		return "已更新托管 Bun 至 " + version + "。", nil
	}
	return "已安装 Bun " + version + "。", nil
}

func BunVersion(ctx context.Context) (string, error) {
	path, err := ResolveCommand("bun")
	if err != nil {
		return "", errors.New("未找到 Bun，请安装后重新检查环境")
	}
	return checkedBunVersion(ctx, path)
}

func checkedBunVersion(parent context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	HideWindow(cmd)
	output, err := cmd.Output()
	version := strings.TrimSpace(string(output))
	if _, valid := parseNodeVersion(version); err != nil || !valid {
		return "", errors.New("无法读取 Bun 版本")
	}
	return version, nil
}

func extractBunExecutable(archive, entry, target string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, item := range reader.File {
		if item.Name != entry || !item.Mode().IsRegular() || item.UncompressedSize64 > bunDownloadLimit {
			continue
		}
		input, err := item.Open()
		if err != nil {
			return err
		}
		defer input.Close()
		return writeBunFile(target, input, 0o700)
	}
	return errors.New("Bun 安装包缺少可执行文件")
}

func writeBunFile(path string, input io.Reader, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	count, copyErr := io.Copy(file, io.LimitReader(input, bunDownloadLimit+1))
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return err
	}
	if count > bunDownloadLimit {
		return errors.New("Bun 安装包超过大小限制")
	}
	return nil
}
