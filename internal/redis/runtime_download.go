package redis

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	runtimeDownloadLimit = 200 << 20
	runtimeExtractLimit  = 300 << 20
)

// runtimeIndexURL is deliberately a variable so release smoke tests can use a
// local signed-off fixture without changing production behaviour.
var runtimeIndexURL = "https://github.com/lemonade-lab/alemonjs-setup/releases/latest/download/redis-runtime-index.json"

type runtimeIndex struct {
	Version string         `json:"version"`
	Assets  []runtimeAsset `json:"assets"`
}
type runtimeAsset struct {
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
	Archive string `json:"archive"`
	Binary  string `json:"binary"`
}

func (m *Manager) prepareRuntime() {
	m.mu.Lock()
	if m.server == nil || m.private || m.config.Disabled {
		m.mu.Unlock()
		return
	}
	m.phase, m.message, m.retryable = "downloading-runtime", "正在下载 ALemonX 私有 Redis 运行时。", true
	m.mu.Unlock()

	version, err := downloadAndActivateRuntime(context.Background(), filepath.Dir(m.path))
	m.mu.Lock()
	if err != nil {
		m.runtimeDownloading = false
		m.phase, m.message, m.retryable = "failed", "私有 Redis 下载失败；仍在使用临时 Redis："+err.Error(), true
		m.mu.Unlock()
		return
	}
	m.runtimeVersion = version
	m.phase, m.message = "migrating", "私有 Redis 已准备好，正在等待安全迁移。"
	m.mu.Unlock()
	m.activatePreparedRuntime()
}

func (m *Manager) activatePreparedRuntime() {
	err := m.ActivatePrivateRuntime()
	m.mu.Lock()
	m.runtimeDownloading = false
	if err == nil {
		m.retryable = false
	}
	m.mu.Unlock()
}

func downloadAndActivateRuntime(ctx context.Context, base string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, runtimeIndexURL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("无法获取运行时索引：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("运行时索引返回 %s", response.Status)
	}
	var index runtimeIndex
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&index); err != nil || index.Version == "" {
		return "", errors.New("运行时索引无效")
	}
	asset, ok := selectRuntimeAsset(index.Assets)
	if !ok {
		return "", fmt.Errorf("没有适用于 %s/%s 的 Redis 运行时", runtime.GOOS, runtime.GOARCH)
	}
	if len(asset.SHA256) != 64 || asset.URL == "" || asset.Size <= 0 || asset.Size > runtimeDownloadLimit {
		return "", errors.New("运行时资产缺少有效校验信息")
	}
	root := filepath.Join(base, "redis-runtime")
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(root, ".staging-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	archive := filepath.Join(stage, "runtime.archive")
	if err := downloadRuntimeAsset(ctx, asset, archive); err != nil {
		return "", err
	}
	extracted := filepath.Join(stage, "contents")
	if err := extractRuntimeArchive(archive, extracted, asset.Archive); err != nil {
		return "", err
	}
	binary, err := findRuntimeBinary(extracted, asset.Binary)
	if err != nil {
		return "", err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(binary, 0700)
	}
	finalStage := filepath.Join(stage, "current")
	if err := os.MkdirAll(finalStage, 0700); err != nil {
		return "", err
	}
	if err := os.Rename(binary, filepath.Join(finalStage, runtimeBinaryName())); err != nil {
		return "", err
	}
	current := filepath.Join(root, "current")
	previous := filepath.Join(root, "previous")
	_ = os.RemoveAll(previous)
	if _, err := os.Stat(current); err == nil {
		if err := os.Rename(current, previous); err != nil {
			return "", err
		}
	}
	if err := os.Rename(finalStage, current); err != nil {
		if _, restoreErr := os.Stat(previous); restoreErr == nil {
			_ = os.Rename(previous, current)
		}
		return "", err
	}
	return index.Version, nil
}

func selectRuntimeAsset(assets []runtimeAsset) (runtimeAsset, bool) {
	for _, asset := range assets {
		if asset.OS == runtime.GOOS && asset.Arch == runtime.GOARCH {
			return asset, true
		}
	}
	return runtimeAsset{}, false
}

func downloadRuntimeAsset(ctx context.Context, asset runtimeAsset, destination string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return err
	}
	response, err := (&http.Client{Timeout: 10 * time.Minute}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("运行时下载返回 %s", response.Status)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	copied, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, runtimeDownloadLimit+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if copied != asset.Size || copied > runtimeDownloadLimit || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), asset.SHA256) {
		return errors.New("运行时下载校验失败")
	}
	return nil
}

func extractRuntimeArchive(source, destination, kind string) error {
	if kind == "zip" || strings.HasSuffix(strings.ToLower(source), ".zip") {
		return extractRuntimeZip(source, destination)
	}
	return extractRuntimeTarGz(source, destination)
}

func safeRuntimePath(root, name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", errors.New("运行时压缩包包含非法路径")
	}
	return filepath.Join(root, clean), nil
}
func extractRuntimeZip(source, destination string) error {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer archive.Close()
	var total int64
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return errors.New("运行时压缩包不允许链接文件")
		}
		target, err := safeRuntimePath(destination, entry.Name)
		if err != nil {
			return err
		}
		total += int64(entry.UncompressedSize64)
		if total > runtimeExtractLimit {
			return errors.New("运行时解压内容过大")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		in, err := entry.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
		if err == nil {
			_, err = io.Copy(out, io.LimitReader(in, runtimeExtractLimit+1))
			_ = out.Close()
		}
		_ = in.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
func extractRuntimeTarGz(source, destination string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	var total int64
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target, err := safeRuntimePath(destination, header.Name)
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0700); err != nil {
				return err
			}
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return errors.New("运行时压缩包不允许特殊文件")
		}
		total += header.Size
		if header.Size < 0 || total > runtimeExtractLimit {
			return errors.New("运行时解压内容过大")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, io.LimitReader(reader, runtimeExtractLimit+1))
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
func runtimeBinaryName() string {
	if runtime.GOOS == "windows" {
		return "redis-server.exe"
	}
	return "redis-server"
}
func findRuntimeBinary(root, name string) (string, error) {
	if name == "" {
		name = runtimeBinaryName()
	}
	var found string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == name {
			if found != "" {
				return errors.New("运行时压缩包包含多个 Redis 可执行文件")
			}
			found = path
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", errors.New("运行时压缩包未包含 redis-server")
	}
	return found, nil
}
