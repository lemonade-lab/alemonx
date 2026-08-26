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
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// DependencySourceStatus describes package-manager sources without changing
// the host. Only managers with a well-defined, reversible overlay are marked
// as writable.
type DependencySourceStatus struct {
	Supported           bool                     `json:"supported"`
	Writable            bool                     `json:"writable"`
	Mode                string                   `json:"mode"`
	ChecksAvailable     bool                     `json:"checksAvailable"`
	OS                  string                   `json:"os"`
	Distribution        string                   `json:"distribution"`
	Architecture        string                   `json:"architecture"`
	Manager             string                   `json:"manager"`
	Reason              string                   `json:"reason,omitempty"`
	Target              string                   `json:"target,omitempty"`
	ActivePreset        string                   `json:"activePreset,omitempty"`
	Managed             bool                     `json:"managed"`
	LegacyManagedSource bool                     `json:"legacyManagedSource"`
	CleanupAvailable    bool                     `json:"cleanupAvailable"`
	SameNameUnmanaged   bool                     `json:"sameNameUnmanaged"`
	ServerBuild         string                   `json:"serverBuild,omitempty"`
	FrontendBuild       string                   `json:"frontendBuild,omitempty"`
	Presets             []DependencySourcePreset `json:"presets"`
	Backups             []DependencySourceBackup `json:"backups"`
}

type DependencySourcePreset struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type DependencySourceBackup struct {
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
	Preset    string `json:"preset"`
	Target    string `json:"target"`
	Checksum  string `json:"checksum,omitempty"`
}

type DependencySourceCheck struct {
	OK        bool   `json:"ok"`
	URL       string `json:"url"`
	Status    int    `json:"status,omitempty"`
	LatencyMS int64  `json:"latencyMs,omitempty"`
	Message   string `json:"message"`
}

type dependencySourceOperation struct {
	Action  string `json:"action"`
	Target  string `json:"target"`
	Content string `json:"content"`
}

const dependencySourceBackupDir = "dependency-mirrors/backups"
const dependencySourceOwnershipMarker = "Managed by ALemonX"
const dependencySourceWritesDisabledReason = "系统依赖源自动写入已停用：MVP 仅提供只读连通性检查和旧版 ALemonX 受管源清理。"

var dependencySourceConfigRoot = defaultDependencySourceConfigDir

func DependencySourceStatusSnapshot() DependencySourceStatus {
	status := DependencySourceStatus{OS: runtime.GOOS, Architecture: runtime.GOARCH, Mode: "readonly", Backups: []DependencySourceBackup{}}
	status.Distribution = readOSReleaseValue("ID")
	if status.Distribution == "" {
		status.Distribution = runtime.GOOS
	}
	status.Manager, _ = hostPackageManager()
	status.Target = dependencySourceTarget(status.Manager)
	status.ChecksAvailable = (status.Manager == "apt-get" && isAPTDistribution(status.Distribution)) || ((status.Manager == "dnf" || status.Manager == "yum") && isCentOSStream(status.Distribution, readOSReleaseValue("VARIANT_ID"), readOSReleaseValue("NAME"), readOSReleaseValue("PRETTY_NAME")))
	// The panel is intentionally visible whenever a host package manager is
	// found, even when no template is verified. It must report why it is read
	// only instead of silently inviting the UI to guess a source format.
	status.Supported = status.Manager != ""
	status.Writable = false
	if status.ChecksAvailable {
		status.Presets = dependencySourcePresets()
		status.Reason = dependencySourceWritesDisabledReason
	} else {
		status.Reason = "当前平台尚无经过验证的镜像模板，ALemonX 仅显示环境状态，不会修改系统仓库。"
	}
	if status.Target != "" {
		if content, err := os.ReadFile(status.Target); err == nil {
			if isALemonXManagedDependencySource(string(content)) {
				status.LegacyManagedSource = true
				status.Managed = true // compatibility for older clients.
				status.CleanupAvailable = true
				status.Mode = "legacy-cleanup"
				status.ActivePreset = activeDependencySourcePreset(status.Target)
			} else {
				status.SameNameUnmanaged = true
			}
		}
	}
	for _, backup := range dependencySourceBackups() {
		if backup.Target == status.Target {
			status.Backups = append(status.Backups, backup)
		}
	}
	sort.Slice(status.Backups, func(i, j int) bool { return status.Backups[i].CreatedAt > status.Backups[j].CreatedAt })
	return status
}

