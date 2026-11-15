package system

import (
	"archive/zip"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"alemonx/internal/systemnetwork"
)

const gitForWindowsRelease = "https://api.github.com/repos/git-for-windows/git/releases/latest"
const gitArchiveLimit = 160 << 20
const gitExtractLimit = 512 << 20

var gitForWindowsTag = regexp.MustCompile(`^v([0-9]+\.[0-9]+\.[0-9]+)\.windows\.[0-9]+$`)
var managedGitInstallMu sync.Mutex

func managedGitDirectory() string {
	cache, err := userCacheDir()
	if err != nil || !filepath.IsAbs(cache) {
		return ""
	}
	return filepath.Join(cache, "alemonx", "environments", "git", "installed")
}

func gitForWindowsAsset(tag, arch string) string {
	match := gitForWindowsTag.FindStringSubmatch(tag)
	if len(match) != 2 {
		return ""
	}
	suffix := map[string]string{"amd64": "64-bit", "arm64": "arm64", "386": "32-bit"}[arch]
	if suffix == "" {
		return ""
	}
	return "MinGit-" + match[1] + "-" + suffix + ".zip"
}

func gitForWindowsDownload(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
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
		return nil, fmt.Errorf("Git for Windows 下载源返回 %s", response.Status)
	}
	return response, nil
}

func installManagedWindowsGit(ctx context.Context) (string, error) {
	managedGitInstallMu.Lock()
	defer managedGitInstallMu.Unlock()

	if existing, err := ResolveCommand("git"); err == nil {
		if version, err := managedGitVersion(ctx, existing); err == nil {
			RefreshCommandEnvironment("git")
			return "已检测到可用的 " + version + "。", nil
		}
	}
	target := managedGitDirectory()
	if target == "" {
		return "", errors.New("无法定位当前 Windows 用户的工作台缓存目录")
	}
	client := systemnetwork.DefaultClient(4 * time.Minute)
	response, err := gitForWindowsDownload(ctx, client, gitForWindowsRelease)
	if err != nil {
		return "", fmt.Errorf("读取 Git for Windows 发布信息失败：%w", err)
	}
	var release struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	err = json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&release)
	response.Body.Close()
	asset := gitForWindowsAsset(release.Tag, runtime.GOARCH)
	if err != nil || asset == "" {
		return "", errors.New("无法识别当前 Windows 架构的 MinGit 稳定版")
	}
	var checksum string
	for _, item := range release.Assets {
		if item.Name == asset {
			checksum, _ = strings.CutPrefix(item.Digest, "sha256:")
			break
		}
	}
	if raw, err := hex.DecodeString(checksum); err != nil || len(raw) != 32 {
		return "", errors.New("Git 官方发布缺少当前架构安装包的 SHA-256 校验值")
	}

	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", err
	}
	staging, err := os.MkdirTemp(parent, ".install-")
	if err != nil {
		return "", err
	}
	keepBackup := false
	defer func() {
		if !keepBackup {
			_ = os.RemoveAll(staging)
		}
	}()
	assetURL := "https://github.com/git-for-windows/git/releases/download/" + release.Tag + "/" + asset
	response, err = gitForWindowsDownload(ctx, client, assetURL)
	if err != nil {
		return "", fmt.Errorf("下载 Git for Windows 失败：%w", err)
	}
	archive := filepath.Join(staging, asset)
	file, err := os.OpenFile(archive, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		response.Body.Close()
		return "", err
	}
	n, copyErr := io.Copy(file, io.LimitReader(response.Body, gitArchiveLimit+1))
	closeErr := file.Close()
	response.Body.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return "", err
	}
	if n > gitArchiveLimit || !nodeFileChecksum(archive, checksum) {
		return "", errors.New("Git 官方安装包超出限制或 SHA-256 校验失败")
	}

	unpacked := filepath.Join(staging, "mingit")
	if err := extractMinGit(ctx, archive, unpacked); err != nil {
		return "", fmt.Errorf("解压 Git for Windows 失败：%w", err)
	}
	executable := filepath.Join(unpacked, "cmd", "git.exe")
	version, err := managedGitVersion(ctx, executable)
	if err != nil {
		return "", fmt.Errorf("Git 安装包解压成功，但 git --version 无法运行：%w", err)
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	probe := exec.CommandContext(probeCtx, executable, "init", "--quiet", "--template=", filepath.Join(staging, "test-repository"))
	probe.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	HideWindow(probe)
	if output, err := probe.CombinedOutput(); err != nil {
		return "", fmt.Errorf("Git 无法初始化测试仓库：%s", limitedNodeOutput(output, err))
	}
	previous := filepath.Join(staging, "previous")
	hadPrevious := false
	if _, err := os.Lstat(target); err == nil {
		if err := os.Rename(target, previous); err != nil {
			return "", fmt.Errorf("无法替换旧的托管 Git：%w", err)
		}
		hadPrevious = true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := os.Rename(unpacked, target); err != nil {
		if hadPrevious {
			if restoreErr := os.Rename(previous, target); restoreErr != nil {
				keepBackup = true
				return "", fmt.Errorf("保存 Git 失败，旧版保留在 %s：%w", previous, errors.Join(err, restoreErr))
			}
		}
		return "", fmt.Errorf("保存 Git 失败：%w", err)
	}
	RefreshCommandEnvironment("git")
	return "已安装工作台托管的 " + version + "，并通过仓库初始化检查。", nil
}

func managedGitVersion(parent context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, path, "--version")
	HideWindow(command)
	out, err := command.Output()
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(out))
	if !strings.HasPrefix(version, "git version ") {
		return "", errors.New("Git 返回了无法识别的版本")
	}
	return version, nil
}

func extractMinGit(ctx context.Context, archive, directory string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	var total uint64
	for _, entry := range reader.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := strings.TrimSuffix(entry.Name, "/")
		if name == "." || !fs.ValidPath(name) || strings.ContainsAny(name, "\\:") || !filepath.IsLocal(filepath.FromSlash(name)) {
			return errors.New("Git 安装包包含无效文件路径")
		}
		for _, part := range strings.Split(name, "/") {
			if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
				return errors.New("Git 安装包包含不兼容的 Windows 文件名")
			}
		}
		path := filepath.Join(directory, filepath.FromSlash(name))
		if entry.Mode().IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			continue
		}
		if !entry.Mode().IsRegular() || entry.UncompressedSize64 > gitExtractLimit-total {
			return errors.New("Git 安装包包含不支持的文件或解压体积超限")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		input, err := entry.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o755)
		if err != nil {
			input.Close()
			return err
		}
		n, err := io.Copy(output, io.LimitReader(input, int64(entry.UncompressedSize64)+1))
		if err := errors.Join(err, output.Close(), input.Close()); err != nil {
			return err
		}
		if uint64(n) != entry.UncompressedSize64 {
			return errors.New("Git 安装包的文件长度不匹配")
		}
		total += uint64(n)
	}
	return nil
}
