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
)

// DependencySourceStatus describes package-manager sources without changing
// the host. Only managers with a well-defined, reversible overlay are marked
// as writable.
type DependencySourceStatus struct {
	Supported    bool                     `json:"supported"`
	Writable     bool                     `json:"writable"`
	OS           string                   `json:"os"`
	Distribution string                   `json:"distribution"`
	Architecture string                   `json:"architecture"`
	Manager      string                   `json:"manager"`
	Reason       string                   `json:"reason,omitempty"`
	Target       string                   `json:"target,omitempty"`
	ActivePreset string                   `json:"activePreset,omitempty"`
	Presets      []DependencySourcePreset `json:"presets"`
	Backups      []DependencySourceBackup `json:"backups"`
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

func DependencySourceStatusSnapshot() DependencySourceStatus {
	status := DependencySourceStatus{OS: runtime.GOOS, Architecture: runtime.GOARCH, Presets: dependencySourcePresets()}
	status.Distribution = readOSReleaseValue("ID")
	if status.Distribution == "" {
		status.Distribution = runtime.GOOS
	}
	status.Manager, _ = hostPackageManager()
	status.Target = dependencySourceTarget(status.Manager)
	status.Supported = (status.Manager == "apt-get" && isAPTDistribution(status.Distribution)) || ((status.Manager == "dnf" || status.Manager == "yum") && isCentOSStream(status.Distribution, readOSReleaseValue("VARIANT_ID")))
	status.Writable = status.Supported && status.Target != ""
	status.ActivePreset = activeDependencySourcePreset(status.Target)
	if !status.Supported {
		status.Reason = "当前包管理器暂未提供可安全回滚的自动改源方案，ALemonX 不会直接修改系统源。"
	}
	for _, backup := range dependencySourceBackups() {
		if backup.Target == status.Target {
			status.Backups = append(status.Backups, backup)
		}
	}
	sort.Slice(status.Backups, func(i, j int) bool { return status.Backups[i].CreatedAt > status.Backups[j].CreatedAt })
	return status
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
	status := DependencySourceStatusSnapshot()
	if !status.Writable {
		return status, errors.New(status.Reason)
	}
	content, err := dependencySourceContent(status, preset)
	if err != nil {
		return status, err
	}
	check, err := testDependencySource(ctx, status, preset)
	if err != nil {
		return status, err
	}
	if !check.OK {
		return status, fmt.Errorf("镜像检测失败：%s", check.Message)
	}
	previous, err := readDependencySource(status.Target)
	if err != nil {
		return status, err
	}
	backupID, err := saveDependencySourceBackup(status.Target, "apply-"+preset, previous)
	if err != nil {
		return status, err
	}
	op := dependencySourceOperation{Action: "write", Target: status.Target, Content: content}
	if _, err := runDependencySourceOperation(op); err != nil {
		return status, err
	}
	if err := refreshDependencySource(ctx, status.Manager); err != nil {
		if rollbackErr := restoreDependencySourceContent(status.Target, previous); rollbackErr != nil {
			return status, fmt.Errorf("依赖源刷新失败：%v；自动回滚也失败：%v（备份 %s）", err, rollbackErr, backupID)
		}
		return status, fmt.Errorf("依赖源刷新失败，已自动回滚到应用前状态（备份 %s）：%v", backupID, err)
	}
	return DependencySourceStatusSnapshot(), nil
}

func RestoreDependencySource(ctx context.Context, id string) (DependencySourceStatus, error) {
	if strings.ContainsAny(id, `/\\`) || strings.TrimSpace(id) == "" {
		return DependencySourceStatusSnapshot(), errors.New("备份编号无效")
	}
	path := filepath.Join(dependencySourceConfigDir(), dependencySourceBackupDir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return DependencySourceStatusSnapshot(), errors.New("备份不存在或无法读取")
	}
	var backup struct {
		Target   string `json:"target"`
		Content  string `json:"content"`
		Checksum string `json:"checksum"`
	}
	if json.Unmarshal(data, &backup) != nil || backup.Target == "" {
		return DependencySourceStatusSnapshot(), errors.New("备份内容无效")
	}
	if backup.Checksum != "" && backup.Checksum != dependencySourceChecksum(backup.Content) {
		return DependencySourceStatusSnapshot(), errors.New("备份校验失败，已拒绝恢复")
	}
	status := DependencySourceStatusSnapshot()
	if backup.Target != status.Target {
		return status, errors.New("备份与当前系统源不匹配")
	}
	current, err := readDependencySource(status.Target)
	if err != nil {
		return status, err
	}
	backupID, err := saveDependencySourceBackup(status.Target, "before-restore", current)
	if err != nil {
		return status, err
	}
	action := "write"
	if backup.Content == "" {
		action = "delete"
	}
	if _, err := runDependencySourceOperation(dependencySourceOperation{Action: action, Target: backup.Target, Content: backup.Content}); err != nil {
		return status, err
	}
	if err := refreshDependencySource(ctx, status.Manager); err != nil {
		if rollbackErr := restoreDependencySourceContent(status.Target, current); rollbackErr != nil {
			return status, fmt.Errorf("恢复后的依赖源刷新失败：%v；自动回滚也失败：%v（备份 %s）", err, rollbackErr, backupID)
		}
		return status, fmt.Errorf("恢复后的依赖源刷新失败，已自动回滚（备份 %s）：%v", backupID, err)
	}
	return DependencySourceStatusSnapshot(), nil
}

// TestDependencySource checks the repository metadata address without
// changing system sources or package-manager caches.
func TestDependencySource(ctx context.Context, preset string) (DependencySourceCheck, error) {
	status := DependencySourceStatusSnapshot()
	if !status.Writable {
		return DependencySourceCheck{}, errors.New(status.Reason)
	}
	if _, err := dependencySourceContent(status, preset); err != nil {
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

func dependencySourceContent(status DependencySourceStatus, preset string) (string, error) {
	if preset != "aliyun" && preset != "tencent" && preset != "official" {
		return "", errors.New("未知的依赖镜像")
	}
	if status.Manager == "apt-get" {
		codename := readOSReleaseValue("VERSION_CODENAME")
		if codename == "" {
			codename = "stable"
		}
		if strings.Contains(strings.ToLower(status.Distribution), "ubuntu") {
			return aptContent(preset, "ubuntu", codename), nil
		}
		return aptContent(preset, "debian", codename), nil
	}
	base := map[string]string{"aliyun": "https://mirrors.aliyun.com/centos-stream", "tencent": "https://mirrors.cloud.tencent.com/centos-stream", "official": "https://mirror.stream.centos.org"}[preset]
	version := readOSReleaseValue("VERSION_ID")
	if version == "" {
		return "", errors.New("无法识别 CentOS Stream 版本，已拒绝修改软件源")
	}
	if version == "9" {
		version = "9-stream"
	}
	gpgKey := "file:///etc/pki/rpm-gpg/RPM-GPG-KEY-centosofficial"
	return fmt.Sprintf("# Managed by ALemonX. Restore the backup before removing this file.\n[alemonx-baseos]\nname=ALemonX BaseOS\nbaseurl=%s/%s/BaseOS/$basearch/os\nenabled=1\ngpgcheck=1\ngpgkey=%s\n\n[alemonx-appstream]\nname=ALemonX AppStream\nbaseurl=%s/%s/AppStream/$basearch/os\nenabled=1\ngpgcheck=1\ngpgkey=%s\n", base, version, gpgKey, base, version, gpgKey), nil
}

func isAPTDistribution(distribution string) bool {
	distribution = strings.ToLower(strings.TrimSpace(distribution))
	return distribution == "debian" || distribution == "ubuntu"
}

func isCentOSStream(distribution, variant string) bool {
	return strings.EqualFold(strings.TrimSpace(distribution), "centos") && strings.EqualFold(strings.TrimSpace(variant), "stream")
}

func aptContent(preset, distro, codename string) string {
	host := map[string]string{"aliyun": "https://mirrors.aliyun.com", "tencent": "https://mirrors.cloud.tencent.com", "official": "https://deb.debian.org"}[preset]
	if distro == "ubuntu" {
		if preset == "official" {
			host = "https://archive.ubuntu.com"
		}
		return fmt.Sprintf("# Managed by ALemonX. Restore the backup before removing this file.\ndeb %s/ubuntu %s main restricted universe multiverse\ndeb %s/ubuntu %s-updates main restricted universe multiverse\n", host, codename, host, codename)
	}
	return fmt.Sprintf("# Managed by ALemonX. Restore the backup before removing this file.\ndeb %s/debian %s main contrib non-free non-free-firmware\ndeb %s/debian %s-updates main contrib non-free non-free-firmware\n", host, codename, host, codename)
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
	backupID := time.Now().UTC().Format("20060102T150405.000000000Z")
	backupPath := filepath.Join(dependencySourceConfigDir(), dependencySourceBackupDir, backupID+".json")
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o700); err != nil {
		return "", err
	}
	backup := map[string]any{"schema": 2, "id": backupID, "createdAt": time.Now().UTC().Format(time.RFC3339), "preset": preset, "target": target, "checksum": dependencySourceChecksum(content), "content": content}
	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(backupPath, data, 0o600); err != nil {
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

func restoreDependencySourceContent(target, content string) error {
	action := "write"
	if content == "" {
		action = "delete"
	}
	_, err := runDependencySourceOperation(dependencySourceOperation{Action: action, Target: target, Content: content})
	return err
}

func refreshDependencySource(ctx context.Context, manager string) error {
	args := map[string][]string{
		"apt-get": {"update"},
		"dnf":     {"makecache"},
		"yum":     {"makecache"},
	}[manager]
	if len(args) == 0 {
		return errors.New("当前包管理器暂未提供安全的索引刷新命令")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := runDependencySourceCommand(ctx, manager, args); err != nil {
		return errors.New("包管理器索引刷新失败")
	}
	return nil
}

func runDependencySourceCommand(ctx context.Context, program string, args []string) (string, error) {
	if os.Geteuid() == 0 {
		command := exec.CommandContext(ctx, program, args...)
		output, err := command.CombinedOutput()
		return string(output), err
	}
	output, err := RunWithPrivilegesInput("", program, args, nil, nil)
	return string(output), err
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
	if json.Unmarshal(data, &op) != nil || (op.Action != "write" && op.Action != "read" && op.Action != "delete") || !validDependencySourceTarget(op.Target) {
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
	if err := os.MkdirAll(filepath.Dir(op.Target), 0o755); err != nil {
		return 1
	}
	tmp, err := os.CreateTemp(filepath.Dir(op.Target), ".alemonx-source-*")
	if err != nil {
		return 1
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.WriteString(op.Content); err == nil {
		err = tmp.Chmod(0o644)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return 1
	}
	if err = os.Rename(tmp.Name(), op.Target); err != nil {
		return 1
	}
	return 0
}

func validDependencySourceTarget(target string) bool {
	return target == "/etc/apt/sources.list.d/alemonx-mirror.list" || target == "/etc/yum.repos.d/alemonx-mirror.repo"
}

func dependencySourceConfigDir() string {
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
