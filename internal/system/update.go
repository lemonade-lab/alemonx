package system

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"alemonx/internal/systemnetwork"
)

const maxUpdateSize int64 = 200 << 20
const updateDownloadTimeout = 10 * time.Minute

var errUpdatePackageManifestMissing = errors.New("更新包缺少版本清单")

type UpdatePhase string

const (
	UpdatePhaseChecking    UpdatePhase = "checking"
	UpdatePhaseDownloading UpdatePhase = "downloading"
	UpdatePhaseStaged      UpdatePhase = "staged"
	UpdatePhaseApplying    UpdatePhase = "applying"
	UpdatePhaseRestarting  UpdatePhase = "restarting"
	UpdatePhaseHealthy     UpdatePhase = "healthy"
	UpdatePhaseRolledBack  UpdatePhase = "rolled_back"
	UpdatePhaseFailed      UpdatePhase = "failed"
)

// UpdateTransaction is the durable source of truth for an update attempt. It
// deliberately lives beside the verified download cache so a restarted server
// can report the outcome to the browser that initiated the update.
type UpdateTransaction struct {
	Phase           UpdatePhase `json:"phase"`
	TargetVersion   string      `json:"targetVersion,omitempty"`
	PreviousVersion string      `json:"previousVersion,omitempty"`
	ArchivePath     string      `json:"archivePath,omitempty"`
	Executable      string      `json:"executable,omitempty"`
	BackupPath      string      `json:"backupPath,omitempty"`
	Port            string      `json:"port,omitempty"`
	Runtime         string      `json:"runtime,omitempty"`
	PluginError     string      `json:"pluginError,omitempty"`
	Error           string      `json:"error,omitempty"`
	UpdatedAt       time.Time   `json:"updatedAt"`
}

const (
	updateRuntimeDirect      = "direct"
	updateRuntimeSystemd     = "systemd-user"
	updateRuntimeLaunchAgent = "launch-agent"
	updateRuntimeTask        = "scheduled-task"
)

// UpdateRuntime records the supervisor while the old service is still alive.
// The restart watcher runs after shutdown, when probing the active state would
// otherwise incorrectly classify a managed service as a direct process.
func UpdateRuntime() string {
	switch runtime.GOOS {
	case "linux":
		if exec.Command("systemctl", "--user", "is-active", "--quiet", "alx.service").Run() == nil {
			return updateRuntimeSystemd
		}
	case "darwin":
		target := fmt.Sprintf("gui/%d/%s", os.Getuid(), serviceName)
		if exec.Command("launchctl", "print", target).Run() == nil {
			return updateRuntimeLaunchAgent
		}
	case "windows":
		if windowsScheduledTaskInstalled() {
			return updateRuntimeTask
		}
	}
	return updateRuntimeDirect
}

type PendingUpdate struct {
	AssetName string `json:"assetName"`
	SHA256    string `json:"sha256"`
	Version   string `json:"version"`
}

type UpdatePackageManifest struct {
	Version      string `json:"version"`
	Platform     string `json:"platform"`
	Architecture string `json:"architecture"`
}

type StagedUpdate struct {
	Path    string
	Version string
}

type AppliedUpdate struct {
	Message    string
	Executable string
	BackupPath string
}

// ReplaceExecutable downloads a release asset and atomically replaces the
// running program on systems that allow it. It only accepts a concrete asset
// URL chosen by the version and platform matcher.
func ReplaceExecutable(downloadURL, assetName, checksum, version string) (string, error) {
	downloaded, err := DownloadUpdate(downloadURL, assetName, checksum, version)
	if err != nil {
		return "", err
	}
	return ReplaceExecutableFile(downloaded)
}

