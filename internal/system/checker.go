// Package system contains safe, read-only checks for local prerequisites.
package system

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

const MinimumNodeVersion = "22.22.3"

var checkerRuntimeGOOS = runtime.GOOS

type Check struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Detail     string `json:"detail"`
	Suggestion string `json:"suggestion"`
	Optional   bool   `json:"optional,omitempty"`
}
type Report struct {
	GoalID    string  `json:"goalId"`
	Ready     bool    `json:"ready"`
	Platform  string  `json:"platform"`
	Checks    []Check `json:"checks"`
	CheckedAt string  `json:"checkedAt"`
}
type Checker struct {
	timeout            time.Duration
	resolveBrowser     func() (string, string)
	missingBrowserDeps func() []string
	canLaunchHeadless  func(string) bool
}

func NewChecker() *Checker { return &Checker{timeout: 5 * time.Second} }

func (c *Checker) CheckGoal(goalID, variant string) Report {
	checks := []Check{
		c.command("node", "NodeJS", "--version", "请安装 Node.js 22 稳定版后重新检查。"),
		c.command("git", "Git", "--version", "请安装 Git 后重新检查。"),
		c.fonts(),
		c.browser(),
		c.browserDependencies(),
		c.commonDependencies(),
	}
	nodeRequired, gitRequired := false, false
	switch goalID {
	case "install", "develop":
		nodeRequired, gitRequired = true, true
	case "web":
		if variant == "clean" {
			nodeRequired, gitRequired = true, true
		} else {
			checks = append(checks, c.command("docker", "Docker", "--version", "请安装并启动 Docker Desktop 后重新检查。"))
		}
	case "mobile":
		// Mobile installation is completed on the phone; the desktop app only checks host support.
	case "build":
		if variant == "git" {
			// Git 发布实际只需要 Node.js 与 Git。Yarn/PNPM 会在执行时
			// 通过 npx 临时运行，jq 也没有参与当前发布链路，不能把它们
			// 误报为新用户必须全局安装的环境。
			nodeRequired, gitRequired = true, true
		} else {
			nodeRequired = true
			checks = append(checks, c.command("npm", "npm", "--version", "请随 Node.js 一并安装 npm 后重新检查。"))
		}
	}
	checks[0].Optional = !nodeRequired
	checks[1].Optional = !gitRequired
	ready := platformSupported() && checksAreUsable(checks)
	return Report{goalID, ready, runtime.GOOS + "/" + runtime.GOARCH, checks, time.Now().Format(time.RFC3339)}
}

// fonts is a fully optional check: CJK/Emoji fonts only affect text rendering
// in screenshots or PDFs, never browser startup, so they must not influence
// the browser environment result.
func (c *Checker) fonts() Check {
	check := Check{ID: "fonts", Name: "系统字体", Status: "ready", Detail: "系统已内置 CJK/Emoji 字体", Optional: true}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		return check
	}
	if hasCJKFonts() {
		return check
	}
	check.Status = "missing"
	check.Detail = "未检测到中文/Emoji 字体"
	check.Suggestion = "可选：安装 Noto CJK/Emoji 字体，避免机器人图片消息、无头浏览器截图或导出 PDF 时缺字。"
	return check
}

func (c *Checker) commonDependencies() Check {
	if runtime.GOOS != "linux" {
		return Check{ID: "common-dependencies", Name: "常用环境依赖", Status: "ready", Detail: "当前系统已提供常用命令", Optional: true}
	}
	missing := make([]string, 0, 3)
	for _, command := range []string{"curl", "tar", "unzip"} {
		if _, err := ResolveCommand(command); err != nil {
			missing = append(missing, command)
		}
	}
	if len(missing) == 0 {
		return Check{ID: "common-dependencies", Name: "常用环境依赖", Status: "ready", Detail: "curl、tar、unzip 已就绪", Optional: true}
	}
	return Check{
		ID:         "common-dependencies",
		Name:       "常用环境依赖",
		Status:     "missing",
		Detail:     "缺少：" + strings.Join(missing, "、"),
		Suggestion: "可选：安装常用环境依赖，便于下载、解压和管理开发工具。",
		Optional:   true,
	}
}

func hasCJKFonts() bool {
	if path, err := exec.LookPath("fc-list"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if exec.CommandContext(ctx, path, ":lang=zh").Run() == nil {
			return true
		}
	}
	for _, pattern := range []string{
		"/usr/share/fonts/noto-cjk/*",
		"/usr/share/fonts/opentype/noto/*",
		"/usr/share/fonts/google-noto*/*",
		"/usr/share/fonts/truetype/wqy*/*",
	} {
		if matches, _ := filepath.Glob(pattern); len(matches) > 0 {
			return true
		}
	}
	return false
}