func isALemonXManagedDependencySource(content string) bool {
	return strings.Contains(content, dependencySourceOwnershipMarker)
}

func activeDependencySourcePreset(target string) string {
	if !validDependencySourceTarget(target) {
		return ""
	}
	content, err := os.ReadFile(target)
	if err != nil {
		return ""
	}
	value := string(content)
	switch {
	case strings.Contains(value, "mirrors.aliyun.com"):
		return "aliyun"
	case strings.Contains(value, "mirrors.cloud.tencent.com"):
		return "tencent"
	case strings.Contains(value, "deb.debian.org"), strings.Contains(value, "archive.ubuntu.com"), strings.Contains(value, "mirror.stream.centos.org"):
		return "official"
	default:
		return ""
	}
}

func dependencySourcePresets() []DependencySourcePreset {
	return []DependencySourcePreset{
		{ID: "aliyun", Name: "阿里云", Description: "中国大陆常用镜像，适合大多数服务器。"},
		{ID: "tencent", Name: "腾讯云", Description: "中国大陆常用镜像。"},
		{ID: "official", Name: "官方源", Description: "恢复为包管理器官方源。"},
	}
}

func dependencySourceTarget(manager string) string {
	switch manager {
	case "apt-get":
		return "/etc/apt/sources.list.d/alemonx-mirror.list"
	case "dnf", "yum":
		return "/etc/yum.repos.d/alemonx-mirror.repo"
	default:
		return ""
	}
}

func ApplyDependencySource(ctx context.Context, preset string) (DependencySourceStatus, error) {
	return DependencySourceStatusSnapshot(), errors.New(dependencySourceWritesDisabledReason)
}

func RestoreDependencySource(ctx context.Context, id string) (DependencySourceStatus, error) {
	return DependencySourceStatusSnapshot(), errors.New("旧版依赖源备份仅供审计；为避免重新启用不兼容仓库，恢复写入已停用。")
}

// TestDependencySource checks the repository metadata address without
// changing system sources or package-manager caches.
func TestDependencySource(ctx context.Context, preset string) (DependencySourceCheck, error) {
	status := DependencySourceStatusSnapshot()
	if !status.ChecksAvailable {
		return DependencySourceCheck{}, errors.New(status.Reason)
	}
	if _, err := dependencySourceCheckURL(status, preset); err != nil {
		return DependencySourceCheck{}, err
	}
	return testDependencySource(ctx, status, preset)
}

func testDependencySource(ctx context.Context, status DependencySourceStatus, preset string) (DependencySourceCheck, error) {
	url, err := dependencySourceCheckURL(status, preset)
	if err != nil {
		return DependencySourceCheck{}, err
	}
	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return DependencySourceCheck{}, err
	}
	req.Header.Set("Range", "bytes=0-0")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return DependencySourceCheck{URL: url, LatencyMS: time.Since(started).Milliseconds(), Message: "无法连接镜像仓库。"}, nil
	}
	defer response.Body.Close()
	ok := response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices
	message := "镜像仓库元数据可访问。"
	if !ok {
		message = "镜像仓库未返回可用的元数据。"
	}
	return DependencySourceCheck{OK: ok, URL: url, Status: response.StatusCode, LatencyMS: time.Since(started).Milliseconds(), Message: message}, nil
}

func isAPTDistribution(distribution string) bool {
	distribution = strings.ToLower(strings.TrimSpace(distribution))
	return distribution == "debian" || distribution == "ubuntu"
}