// DownloadUpdate verifies a Release archive's SHA-256 before retaining it in
// the user's cache, so it can be applied later without another network call.
func DownloadUpdate(downloadURL, assetName, checksum, version string) (string, error) {
	if downloadURL == "" || len(checksum) != 64 {
		return "", errors.New("没有可用的匹配安装包")
	}
	path, exists, err := CachedUpdate(assetName, checksum)
	if err != nil {
		return "", err
	}
	if exists {
		if err := savePendingUpdate(PendingUpdate{AssetName: assetName, SHA256: checksum, Version: version}); err != nil {
			return "", err
		}
		return path, nil
	}
	response, err := systemnetwork.DefaultClient(updateDownloadTimeout).Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("下载更新失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载更新失败：服务器返回 %s", response.Status)
	}
	partial := path + ".part"
	file, err := os.OpenFile(partial, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(file, io.LimitReader(response.Body, maxUpdateSize+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(partial)
		if copyErr != nil {
			return "", copyErr
		}
		return "", closeErr
	}
	if err := os.Rename(partial, path); err != nil {
		return "", err
	}
	if _, ready, err := CachedUpdate(assetName, checksum); err != nil || !ready {
		_ = os.Remove(path)
		if err != nil {
			return "", err
		}
		return "", errors.New("更新包校验失败")
	}
	if err := savePendingUpdate(PendingUpdate{AssetName: assetName, SHA256: checksum, Version: version}); err != nil {
		return "", err
	}
	return path, nil
}

// CachedUpdate returns the persistent location for one release asset and
// whether a complete file is already available there.
func CachedUpdate(assetName, checksum string) (string, bool, error) {
	assetName = filepath.Base(assetName)
	if assetName == "." || assetName == "" || assetName == string(filepath.Separator) {
		return "", false, errors.New("更新包名称无效")
	}
	base, err := updateCacheBase()
	if err != nil {
		return "", false, fmt.Errorf("无法定位应用存储目录：%w", err)
	}
	directory := filepath.Join(base, "alemonjs", "alx", "updates")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", false, fmt.Errorf("无法创建更新存储目录：%w", err)
	}
	path := filepath.Join(directory, assetName)
	info, err := os.Stat(path)
	if err == nil && info.Mode().IsRegular() && info.Size() > 0 && info.Size() <= maxUpdateSize && checksumFile(path, checksum) {
		return path, true, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return "", false, err
	}
	return path, false, nil
}

func checksumFile(path, expected string) bool {
	input, err := os.Open(path)
	if err != nil {
		return false
	}
	defer input.Close()
	digest := sha256.New()
	_, err = io.Copy(digest, io.LimitReader(input, maxUpdateSize+1))
	return err == nil && fmt.Sprintf("%x", digest.Sum(nil)) == strings.ToLower(expected)
}

func pendingUpdatePath() (string, error) {
	base, err := updateCacheBase()
	if err != nil {
		return "", fmt.Errorf("无法定位应用存储目录：%w", err)
	}
	directory := filepath.Join(base, "alemonjs", "alx", "updates")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	return filepath.Join(directory, "pending.json"), nil
}

func updateTransactionPath() (string, error) {
	base, err := updateCacheBase()
	if err != nil {
		return "", fmt.Errorf("无法定位应用存储目录：%w", err)
	}
	directory := filepath.Join(base, "alemonjs", "alx", "updates")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	return filepath.Join(directory, "transaction.json"), nil
}

func ReadUpdateTransaction() (UpdateTransaction, bool, error) {
	path, err := updateTransactionPath()
	if err != nil {
		return UpdateTransaction{}, false, err
	}
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return UpdateTransaction{}, false, nil
	}
	if err != nil {
		return UpdateTransaction{}, false, err
	}
	var transaction UpdateTransaction
	if err := json.Unmarshal(body, &transaction); err != nil || transaction.Phase == "" {
		return UpdateTransaction{}, false, errors.New("更新事务状态无效")
	}
	return transaction, true, nil
}

func SaveUpdateTransaction(transaction UpdateTransaction) error {
	path, err := updateTransactionPath()
	if err != nil {
		return err
	}
	transaction.UpdatedAt = time.Now().UTC()
	body, err := json.Marshal(transaction)
	if err != nil {
		return err
	}
	temporary := path + ".new"
	if err := os.WriteFile(temporary, body, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

// MarkUpdateHealthy is called by the newly started server. Matching the
// expected release version proves the replacement survived the restart.
func MarkUpdateHealthy(version string) (UpdateTransaction, bool, error) {
	transaction, ok, err := ReadUpdateTransaction()
	if err != nil || !ok || transaction.Phase != UpdatePhaseRestarting || (transaction.TargetVersion != "" && !sameUpdateVersion(transaction.TargetVersion, version)) {
		return transaction, false, err
	}
	transaction.Phase = UpdatePhaseHealthy
	transaction.Error = ""
	if err := SaveUpdateTransaction(transaction); err != nil {
		return transaction, false, err
	}
	return transaction, true, nil
}

func sameUpdateVersion(left, right string) bool {
	return strings.TrimPrefix(strings.TrimSpace(left), "v") == strings.TrimPrefix(strings.TrimSpace(right), "v")
}

func updateCacheBase() (string, error) {
	if dir := os.Getenv("ALX_TEST_CACHE_DIR"); dir != "" {
		return dir, nil
	}
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return dir, nil
	}
	return os.UserCacheDir()
}

func savePendingUpdate(update PendingUpdate) error {
	path, err := pendingUpdatePath()
	if err != nil {
		return err
	}
	body, err := json.Marshal(update)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0600)
}

