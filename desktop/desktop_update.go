package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
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
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

var desktopVersion = "dev"

const desktopReleaseAPI = "https://api.github.com/repos/lemonade-lab/alemonx/releases/latest"
const desktopUpdateLimit = 1 << 30

var desktopReleaseTag = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type desktopUpdateState struct {
	Current string `json:"current"`
	Latest  string `json:"latest,omitempty"`
	Asset   string `json:"asset,omitempty"`
	Ready   bool   `json:"ready"`
	Error   string `json:"error,omitempty"`
}

type desktopUpdater struct {
	mu          sync.Mutex
	state       desktopUpdateState
	downloadURL string
	checksumURL string
	stage       string
	installing  bool
}

func desktopAssetName(platform, arch string) string {
	switch {
	case platform == "darwin" && arch == "arm64":
		return "alemonx-desktop-macos_arm64.zip"
	case platform == "windows" && arch == "amd64":
		return "alemonx-desktop-windows_amd64.zip"
	case platform == "linux" && (arch == "amd64" || arch == "arm64"):
		return "alemonx-desktop-linux_" + arch + ".tar.gz"
	default:
		return ""
	}
}

func newerDesktopRelease(latest, current string) bool {
	a := desktopReleaseTag.FindStringSubmatch(latest)
	b := desktopReleaseTag.FindStringSubmatch(current)
	if a == nil || b == nil {
		return false
	}
	for i := 1; i < len(a); i++ {
		if len(a[i]) != len(b[i]) {
			return len(a[i]) > len(b[i])
		}
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func desktopUpdateGET(client *http.Client, address string) (io.ReadCloser, error) {
	response, err := client.Get(address)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("GitHub 返回 HTTP %d", response.StatusCode)
	}
	return response.Body, nil
}

func (u *desktopUpdater) check() error {
	body, err := desktopUpdateGET(&http.Client{Timeout: 20 * time.Second}, desktopReleaseAPI)
	if err != nil {
		return err
	}
	defer body.Close()
	var release struct {
		Tag    string `json:"tag_name"`
		Assets []struct{ Name string }
	}
	if err := json.NewDecoder(io.LimitReader(body, 2<<20)).Decode(&release); err != nil {
		return err
	}
	if !desktopReleaseTag.MatchString(release.Tag) {
		return errors.New("发布版本号无效")
	}
	state := desktopUpdateState{Current: desktopVersion, Latest: release.Tag}
	if u.stage != "" && release.Tag == u.state.Latest {
		state.Ready = true
	}
	u.downloadURL, u.checksumURL = "", ""
	if newerDesktopRelease(release.Tag, desktopVersion) {
		base := "https://github.com/lemonade-lab/alemonx/releases/download/" + release.Tag + "/"
		for _, asset := range release.Assets {
			switch asset.Name {
			case desktopAssetName(runtime.GOOS, runtime.GOARCH):
				if asset.Name != "" {
					state.Asset = asset.Name
					u.downloadURL = base + asset.Name
				}
			case "alemonx-desktop-SHA256SUMS":
				u.checksumURL = base + asset.Name
			}
		}
	}
	if u.stage != "" && !state.Ready {
		_ = os.RemoveAll(u.stage)
		u.stage = ""
	}
	u.state = state
	if desktopReleaseTag.FindStringSubmatch(desktopVersion) == nil {
		return errors.New("当前为开发构建，请从发布页安装正式版")
	}
	if newerDesktopRelease(release.Tag, desktopVersion) && (u.downloadURL == "" || u.checksumURL == "") {
		u.state.Asset = ""
		return errors.New("此版本缺少当前平台的桌面包或校验文件")
	}
	return nil
}

func desktopArchiveChecksum(body io.Reader, asset string) (string, error) {
	data, err := io.ReadAll(io.LimitReader(body, 64<<10))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != asset {
			continue
		}
		decoded, err := hex.DecodeString(fields[0])
		if err == nil && len(decoded) == sha256.Size {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", errors.New("找不到桌面包的 SHA-256 校验值")
}

func (u *desktopUpdater) download() error {
	if u.state.Ready {
		return nil
	}
	if u.downloadURL == "" || u.checksumURL == "" {
		return errors.New("请先检查桌面更新")
	}
	body, err := desktopUpdateGET(&http.Client{Timeout: 20 * time.Second}, u.checksumURL)
	if err != nil {
		return err
	}
	expected, err := desktopArchiveChecksum(body, u.state.Asset)
	body.Close()
	if err != nil {
		return err
	}
	_, destination, err := desktopInstallPaths()
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp(destination, ".alemonx-update-")
	if err != nil {
		return fmt.Errorf("无法写入应用安装目录：%w", err)
	}
	defer func() {
		if u.stage != stage {
			_ = os.RemoveAll(stage)
		}
	}()
	archive := filepath.Join(stage, "download")
	output, err := os.Create(archive)
	if err != nil {
		return err
	}
	body, err = desktopUpdateGET(&http.Client{Timeout: 20 * time.Minute}, u.downloadURL)
	if err != nil {
		output.Close()
		return err
	}
	sum := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(output, sum), io.LimitReader(body, desktopUpdateLimit+1))
	body.Close()
	if err := errors.Join(copyErr, output.Close()); err != nil {
		return err
	}
	if size > desktopUpdateLimit || hex.EncodeToString(sum.Sum(nil)) != expected {
		return errors.New("桌面更新包完整性校验失败")
	}
	if err := extractDesktopUpdate(archive, stage); err != nil {
		return err
	}
	if runtime.GOOS == "darwin" {
		app := filepath.Join(stage, "ALemonX Desktop.app")
		for _, name := range []string{"ALemonX Desktop", "alx"} {
			info, err := os.Stat(filepath.Join(app, "Contents", "MacOS", name))
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0100 == 0 {
				return errors.New("macOS 更新包缺少可执行文件")
			}
		}
		if err := exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", app).Run(); err != nil {
			return fmt.Errorf("macOS 应用签名验证失败：%w", err)
		}
	}
	_ = os.Remove(archive)
	u.stage, u.state.Ready = stage, true
	return nil
}