func isCentOSStream(distribution, variant, name, prettyName string) bool {
	if !strings.EqualFold(strings.TrimSpace(distribution), "centos") {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(variant), "stream") {
		return true
	}
	identity := strings.ToLower(strings.TrimSpace(name + " " + prettyName))
	return strings.Contains(identity, "centos stream")
}

func dependencySourceCheckURL(status DependencySourceStatus, preset string) (string, error) {
	if status.Manager == "apt-get" {
		codename := readOSReleaseValue("VERSION_CODENAME")
		if codename == "" {
			return "", errors.New("无法识别系统版本代号")
		}
		if strings.EqualFold(status.Distribution, "ubuntu") {
			host := map[string]string{"aliyun": "https://mirrors.aliyun.com", "tencent": "https://mirrors.cloud.tencent.com", "official": "https://archive.ubuntu.com"}[preset]
			return fmt.Sprintf("%s/ubuntu/dists/%s/Release", host, codename), nil
		}
		host := map[string]string{"aliyun": "https://mirrors.aliyun.com", "tencent": "https://mirrors.cloud.tencent.com", "official": "https://deb.debian.org"}[preset]
		return fmt.Sprintf("%s/debian/dists/%s/Release", host, codename), nil
	}
	version := readOSReleaseValue("VERSION_ID")
	if version == "" {
		return "", errors.New("无法识别 CentOS Stream 版本")
	}
	if version == "9" {
		version = "9-stream"
	}
	base := map[string]string{"aliyun": "https://mirrors.aliyun.com/centos-stream", "tencent": "https://mirrors.cloud.tencent.com/centos-stream", "official": "https://mirror.stream.centos.org"}[preset]
	architecture := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[runtime.GOARCH]
	if architecture == "" {
		return "", errors.New("当前架构暂未提供 CentOS Stream 镜像检测")
	}
	return fmt.Sprintf("%s/%s/BaseOS/%s/os/repodata/repomd.xml", base, version, architecture), nil
}

func saveDependencySourceBackup(target, preset, content string) (string, error) {
	directory := filepath.Join(dependencySourceConfigDir(), dependencySourceBackupDir)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	baseID := time.Now().UTC().Format("20060102T150405.000000000Z")
	backupID, backupPath := baseID, filepath.Join(directory, baseID+".json")
	for index := 1; ; index++ {
		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			break
		} else if err != nil {
			return "", err
		}
		backupID = fmt.Sprintf("%s-%d", baseID, index)
		backupPath = filepath.Join(directory, backupID+".json")
	}
	backup := map[string]any{"schema": 2, "id": backupID, "createdAt": time.Now().UTC().Format(time.RFC3339Nano), "preset": preset, "target": target, "checksum": dependencySourceChecksum(content), "content": content}
	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(directory, ".alemonx-backup-*")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err == nil {
		err = temporary.Chmod(0o600)
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if err := os.Rename(temporaryPath, backupPath); err != nil {
		return "", err
	}
	pruneDependencySourceBackups(target, 30)
	return backupID, nil
}

func dependencySourceChecksum(content string) string {
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("sha256:%x", sum[:])
}

func DeleteDependencySourceBackup(id string) (DependencySourceStatus, error) {
	if strings.ContainsAny(id, `/\\`) || strings.TrimSpace(id) == "" {
		return DependencySourceStatusSnapshot(), errors.New("备份编号无效")
	}
	path := filepath.Join(dependencySourceConfigDir(), dependencySourceBackupDir, id+".json")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return DependencySourceStatusSnapshot(), errors.New("备份不存在或已删除")
		}
		return DependencySourceStatusSnapshot(), err
	}
	return DependencySourceStatusSnapshot(), nil
}