// ReadyPendingUpdate verifies and returns the package selected when the user
// downloaded it. Applying it therefore does not need another GitHub request.
func ReadyPendingUpdate() (PendingUpdate, string, bool, error) {
	path, err := pendingUpdatePath()
	if err != nil {
		return PendingUpdate{}, "", false, err
	}
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return PendingUpdate{}, "", false, nil
	}
	if err != nil {
		return PendingUpdate{}, "", false, err
	}
	var update PendingUpdate
	if err := json.Unmarshal(body, &update); err != nil || update.AssetName == "" || len(update.SHA256) != 64 {
		return PendingUpdate{}, "", false, errors.New("缓存的更新元数据无效")
	}
	archive, ready, err := CachedUpdate(update.AssetName, update.SHA256)
	return update, archive, ready, err
}

func ClearPendingUpdate() error {
	path, err := pendingUpdatePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// StageUploadedUpdate moves a just-uploaded archive into the verified update
// cache and records it as pending. Applying it later reuses the exact same
// apply path as an auto-downloaded release.
func StageUploadedUpdate(source, filename, expectedVersion string) (StagedUpdate, error) {
	filename = filepath.Base(filename)
	if filename == "." || filename == "" {
		return StagedUpdate{}, errors.New("更新包名称无效")
	}
	manifest, err := inspectUpdatePackage(source, expectedVersion)
	if err != nil {
		return StagedUpdate{}, err
	}
	base, err := updateCacheBase()
	if err != nil {
		return StagedUpdate{}, fmt.Errorf("无法定位应用存储目录：%w", err)
	}
	directory := filepath.Join(base, "alemonjs", "alx", "updates")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return StagedUpdate{}, fmt.Errorf("无法创建更新存储目录：%w", err)
	}
	path := filepath.Join(directory, filename)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return StagedUpdate{}, err
	}
	input, err := os.Open(source)
	if err != nil {
		return StagedUpdate{}, err
	}
	defer input.Close()
	output, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return StagedUpdate{}, err
	}
	copied, copyErr := io.Copy(output, io.LimitReader(input, maxUpdateSize+1))
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil || copied > maxUpdateSize {
		_ = os.Remove(path)
		if copyErr != nil {
			return StagedUpdate{}, copyErr
		}
		if copied > maxUpdateSize {
			return StagedUpdate{}, errors.New("更新包超过 200 MB 限制")
		}
		return StagedUpdate{}, closeErr
	}
	sum, err := sha256File(path)
	if err != nil {
		_ = os.Remove(path)
		return StagedUpdate{}, err
	}
	if err := savePendingUpdate(PendingUpdate{AssetName: filename, SHA256: sum, Version: manifest.Version}); err != nil {
		_ = os.Remove(path)
		return StagedUpdate{}, err
	}
	return StagedUpdate{Path: path, Version: manifest.Version}, nil
}

func inspectUpdatePackage(source, expectedVersion string) (UpdatePackageManifest, error) {
	temporary, err := os.MkdirTemp("", "alx-update-inspect-")
	if err != nil {
		return UpdatePackageManifest{}, err
	}
	defer os.RemoveAll(temporary)
	binary, err := releaseBinary(source, temporary)
	if err != nil {
		return UpdatePackageManifest{}, fmt.Errorf("更新包无法解压：%w", err)
	}
	if err := verifyBinaryPlatform(binary); err != nil {
		return UpdatePackageManifest{}, err
	}
	manifest, err := readUpdatePackageManifest(source)
	if err != nil {
		if errors.Is(err, errUpdatePackageManifestMissing) && strings.TrimSpace(expectedVersion) != "" {
			// Legacy Release archives predate alx-update.json. They remain
			// installable only when the operator explicitly selected a release
			// version; post-restart health verification will enforce that target.
			return UpdatePackageManifest{Version: strings.TrimSpace(expectedVersion), Platform: runtime.GOOS, Architecture: runtime.GOARCH}, nil
		}
		return UpdatePackageManifest{}, err
	}
	if strings.TrimSpace(manifest.Version) == "" || manifest.Platform != runtime.GOOS || manifest.Architecture != runtime.GOARCH {
		return UpdatePackageManifest{}, errors.New("更新包版本、系统或架构信息与当前应用不匹配")
	}
	return manifest, nil
}