func desktopInstallPaths() (string, string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", "", err
	}
	dir := filepath.Dir(executable)
	if runtime.GOOS == "darwin" {
		bundle := filepath.Dir(filepath.Dir(dir))
		if filepath.Base(dir) != "MacOS" || filepath.Base(filepath.Dir(dir)) != "Contents" || !strings.HasSuffix(bundle, ".app") {
			return "", "", errors.New("请从 macOS 应用包运行桌面版")
		}
		dir = filepath.Dir(bundle)
		if strings.Contains(dir, "/AppTranslocation/") {
			return "", "", errors.New("请先把应用移动到「应用程序」目录再更新")
		}
	}
	return executable, dir, nil
}

func extractDesktopUpdate(archive, stage string) error {
	var remaining int64 = desktopUpdateLimit
	save := func(name string, size int64, mode os.FileMode, source io.Reader) error {
		if size < 0 || size > remaining || !mode.IsRegular() || !filepath.IsLocal(name) {
			return errors.New("桌面更新包包含无效文件")
		}
		remaining -= size
		target := filepath.Join(stage, name)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644|mode.Perm()&0111)
		if err != nil {
			return err
		}
		n, err := io.Copy(file, io.LimitReader(source, size+1))
		if n != size {
			err = errors.Join(err, io.ErrUnexpectedEOF)
		}
		return errors.Join(err, file.Close())
	}
	if runtime.GOOS == "linux" {
		file, err := os.Open(archive)
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
		prefix := "alemonx-desktop-linux-" + runtime.GOARCH + "/"
		for {
			entry, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			name, ok := strings.CutPrefix(entry.Name, prefix)
			if !ok || (name != "alx" && name != "alemonx-desktop" && name != "README.txt") {
				continue
			}
			if err := save(name, entry.Size, entry.FileInfo().Mode(), reader); err != nil {
				return err
			}
		}
		return nil
	}
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if runtime.GOOS == "darwin" {
			if !strings.HasPrefix(entry.Name, "ALemonX Desktop.app/") {
				continue
			}
			if !strings.HasPrefix(filepath.ToSlash(filepath.Clean(entry.Name)), "ALemonX Desktop.app/") {
				return errors.New("应用包路径无效")
			}
		} else if entry.Name != "alx.exe" && entry.Name != "ALemonX Desktop.exe" {
			continue
		}
		source, err := entry.Open()
		if err != nil {
			return err
		}
		err = save(entry.Name, int64(entry.UncompressedSize64), entry.Mode(), source)
		source.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

type desktopInstallPlan struct {
	Executable   string
	Stage        string
	Files        []string
	Destinations []string
	Arguments    []string
	Result       string
	ParentPID    int
}

const desktopInstallFlag = "--alx-install-desktop-update"