// RemoveManagedDependencySource removes only the fixed ALemonX-owned source
// file. It never changes any distribution-owned repository file.
func RemoveManagedDependencySource() (DependencySourceStatus, error) {
	status := DependencySourceStatusSnapshot()
	if status.Target == "" || !validDependencySourceTarget(status.Target) || !status.CleanupAvailable {
		return status, errors.New("当前系统没有可移除的 ALemonX 受管依赖源")
	}
	previous, err := readDependencySource(status.Target)
	if err != nil {
		return status, err
	}
	if !isALemonXManagedDependencySource(previous) {
		return status, errors.New("同名文件不属于 ALemonX，已拒绝删除")
	}
	if _, err := saveDependencySourceBackup(status.Target, "remove-managed-source", previous); err != nil {
		return status, err
	}
	if _, err := runDependencySourceOperation(dependencySourceOperation{Action: "delete", Target: status.Target}); err != nil {
		return status, err
	}
	return DependencySourceStatusSnapshot(), nil
}

func runDependencySourceOperation(op dependencySourceOperation) (string, error) {
	data, _ := json.Marshal(op)
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		old, _ := os.ReadFile(op.Target)
		if dependencySourceOperationHelperTo(data, io.Discard) != 0 {
			return "", errors.New("写入系统依赖源失败")
		}
		return string(old), nil
	}
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	output, err := RunWithPrivilegesInput("", executable, []string{"__alx-dependency-source"}, data, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func readDependencySource(target string) (string, error) {
	if !validDependencySourceTarget(target) {
		return "", errors.New("依赖源目标无效")
	}
	data, _ := json.Marshal(dependencySourceOperation{Action: "read", Target: target})
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		var output strings.Builder
		if dependencySourceOperationHelperTo(data, &output) != 0 {
			return "", errors.New("读取系统依赖源失败")
		}
		return output.String(), nil
	}
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	output, err := RunWithPrivilegesInput("", executable, []string{"__alx-dependency-source"}, data, nil)
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func DependencySourceOperationHelper(data []byte) int {
	return dependencySourceOperationHelperTo(data, os.Stdout)
}

func dependencySourceOperationHelperTo(data []byte, output io.Writer) int {
	var op dependencySourceOperation
	if json.Unmarshal(data, &op) != nil || (op.Action != "read" && op.Action != "delete") || !validDependencySourceTarget(op.Target) {
		return 2
	}
	if op.Action == "read" {
		old, _ := os.ReadFile(op.Target)
		if _, err := output.Write(old); err != nil {
			return 1
		}
		return 0
	}
	if op.Action == "delete" {
		if err := os.Remove(op.Target); err != nil && !os.IsNotExist(err) {
			return 1
		}
		return 0
	}
	return 2
}

func validDependencySourceTarget(target string) bool {
	return target == "/etc/apt/sources.list.d/alemonx-mirror.list" || target == "/etc/yum.repos.d/alemonx-mirror.repo"
}

func dependencySourceConfigDir() string {
	return dependencySourceConfigRoot()
}

func defaultDependencySourceConfigDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "."
	}
	return filepath.Join(dir, "alemonx")
}
func dependencySourceBackups() []DependencySourceBackup {
	entries, _ := os.ReadDir(filepath.Join(dependencySourceConfigDir(), dependencySourceBackupDir))
	result := []DependencySourceBackup{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(dependencySourceConfigDir(), dependencySourceBackupDir, entry.Name()))
		var b DependencySourceBackup
		if json.Unmarshal(data, &b) == nil {
			result = append(result, b)
		}
	}
	return result
}

func pruneDependencySourceBackups(target string, keep int) {
	if keep < 1 {
		return
	}
	backups := dependencySourceBackups()
	filtered := make([]DependencySourceBackup, 0, len(backups))
	for _, backup := range backups {
		if backup.Target == target {
			filtered = append(filtered, backup)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].CreatedAt > filtered[j].CreatedAt })
	if len(filtered) <= keep {
		return
	}
	for _, backup := range filtered[keep:] {
		if strings.ContainsAny(backup.ID, `/\\`) || backup.ID == "" {
			continue
		}
		_ = os.Remove(filepath.Join(dependencySourceConfigDir(), dependencySourceBackupDir, backup.ID+".json"))
	}
}
func readOSReleaseValue(key string) string {
	data, _ := os.ReadFile("/etc/os-release")
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, key+"=") {
			return strings.Trim(strings.TrimPrefix(line, key+"="), `"`)
		}
	}
	return ""
}