// checksAreUsable keeps a detected but old Node.js version visible as an
// exception without treating it as a hard stop. The user can continue using
// the workbench and choose when to upgrade; a missing or broken tool remains
// a blocking prerequisite.
func checksAreUsable(checks []Check) bool {
	for _, check := range checks {
		if !check.Optional && check.Status != "ready" && check.Status != "outdated" {
			return false
		}
	}
	return true
}

// platformSupported gates unsupported operating systems without exposing a
// redundant “当前系统” row: the user already knows their own OS. The report
// still carries the platform string for internal install-plan decisions.
func platformSupported() bool {
	switch runtime.GOOS {
	case "darwin", "windows", "linux", "freebsd":
		return true
	}
	return false
}

func (c *Checker) command(id, name, argument, suggestion string) Check {
	previousPath, previousErr := exec.LookPath(id)
	RefreshCommandEnvironment(id)
	path, err := ResolveCommand(id)
	if err != nil {
		return Check{ID: id, Name: name, Status: "missing", Detail: "未检测到", Suggestion: suggestion}
	}
	repaired := previousErr != nil || filepath.Clean(previousPath) != filepath.Clean(path)
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, argument).CombinedOutput()
	if ctx.Err() != nil {
		return Check{ID: id, Name: name, Status: "warning", Detail: "检测超时", Suggestion: "请确认程序可以正常启动后重试。"}
	}
	if err != nil {
		return Check{ID: id, Name: name, Status: "warning", Detail: "已找到，但无法正常运行", Suggestion: "请重新安装或修复 " + name + " 后重试。"}
	}
	rawVersion := strings.TrimSpace(strings.Split(normalizeCommandOutput(output), "\n")[0])
	version := rawVersion
	if repaired {
		if version != "" {
			version += " · "
		}
		version += "已自动修复当前服务 PATH"
	}
	if id == "node" && !nodeVersionAtLeast(rawVersion, MinimumNodeVersion) {
		return Check{
			ID:         id,
			Name:       name,
			Status:     "outdated",
			Detail:     version + " · 低于最低要求 v" + MinimumNodeVersion,
			Suggestion: "当前 Node.js 版本低于 v" + MinimumNodeVersion + "。建议升级；不限制继续使用，但部分项目或依赖可能无法正常运行。",
		}
	}
	return Check{ID: id, Name: name, Status: "ready", Detail: version}
}