func readUpdatePackageManifest(source string) (UpdatePackageManifest, error) {
	if !strings.HasSuffix(strings.ToLower(source), ".zip") {
		return UpdatePackageManifest{}, errUpdatePackageManifestMissing
	}
	archive, err := zip.OpenReader(source)
	if err != nil {
		return UpdatePackageManifest{}, errors.New("更新包不是有效的 zip 文件")
	}
	defer archive.Close()
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() || filepath.Base(entry.Name) != "alx-update.json" || entry.UncompressedSize64 > 64<<10 {
			continue
		}
		input, err := entry.Open()
		if err != nil {
			return UpdatePackageManifest{}, err
		}
		body, readErr := io.ReadAll(io.LimitReader(input, 64<<10))
		_ = input.Close()
		var manifest UpdatePackageManifest
		if readErr != nil || json.Unmarshal(body, &manifest) != nil {
			return UpdatePackageManifest{}, errors.New("更新包版本清单无效")
		}
		return manifest, nil
	}
	return UpdatePackageManifest{}, errUpdatePackageManifestMissing
}

func sha256File(path string) (string, error) {
	input, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer input.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, io.LimitReader(input, maxUpdateSize+1)); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}

// ReplaceExecutableFile applies a user-selected local release archive. The
// caller must obtain explicit confirmation before passing local files here.
func ReplaceExecutableFile(source string) (string, error) {
	applied, err := ApplyUpdateFile(source)
	if err != nil {
		return "", err
	}
	return applied.Message, nil
}

// ApplyUpdateFile stages or replaces the executable and retains a rollback
// copy. The caller is responsible for persisting this metadata before asking
// the process to restart.
func ApplyUpdateFile(source string) (AppliedUpdate, error) {
	temporary, err := os.MkdirTemp("", "alx-update-")
	if err != nil {
		return AppliedUpdate{}, err
	}
	defer os.RemoveAll(temporary)
	binary, err := releaseBinary(source, temporary)
	if err != nil {
		return AppliedUpdate{}, err
	}
	if err := verifyBinaryPlatform(binary); err != nil {
		return AppliedUpdate{}, err
	}
	return replaceExecutable(binary, source, temporary)
}