func (u *desktopUpdater) prepareInstall() (*exec.Cmd, error) {
	if !u.state.Ready || u.stage == "" {
		return nil, errors.New("请先下载桌面更新包")
	}
	executable, dir, err := desktopInstallPaths()
	if err != nil {
		return nil, err
	}
	plan := desktopInstallPlan{Executable: executable, Stage: u.stage, Arguments: os.Args[1:], ParentPID: os.Getpid(), Result: desktopUpdateResultPath()}
	switch runtime.GOOS {
	case "darwin":
		plan.Files = []string{"ALemonX Desktop.app"}
		plan.Destinations = []string{filepath.Dir(filepath.Dir(filepath.Dir(executable)))}
	case "windows":
		plan.Files = []string{"ALemonX Desktop.exe", "alx.exe"}
		plan.Destinations = []string{executable, filepath.Join(dir, "alx.exe")}
	case "linux":
		plan.Files = []string{"alemonx-desktop", "alx"}
		plan.Destinations = []string{executable, filepath.Join(dir, "alx")}
	}
	for _, name := range plan.Files {
		info, err := os.Stat(filepath.Join(u.stage, name))
		if err != nil || (runtime.GOOS != "darwin" && (!info.Mode().IsRegular() || (runtime.GOOS == "linux" && info.Mode().Perm()&0100 == 0))) {
			return nil, errors.New("桌面更新包缺少可执行文件")
		}
	}
	helper, err := os.MkdirTemp("", "alemonx-install-")
	if err != nil {
		return nil, err
	}
	copied := false
	defer func() {
		if !copied {
			_ = os.RemoveAll(helper)
		}
	}()
	target := filepath.Join(helper, filepath.Base(executable))
	source, err := os.Open(executable)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(out, source)
	if err = errors.Join(err, out.Close()); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(helper, "plan.json")
	if err := os.WriteFile(path, payload, 0600); err != nil {
		return nil, err
	}
	copied = true
	return exec.Command(target, desktopInstallFlag, path), nil
}

func desktopUpdateResultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "AlemonX Desktop", "update-result")
}

func applyDesktopInstall(plan desktopInstallPlan) (err error) {
	backups := make([]string, len(plan.Files))
	installed := make([]bool, len(plan.Files))
	defer func() {
		if err == nil {
			return
		}
		for i := len(plan.Files) - 1; i >= 0; i-- {
			if installed[i] {
				err = errors.Join(err, os.RemoveAll(plan.Destinations[i]))
			}
			if backups[i] != "" {
				err = errors.Join(err, os.Rename(backups[i], plan.Destinations[i]))
			}
		}
	}()
	for i, name := range plan.Files {
		backup := filepath.Join(plan.Stage, fmt.Sprintf("previous-%d", i))
		if err := os.Rename(plan.Destinations[i], backup); err != nil {
			return err
		}
		backups[i] = backup
		if err := os.Rename(filepath.Join(plan.Stage, name), plan.Destinations[i]); err != nil {
			return err
		}
		installed[i] = true
	}
	return startUpdatedDesktop(plan)
}

func startUpdatedDesktop(plan desktopInstallPlan) error {
	cmd := exec.Command(plan.Executable, plan.Arguments...)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	return nil
}

func runDesktopInstaller(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var plan desktopInstallPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return err
	}
	if len(plan.Files) == 0 || len(plan.Files) != len(plan.Destinations) {
		return errors.New("无效的桌面安装计划")
	}
	_ = os.Remove(path)
	if runtime.GOOS != "windows" {
		defer os.RemoveAll(filepath.Dir(path))
	}
	if err := waitForDesktopExit(plan.ParentPID); err != nil {
		_ = os.WriteFile(plan.Result, []byte(err.Error()), 0600)
		return err
	}
	if err := applyDesktopInstall(plan); err != nil {
		_ = os.WriteFile(plan.Result, []byte("安装更新失败："+err.Error()), 0600)
		return errors.Join(err, startUpdatedDesktop(plan))
	}
	return os.RemoveAll(plan.Stage)
}

func (u *desktopUpdater) handle(action string, install func(*exec.Cmd) bool) desktopUpdateState {
	if action == "status" {
		u.mu.Lock()
	} else if !u.mu.TryLock() {
		return desktopUpdateState{Current: desktopVersion, Error: "已有桌面更新操作正在进行"}
	}
	defer u.mu.Unlock()
	var err error
	if u.installing {
		return desktopUpdateState{Current: desktopVersion, Error: "桌面更新即将安装"}
	}
	switch action {
	case "status":
		if data, e := os.ReadFile(desktopUpdateResultPath()); e == nil {
			_ = os.Remove(desktopUpdateResultPath())
			err = errors.New(string(data))
		}
	case "check":
		err = u.check()
	case "download":
		err = u.download()
	case "install":
		var cmd *exec.Cmd
		cmd, err = u.prepareInstall()
		if err == nil {
			if install(cmd) {
				u.installing = true
			} else {
				_ = os.RemoveAll(filepath.Dir(cmd.Path))
				err = errors.New("应用正在退出，请稍后重试")
			}
		}
	}
	state := u.state
	state.Current = desktopVersion
	if err != nil {
		state.Error = err.Error()
	}
	return state
}

func (u *desktopUpdater) discardStagedUpdate() {
	if !u.mu.TryLock() {
		return
	}
	defer u.mu.Unlock()
	if !u.installing && u.stage != "" {
		_ = os.RemoveAll(u.stage)
	}
}