// browser is optional because a project can download a compatible browser as
// needed. Browser binaries and their Linux runtime environment are intentionally
// reported separately so each can be installed independently.
func (c *Checker) browser() Check {
	path, name := c.browserCommand()
	if path == "" {
		return Check{
			ID:         "browser",
			Name:       "浏览器",
			Status:     "missing",
			Detail:     "未检测到 Chrome、Chromium 或 Edge",
			Suggestion: "可选：需要浏览器自动化时，请安装 Chrome、Chromium 或 Edge。",
			Optional:   true,
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if ctx.Err() != nil || err != nil {
		return Check{
			ID:         "browser",
			Name:       "浏览器",
			Status:     "warning",
			Detail:     "已找到 " + name + "，但无法正常启动",
			Suggestion: "可选：请修复或重新安装浏览器后重试。",
			Optional:   true,
		}
	}
	version := strings.TrimSpace(strings.Split(normalizeCommandOutput(output), "\n")[0])
	return Check{ID: "browser", Name: "浏览器", Status: "ready", Detail: version, Optional: true}
}

func (c *Checker) browserDependencies() Check {
	path, _ := c.browserCommand()
	if path == "" {
		return Check{ID: "browser-dependencies", Name: "浏览器自动化运行环境", Status: "ready", Detail: "未检测到浏览器，暂不检查", Optional: true}
	}
	if !c.headlessBrowserCanLaunch(path) {
		missing := []string(nil)
		if checkerRuntimeGOOS == "linux" {
			missing = c.browserMissingDependencies()
		}
		detail := "无头浏览器启动失败"
		if len(missing) > 0 {
			detail += "；可能缺少运行库：" + strings.Join(missing, "、")
		}
		return Check{
			ID:         "browser-dependencies",
			Name:       "浏览器自动化运行环境",
			Status:     "warning",
			Detail:     detail,
			Suggestion: "可选：请检查浏览器权限与运行环境；Linux 可安装浏览器自动化运行环境后重试。",
			Optional:   true,
		}
	}
	detail := "无头浏览器可用"
	if checkerRuntimeGOOS == "linux" && browserFontDependencyMissing(c.browserMissingDependencies()) {
		detail += "；缺少 fonts-liberation，截图与网页文字可能使用替代字体"
	}
	return Check{ID: "browser-dependencies", Name: "浏览器自动化运行环境", Status: "ready", Detail: detail, Optional: true}
}

func (c *Checker) browserCommand() (string, string) {
	if c.resolveBrowser != nil {
		return c.resolveBrowser()
	}
	return resolveBrowserCommand()
}

func (c *Checker) browserMissingDependencies() []string {
	if c.missingBrowserDeps != nil {
		return c.missingBrowserDeps()
	}
	return missingBrowserDependencies()
}

func (c *Checker) headlessBrowserCanLaunch(path string) bool {
	if c.canLaunchHeadless != nil {
		return c.canLaunchHeadless(path)
	}
	return browserCanLaunchHeadless(path)
}

func browserFontDependencyMissing(missing []string) bool {
	for _, dependency := range missing {
		if strings.Contains(dependency, "fonts-liberation") {
			return true
		}
	}
	return false
}

// normalizeCommandOutput keeps command diagnostics readable when a Windows
// executable writes the active system code page instead of UTF-8. Go turns
// invalid UTF-8 bytes into replacement characters if they are converted to a
// string too early, so all subprocess output must pass through this function
// first.
func normalizeCommandOutput(output []byte) string {
	if len(output) == 0 {
		return ""
	}
	if bytes.HasPrefix(output, []byte{0xff, 0xfe}) || bytes.HasPrefix(output, []byte{0xfe, 0xff}) {
		littleEndian := output[0] == 0xff
		data := output[2:]
		if len(data)%2 == 1 {
			data = data[:len(data)-1]
		}
		units := make([]uint16, len(data)/2)
		for index := range units {
			offset := index * 2
			if littleEndian {
				units[index] = binary.LittleEndian.Uint16(data[offset : offset+2])
			} else {
				units[index] = binary.BigEndian.Uint16(data[offset : offset+2])
			}
		}
		return string(utf16.Decode(units))
	}
	if utf8.Valid(output) {
		return string(output)
	}
	decoded, _, err := transform.Bytes(simplifiedchinese.GB18030.NewDecoder(), output)
	if err == nil && utf8.Valid(decoded) {
		return string(decoded)
	}
	return string(output)
}

func resolveBrowserCommand() (string, string) {
	for _, variable := range []string{"PUPPETEER_EXECUTABLE_PATH", "CHROME_BIN", "BROWSER_BIN"} {
		if candidate := strings.TrimSpace(os.Getenv(variable)); candidate != "" {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, filepath.Base(candidate)
			}
		}
	}
	for _, candidate := range []struct {
		command string
		name    string
	}{
		{"google-chrome", "Google Chrome"},
		{"google-chrome-stable", "Google Chrome"},
		{"chromium", "Chromium"},
		{"chromium-browser", "Chromium"},
		{"msedge", "Microsoft Edge"},
		{"microsoft-edge", "Microsoft Edge"},
	} {
		if path, err := ResolveCommand(candidate.command); err == nil {
			return path, candidate.name
		}
	}
	for _, path := range browserApplicationPaths() {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, filepath.Base(path)
		}
	}
	return "", ""
}

func browserApplicationPaths() []string {
	switch runtime.GOOS {
	case "darwin":
		paths := []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		}
		if home := os.Getenv("HOME"); home != "" {
			paths = append(paths,
				filepath.Join(home, "Applications/Google Chrome.app/Contents/MacOS/Google Chrome"),
				filepath.Join(home, "Applications/Chromium.app/Contents/MacOS/Chromium"),
				filepath.Join(home, "Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"),
			)
		}
		return paths
	case "linux":
		return []string{
			"/snap/bin/chromium",
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/microsoft-edge",
		}
	case "windows":
		directories := []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramW6432"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA"), os.Getenv("LocalAppData")}
		paths := make([]string, 0, len(directories)*3)
		for _, directory := range directories {
			if directory == "" {
				continue
			}
			paths = append(paths,
				filepath.Join(directory, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(directory, "Chromium", "Application", "chrome.exe"),
				filepath.Join(directory, "Microsoft", "Edge", "Application", "msedge.exe"),
			)
		}
		return paths
	default:
		return nil
	}
}