func replaceExecutable(binary, _ string, _ string) (AppliedUpdate, error) {
	executable, err := os.Executable()
	if err != nil {
		return AppliedUpdate{}, fmt.Errorf("无法定位当前 alx：%w", err)
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	if runtime.GOOS == "windows" {
		backup := executable + ".previous-" + time.Now().Format("20060102150405") + ".exe"
		if err := copyExecutable(executable, backup); err != nil {
			return AppliedUpdate{}, fmt.Errorf("无法保存旧版本备份：%w", err)
		}
		next := executable + ".new.exe"
		if err := copyExecutable(binary, next); err != nil {
			_ = os.Remove(backup)
			return AppliedUpdate{}, err
		}
		message := "新版已准备就绪；应用退出后会自动替换并重启。"
		return AppliedUpdate{Message: message, Executable: executable, BackupPath: backup}, nil
	}
	next := executable + ".new"
	if err := copyExecutable(binary, next); err != nil {
		return AppliedUpdate{}, err
	}
	backup := executable + ".previous-" + time.Now().Format("20060102150405")
	if err := copyExecutable(executable, backup); err != nil {
		_ = os.Remove(next)
		return AppliedUpdate{}, fmt.Errorf("无法保存旧版本备份：%w", err)
	}
	if err := os.Rename(next, executable); err != nil {
		_ = os.Remove(next)
		return AppliedUpdate{}, fmt.Errorf("无法替换当前 alx：%w", err)
	}
	message := "已更新 alx：" + executable + "。旧版本备份为 " + backup + "；请重新执行命令，后台服务会在下次重启后使用新版本。"
	return AppliedUpdate{Message: message, Executable: executable, BackupPath: backup}, nil
}

// SyncBundledPluginExecutors runs only after the new server has confirmed its
// own version. A failed core replacement must never leave plugins upgraded on
// their own.
func SyncBundledPluginExecutors(archive string) (int, error) {
	if archive == "" {
		return 0, nil
	}
	executable, err := os.Executable()
	if err != nil {
		return 0, err
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	return updateBundledPluginExecutors(archive, filepath.Dir(executable))
}

// WatchUpdate runs in a short-lived helper process after the old server has
// exited. It owns restart verification and rollback, so the HTTP process never
// needs to remain alive while its executable is being replaced.
func WatchUpdate(port string) error {
	transaction, exists, err := ReadUpdateTransaction()
	if err != nil || !exists || transaction.Phase != UpdatePhaseRestarting {
		return err
	}
	if port != "" {
		transaction.Port = port
		_ = SaveUpdateTransaction(transaction)
	}
	child, err := restartForUpdate(transaction)
	if err == nil && waitForUpdateHealth(transaction) {
		_, _, confirmErr := MarkUpdateHealthy(transaction.TargetVersion)
		return confirmErr
	}
	if err == nil {
		err = errors.New("新版未在 40 秒内通过健康检查")
	}
	if child != nil && child.Process != nil {
		_ = child.Process.Kill()
	}
	return rollbackUpdate(transaction, err)
}

func restartForUpdate(transaction UpdateTransaction) (*exec.Cmd, error) {
	switch transaction.Runtime {
	case updateRuntimeSystemd:
		return nil, exec.Command("systemctl", "--user", "restart", "alx.service").Run()
	case updateRuntimeLaunchAgent:
		return nil, restartLaunchAgent()
	case updateRuntimeTask:
		return nil, exec.Command("schtasks", "/Run", "/TN", "ALemonX").Run()
	}
	if transaction.Executable == "" {
		return nil, errors.New("更新事务缺少可执行文件路径")
	}
	port := transaction.Port
	if port == "" {
		port = "17390"
	}
	command := exec.Command(transaction.Executable, "--port", port)
	if err := command.Start(); err != nil {
		return nil, err
	}
	return command, nil
}

func restartLaunchAgent() error {
	path, err := launchAgentPath()
	if err != nil {
		return err
	}
	target := fmt.Sprintf("gui/%d/%s", os.Getuid(), serviceName)
	// rollbackUpdate calls bootout to stop the unhealthy service. A plist still
	// exists after bootout, so kickstart alone would target a service that is no
	// longer loaded. Bootstrap it again before asking launchd to start it.
	if exec.Command("launchctl", "print", target).Run() != nil {
		if output, err := exec.Command("launchctl", "bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), path).CombinedOutput(); err != nil {
			return fmt.Errorf("重新加载 LaunchAgent 失败：%s", strings.TrimSpace(string(output)))
		}
	}
	if output, err := exec.Command("launchctl", "kickstart", "-k", target).CombinedOutput(); err != nil {
		return fmt.Errorf("重启 LaunchAgent 失败：%s", strings.TrimSpace(string(output)))
	}
	return nil
}

func waitForUpdateHealth(transaction UpdateTransaction) bool {
	port := transaction.Port
	if port == "" {
		port = "17390"
	}
	deadline := time.Now().Add(40 * time.Second)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		response, err := client.Get("http://127.0.0.1:" + port + "/healthz")
		if err == nil {
			var health struct {
				Version string `json:"version"`
			}
			decodeErr := json.NewDecoder(response.Body).Decode(&health)
			response.Body.Close()
			if decodeErr == nil && response.StatusCode == http.StatusOK && (transaction.TargetVersion == "" || sameUpdateVersion(transaction.TargetVersion, health.Version)) {
				return true
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func rollbackUpdate(transaction UpdateTransaction, cause error) error {
	transaction.Error = cause.Error()
	if transaction.BackupPath == "" || transaction.Executable == "" {
		transaction.Phase = UpdatePhaseFailed
		_ = SaveUpdateTransaction(transaction)
		return cause
	}
	if runtime.GOOS == "windows" {
		transaction.Phase = UpdatePhaseRolledBack
		_ = SaveUpdateTransaction(transaction)
		return scheduleWindowsRollback(transaction)
	}
	_ = stopManagedUpdateService(transaction.Runtime)
	rollback := transaction.Executable + ".rollback"
	if err := copyExecutable(transaction.BackupPath, rollback); err != nil {
		transaction.Phase, transaction.Error = UpdatePhaseFailed, err.Error()
		_ = SaveUpdateTransaction(transaction)
		return err
	}
	if err := os.Rename(rollback, transaction.Executable); err != nil {
		_ = os.Remove(rollback)
		transaction.Phase, transaction.Error = UpdatePhaseFailed, err.Error()
		_ = SaveUpdateTransaction(transaction)
		return err
	}
	transaction.Phase = UpdatePhaseRolledBack
	_ = SaveUpdateTransaction(transaction)
	_, restartErr := restartForUpdate(transaction)
	return restartErr
}

// RollbackAppliedUpdate restores the on-disk executable before the current
// process has shut down. It is used when the restart helper itself cannot be
// scheduled, leaving the already-running old process available to report the
// failure to the user.
func RollbackAppliedUpdate(transaction UpdateTransaction, cause error) error {
	transaction.Error = cause.Error()
	if transaction.BackupPath == "" || transaction.Executable == "" {
		transaction.Phase = UpdatePhaseFailed
		_ = SaveUpdateTransaction(transaction)
		return cause
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(transaction.Executable + ".new.exe")
		transaction.Phase = UpdatePhaseRolledBack
		_ = SaveUpdateTransaction(transaction)
		return nil
	}
	rollback := transaction.Executable + ".rollback"
	if err := copyExecutable(transaction.BackupPath, rollback); err != nil {
		transaction.Phase, transaction.Error = UpdatePhaseFailed, err.Error()
		_ = SaveUpdateTransaction(transaction)
		return err
	}
	if err := os.Rename(rollback, transaction.Executable); err != nil {
		_ = os.Remove(rollback)
		transaction.Phase, transaction.Error = UpdatePhaseFailed, err.Error()
		_ = SaveUpdateTransaction(transaction)
		return err
	}
	transaction.Phase = UpdatePhaseRolledBack
	_ = SaveUpdateTransaction(transaction)
	return nil
}

func stopManagedUpdateService(mode string) error {
	switch mode {
	case updateRuntimeSystemd:
		return exec.Command("systemctl", "--user", "stop", "alx.service").Run()
	case updateRuntimeLaunchAgent:
		target := fmt.Sprintf("gui/%d/%s", os.Getuid(), serviceName)
		return exec.Command("launchctl", "bootout", target).Run()
	case updateRuntimeTask:
		return exec.Command("schtasks", "/End", "/TN", "ALemonX").Run()
	}
	return nil
}

func scheduleWindowsRollback(transaction UpdateTransaction) error {
	port := transaction.Port
	if port == "" {
		port = "17390"
	}
	arguments := "@('--port'," + powershellQuote(port) + ")"
	script := strings.Join([]string{
		"$target=" + powershellQuote(transaction.Executable),
		"$backup=" + powershellQuote(transaction.BackupPath),
		"Start-Sleep -Milliseconds 500",
		"if(Test-Path -LiteralPath $target){ Remove-Item -LiteralPath $target -Force -ErrorAction SilentlyContinue }",
		"Move-Item -LiteralPath $backup -Destination $target -Force",
		"schtasks.exe /Query /TN 'ALemonX' *> $null",
		"if($LASTEXITCODE -eq 0){ schtasks.exe /Run /TN 'ALemonX' *> $null } else { Start-Process -FilePath $target -ArgumentList " + arguments + " }",
	}, "; ")
	return exec.Command("powershell.exe", "-NoProfile", "-WindowStyle", "Hidden", "-Command", script).Start()
}

// ScheduleRestart starts a short-lived helper which relaunches alx after the
// HTTP response has been written and the current process exits. On Windows it
// also promotes the .new.exe file created while the old executable was locked.
func ScheduleRestart() error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("无法定位当前 alx：%w", err)
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	args := os.Args[1:]
	transaction, exists, transactionErr := ReadUpdateTransaction()
	if transactionErr != nil || !exists {
		return errors.New("更新事务状态不可用")
	}
	port := transaction.Port
	if port == "" {
		port = "17390"
	}
	if runtime.GOOS == "windows" {
		replacement := executable + ".new.exe"
		if _, err := os.Stat(replacement); err == nil {
			return exec.Command("powershell.exe", "-NoProfile", "-WindowStyle", "Hidden", "-Command", windowsRestartScript(executable, replacement, transaction.BackupPath, args, port)).Start()
		}
	}
	// A child process of alx.service is terminated with the service's cgroup
	// during graceful shutdown. Put the watcher in its own transient user unit
	// so it can restart and verify the replacement after alx.service exits.
	if transaction.Runtime == updateRuntimeSystemd {
		unit := "alx-update-watch-" + strconv.FormatInt(time.Now().UnixNano(), 10)
		return exec.Command("systemd-run", "--user", "--collect", "--unit", unit, executable, "update-watch", "--port", port).Run()
	}
	command := []string{"-c", "sleep 0.8; exec \"$@\"", "alx-restart", executable, "update-watch", "--port", port}
	return exec.Command("/bin/sh", command...).Start()
}

func userSystemdServiceInstalled() bool {
	return exec.Command("systemctl", "--user", "is-enabled", "--quiet", "alx.service").Run() == nil
}

func userLaunchAgentInstalled() bool {
	path, err := launchAgentPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func windowsScheduledTaskInstalled() bool {
	return exec.Command("schtasks", "/Query", "/TN", "ALemonX").Run() == nil
}

// windowsRestartScript is deliberately self-contained: PowerShell's $args is
// unreliable when -Command is followed by an empty application argument list.
// The helper retries until the exiting process releases the executable, keeps a
// rollback copy, and records failures next to the staged binary for diagnosis.
func windowsRestartScript(executable, replacement, backup string, args []string, port string) string {
	quotedArgs := make([]string, len(args))
	for index, argument := range args {
		quotedArgs[index] = powershellQuote(argument)
	}
	arguments := "@(" + strings.Join(quotedArgs, ",") + ")"
	return strings.Join([]string{
		"$target=" + powershellQuote(executable),
		"$replacement=" + powershellQuote(replacement),
		"$backup=" + powershellQuote(backup),
		"$failure=" + powershellQuote(replacement+".failure.txt"),
		"$arguments=" + arguments,
		"$watcherArgs=@('update-watch','--port'," + powershellQuote(port) + ")",
		"Start-Sleep -Milliseconds 500",
		"$restarted=$false",
		"for($attempt=0; $attempt -lt 150; $attempt++){ try { if(Test-Path -LiteralPath $target){ Remove-Item -LiteralPath $target -Force -ErrorAction Stop }; Move-Item -LiteralPath $replacement -Destination $target -Force -ErrorAction Stop; Start-Process -FilePath $target -ArgumentList $watcherArgs; $restarted=$true; break } catch { Start-Sleep -Milliseconds 200 } }",
		"if(!$restarted){ [System.IO.File]::WriteAllText($failure, 'Automatic update could not replace or restart alx. The previous executable was restored when possible.') }",
	}, "; ")
}

func updateMessage(message string, pluginUpdates int, pluginErr error) string {
	if pluginUpdates > 0 {
		message += fmt.Sprintf(" 已同步 %d 个已安装插件执行器。", pluginUpdates)
	}
	if pluginErr != nil {
		message += " 插件执行器未同步：" + pluginErr.Error()
	}
	return message
}

func copyExecutable(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
func releaseBinary(source, directory string) (string, error) {
	lower := strings.ToLower(source)
	if strings.HasSuffix(lower, ".zip") {
		return unzipBinary(source, directory)
	}
	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		return untarBinary(source, directory)
	}
	return source, nil
}
func isBinaryName(name string) bool {
	name = strings.ToLower(filepath.Base(name))
	if name == "alx" || name == "alx.exe" {
		return true
	}
	if !strings.HasPrefix(name, "alx-") && !strings.HasPrefix(name, "alemonx") {
		return false
	}
	// A release bundle also contains alx-update.json. Do not mistake metadata
	// (or any other dotted asset) for the executable merely because it starts
	// with the application name.
	extension := filepath.Ext(name)
	return extension == "" || extension == ".exe"
}

func verifyBinaryPlatform(path string) error {
	switch runtime.GOOS {
	case "windows":
		file, err := pe.Open(path)
		if err != nil {
			return errors.New("更新包中的程序不是 Windows 可执行文件")
		}
		defer file.Close()
		if runtime.GOARCH == "amd64" && file.FileHeader.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
			return errors.New("更新包与当前 Windows 架构不匹配")
		}
	case "darwin":
		file, err := macho.Open(path)
		if err != nil {
			return errors.New("更新包中的程序不是 macOS 可执行文件")
		}
		defer file.Close()
		if runtime.GOARCH == "arm64" && file.Cpu != macho.CpuArm64 || runtime.GOARCH == "amd64" && file.Cpu != macho.CpuAmd64 {
			return errors.New("更新包与当前 macOS 架构不匹配")
		}
	case "linux":
		file, err := elf.Open(path)
		if err != nil {
			return errors.New("更新包中的程序不是 Linux 可执行文件")
		}
		defer file.Close()
		if runtime.GOARCH == "arm64" && file.Machine != elf.EM_AARCH64 || runtime.GOARCH == "amd64" && file.Machine != elf.EM_X86_64 {
			return errors.New("更新包与当前 Linux 架构不匹配")
		}
	}
	return nil
}

func unzipBinary(source, directory string) (string, error) {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return "", err
	}
	defer archive.Close()
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() || !isBinaryName(entry.Name) {
			continue
		}
		in, err := entry.Open()
		if err != nil {
			return "", err
		}
		target := filepath.Join(directory, "update-binary")
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err == nil {
			if entry.UncompressedSize64 > uint64(maxUpdateSize) {
				err = errors.New("更新包解压后超过 200 MB")
			} else {
				var copied int64
				copied, err = io.Copy(out, io.LimitReader(in, maxUpdateSize+1))
				if err == nil && copied > maxUpdateSize {
					err = errors.New("更新包解压后超过 200 MB")
				}
			}
			_ = out.Close()
		}
		_ = in.Close()
		if err != nil {
			return "", err
		}
		return target, nil
	}
	return "", errors.New("安装包中未找到 alx 可执行文件")
}
func untarBinary(source, directory string) (string, error) {
	input, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer input.Close()
	gzipReader, err := gzip.NewReader(input)
	if err != nil {
		return "", err
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if header.FileInfo().IsDir() || !isBinaryName(header.Name) {
			continue
		}
		target := filepath.Join(directory, "update-binary")
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return "", err
		}
		if header.Size > maxUpdateSize {
			_ = out.Close()
			return "", errors.New("更新包解压后超过 200 MB")
		}
		copied, copyErr := io.Copy(out, io.LimitReader(reader, maxUpdateSize+1))
		if copyErr == nil && copied > maxUpdateSize {
			copyErr = errors.New("更新包解压后超过 200 MB")
		}
		closeErr := out.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		return target, nil
	}
	return "", errors.New("安装包中未找到 alx 可执行文件")
}

// updateBundledPluginExecutors only updates an already installed plugin. An
// explicit uninstall therefore remains respected: updating alx never silently
// recreates a plugin directory the user removed.
func updateBundledPluginExecutors(source, executableDirectory string) (int, error) {
	if !strings.HasSuffix(strings.ToLower(source), ".zip") {
		return 0, nil
	}
	archive, err := zip.OpenReader(source)
	if err != nil {
		return 0, err
	}
	defer archive.Close()
	updated := 0
	for _, entry := range archive.File {
		parts := strings.Split(filepath.ToSlash(entry.Name), "/")
		if len(parts) != 4 || parts[0] != "plugins" || parts[2] != "dist" || parts[1] == "" || parts[3] == "" || entry.FileInfo().IsDir() {
			continue
		}
		if strings.Contains(parts[1], ".") || strings.Contains(parts[3], "/") {
			continue
		}
		pluginDirectory := filepath.Join(executableDirectory, "plugins", parts[1])
		manifest, manifestErr := os.Lstat(filepath.Join(pluginDirectory, "alx.json"))
		if manifestErr != nil || !manifest.Mode().IsRegular() || manifest.Mode()&os.ModeSymlink != 0 {
			continue
		}
		targetDirectory := filepath.Join(pluginDirectory, "dist")
		if err := os.MkdirAll(targetDirectory, 0755); err != nil {
			return updated, err
		}
		input, err := entry.Open()
		if err != nil {
			return updated, err
		}
		temporary := filepath.Join(targetDirectory, "."+parts[3]+".new")
		output, err := os.OpenFile(temporary, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err == nil {
			_, err = io.Copy(output, io.LimitReader(input, 100<<20))
			closeErr := output.Close()
			if err == nil {
				err = closeErr
			}
		}
		_ = input.Close()
		if err != nil {
			_ = os.Remove(temporary)
			return updated, err
		}
		if err := os.Rename(temporary, filepath.Join(targetDirectory, parts[3])); err != nil {
			_ = os.Remove(temporary)
			return updated, err
		}
		updated++
	}
	return updated, nil
}