func missingBrowserDependencies() []string {
	manager := browserDependencyPackageManager()
	if manager == "" {
		return nil
	}
	groups := browserDependencyPackageCandidates(manager)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	missing := make([]string, 0, len(groups))
	for _, group := range groups {
		installed := false
		for _, pkg := range group {
			if packageInstalled(ctx, manager, pkg) {
				installed = true
				break
			}
		}
		if !installed {
			missing = append(missing, strings.Join(group, "/"))
		}
		if ctx.Err() != nil {
			return nil
		}
	}
	return missing
}

func browserDependencyPackageManager() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	for _, manager := range []string{"apt-get", "dnf", "yum", "apk", "pacman"} {
		if _, err := ResolveCommand(manager); err == nil {
			return manager
		}
	}
	return ""
}

func packageInstalled(ctx context.Context, manager, packageName string) bool {
	var command string
	var args []string
	switch manager {
	case "apt-get":
		command = "dpkg-query"
		args = []string{"-W", "-f=${Status}", packageName}
	case "dnf", "yum":
		command = "rpm"
		args = []string{"-q", packageName}
	case "apk":
		command = "apk"
		args = []string{"info", "-e", packageName}
	case "pacman":
		command = "pacman"
		args = []string{"-Q", packageName}
	default:
		return false
	}
	path, err := ResolveCommand(command)
	if err != nil {
		return false
	}
	probe := exec.CommandContext(ctx, path, args...)
	if manager == "apt-get" {
		output, err := probe.Output()
		return err == nil && strings.Contains(string(output), "install ok installed")
	}
	return probe.Run() == nil
}

// browserCanLaunchHeadless verifies the behavior browser automation needs,
// instead of treating a successful --version invocation as sufficient.
func browserCanLaunchHeadless(path string) bool {
	profile, err := os.MkdirTemp("", "alx-browser-check-")
	if err != nil {
		return false
	}
	defer os.RemoveAll(profile)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, headless := range []string{"--headless=new", "--headless"} {
		args := []string{
			headless,
			"--disable-gpu",
			"--disable-dev-shm-usage",
			"--no-first-run",
			"--no-default-browser-check",
			"--user-data-dir=" + profile,
			"--dump-dom",
			"about:blank",
		}
		if runtime.GOOS == "linux" && os.Geteuid() == 0 {
			args = append(args, "--no-sandbox")
		}
		command := exec.CommandContext(ctx, path, args...)
		command.Stdout = io.Discard
		command.Stderr = io.Discard
		if err := command.Run(); err == nil && ctx.Err() == nil {
			return true
		}
		if ctx.Err() != nil {
			return false
		}
	}
	return false
}

func nodeVersionAtLeast(version, minimum string) bool {
	actual, ok := parseNodeVersion(version)
	if !ok {
		return false
	}
	required, ok := parseNodeVersion(minimum)
	if !ok {
		return false
	}
	for index := range actual {
		if actual[index] != required[index] {
			return actual[index] > required[index]
		}
	}
	return true
}

func parseNodeVersion(value string) ([3]int, bool) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var parsed [3]int
	for index, part := range parts {
		if part == "" || strings.ContainsAny(part, "+-") {
			return [3]int{}, false
		}
		item, err := strconv.Atoi(part)
		if err != nil || item < 0 {
			return [3]int{}, false
		}
		parsed[index] = item
	}
	return parsed, true
}

// ResolveCommand resolves a prerequisite without relying solely on the PATH
// captured when AlemonX started. An NVM-selected Node runtime deliberately
// wins over an older system Node so installs immediately use the LTS chosen
// through the workbench.
func ResolveCommand(name string) (string, error) {
	if path := nvmNodeCommand(name); path != "" {
		return path, nil
	}
	if path := ManagedNodeCommand(name); path != "" {
		return path, nil
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	if runtime.GOOS == "windows" {
		for _, directory := range windowsCommandDirectories(name) {
			path := filepath.Join(directory, name+".exe")
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path, nil
			}
		}
		return "", exec.ErrNotFound
	}
	for _, directory := range unixCommandDirectories(name) {
		path := filepath.Join(directory, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return path, nil
		}
	}
	return "", exec.ErrNotFound
}

// RefreshCommandEnvironment updates AlemonX's own process environment after
// an approved install. It does not alter the machine-wide PATH; it makes the
// current service and every child process immediately see the new tool.
func RefreshCommandEnvironment(names ...string) []string {
	directories := []string{}
	seen := map[string]bool{}
	for _, name := range names {
		path, err := ResolveCommand(name)
		if err != nil {
			continue
		}
		directory := filepath.Dir(path)
		key := directory
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if !seen[key] {
			seen[key] = true
			directories = append(directories, directory)
		}
	}
	if len(directories) == 0 {
		return nil
	}
	entries := filepath.SplitList(os.Getenv("PATH"))
	merged := make([]string, 0, len(directories)+len(entries))
	seen = map[string]bool{}
	for _, directory := range append(directories, entries...) {
		key := directory
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if directory != "" && !seen[key] {
			seen[key] = true
			merged = append(merged, directory)
		}
	}
	_ = os.Setenv("PATH", strings.Join(merged, string(os.PathListSeparator)))
	return directories
}

func unixCommandDirectories(name string) []string {
	directories := []string{"/usr/local/bin", "/usr/bin", "/bin"}
	if runtime.GOOS == "darwin" {
		directories = append([]string{"/opt/homebrew/bin", "/Applications/Docker.app/Contents/Resources/bin"}, directories...)
	}
	return directories
}

func windowsCommandDirectories(name string) []string {
	directories := windowsRegistryPathDirectories()
	programFiles := []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramW6432"), os.Getenv("ProgramFiles(x86)")}
	switch name {
	case "node", "npm", "npx":
		for _, root := range programFiles {
			if root != "" {
				directories = append(directories, filepath.Join(root, "nodejs"))
			}
		}
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			directories = append(directories, filepath.Join(localAppData, "Programs", "nodejs"))
		}
		if appData := os.Getenv("APPDATA"); appData != "" {
			directories = append(directories, filepath.Join(appData, "nvm"))
		}
	case "nvm":
		for _, root := range programFiles {
			if root != "" {
				directories = append(directories, filepath.Join(root, "nvm"))
			}
		}
		if appData := os.Getenv("APPDATA"); appData != "" {
			directories = append(directories, filepath.Join(appData, "nvm"))
		}
	case "git":
		for _, root := range programFiles {
			if root != "" {
				directories = append(directories, filepath.Join(root, "Git", "cmd"), filepath.Join(root, "Git", "bin"))
			}
		}
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			directories = append(directories, filepath.Join(localAppData, "Programs", "Git", "cmd"), filepath.Join(localAppData, "Programs", "Git", "bin"))
		}
	case "docker":
		for _, root := range programFiles {
			if root != "" {
				directories = append(directories, filepath.Join(root, "Docker", "Docker", "resources", "bin"))
			}
		}
	}

	seen := make(map[string]bool, len(directories))
	result := make([]string, 0, len(directories))
	for _, directory := range directories {
		directory = strings.Trim(strings.TrimSpace(directory), `"`)
		if directory != "" && !seen[strings.ToLower(directory)] {
			seen[strings.ToLower(directory)] = true
			result = append(result, directory)
		}
	}
	return result
}

// windowsRegistryPathDirectories reads both persistent PATH values. It is kept
// best-effort: the standard install directories above still cover installations
// where access to one of the registry hives is unavailable.
func windowsRegistryPathDirectories() []string {
	if runtime.GOOS != "windows" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	directories := []string{}
	for _, key := range []string{
		`HKCU\Environment`,
		`HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment`,
	} {
		output, err := exec.CommandContext(ctx, "reg.exe", "query", key, "/v", "Path").Output()
		if err != nil {
			continue
		}
		if value := windowsRegistryPathValue(string(output)); value != "" {
			directories = append(directories, filepath.SplitList(expandWindowsEnvironment(value))...)
		}
	}
	return directories
}

func windowsRegistryPathValue(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if typeIndex := strings.Index(line, "REG_"); typeIndex >= 0 {
			if valueIndex := strings.IndexAny(line[typeIndex:], " \t"); valueIndex >= 0 {
				return strings.TrimSpace(line[typeIndex+valueIndex:])
			}
		}
	}
	return ""
}

func expandWindowsEnvironment(value string) string {
	return os.Expand(expandWindowsPercentVariables(value), func(key string) string { return os.Getenv(key) })
}

func expandWindowsPercentVariables(value string) string {
	for start := strings.IndexByte(value, '%'); start >= 0; {
		endOffset := strings.IndexByte(value[start+1:], '%')
		if endOffset < 0 {
			break
		}
		end := start + endOffset + 1
		key := value[start+1 : end]
		value = value[:start] + os.Getenv(key) + value[end+1:]
		start = strings.IndexByte(value, '%')
	}
	return value
}
