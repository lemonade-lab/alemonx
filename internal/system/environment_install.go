package system

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// EnvironmentInstallPlan is a reviewed, fixed package-manager operation. It
// never contains browser input other than the small prerequisite identifier.
type EnvironmentInstallPlan struct {
	CheckID        string
	Name           string
	PackageManager string
	Packages       []string
	// BrowserPackage is the system browser binary selected for RPM hosts when
	// the configured repositories provide one. Empty means the plan installs
	// only the fixed runtime libraries and fonts and Puppeteer uses the
	// browser it downloads per project.
	BrowserPackage string
}

// EnvironmentInstallPlanForHost returns the package-manager action supported
// by the workbench for a missing prerequisite. Package names are host policy,
// not plugin or browser data.
func EnvironmentInstallPlanForHost(checkID string) (EnvironmentInstallPlan, error) {
	checkID = strings.TrimSpace(checkID)
	manager, err := hostPackageManager()
	if err != nil {
		return EnvironmentInstallPlan{}, err
	}
	return environmentInstallPlan(checkID, manager)
}

func environmentInstallPlan(checkID, manager string) (EnvironmentInstallPlan, error) {
	plan := EnvironmentInstallPlan{CheckID: checkID, PackageManager: manager}
	switch checkID {
	case "node":
		plan.Name = "Node.js 与 npm"
		switch manager {
		case "winget":
			plan.Packages = []string{"OpenJS.NodeJS.LTS"}
		case "choco":
			plan.Packages = []string{"nodejs-lts"}
		case "apt-get":
			plan.Packages = []string{"nodejs", "npm"}
		default:
			plan.Packages = []string{"nodejs", "npm"}
		}
	case "git":
		plan.Name, plan.Packages = "Git", []string{"git"}
		if manager == "winget" {
			plan.Packages = []string{"Git.Git"}
		}
	case "docker":
		plan.Name = "Docker"
		switch manager {
		case "apt-get":
			plan.Packages = []string{"docker.io"}
		case "dnf":
			plan.Packages = []string{"moby-engine"}
		case "brew":
			// Docker Desktop requires a GUI. Colima provides a headless macOS
			// runtime while the docker formula supplies the client command.
			plan.Name, plan.Packages = "Docker CLI 与 Colima", []string{"docker", "colima"}
		case "winget":
			plan.Name, plan.Packages = "Docker Desktop", []string{"Docker.DockerDesktop"}
		case "choco":
			plan.Name, plan.Packages = "Docker Desktop", []string{"docker-desktop"}
		default:
			plan.Packages = []string{"docker"}
		}
	case "browser":
		plan.Name = "浏览器"
		switch manager {
		case "pkg":
			plan.Packages = []string{"chromium"}
		case "winget":
			plan.Packages = []string{"Google.Chrome"}
		case "choco":
			plan.Packages = []string{"googlechrome"}
		case "brew":
			plan.Packages = []string{"--cask", "google-chrome"}
		case "dnf", "yum":
			if browser := rpmBrowserPackageAvailable(manager); browser != "" {
				plan.BrowserPackage = browser
				plan.Packages = []string{browser}
			}
		case "apt-get", "apk", "pacman":
			for _, group := range browserPackageCandidates(manager) {
				plan.Packages = append(plan.Packages, group...)
			}
		default:
			plan.Packages = []string{"chromium"}
		}
	case "browser-dependencies":
		plan.Name = "浏览器依赖补丁"
		switch manager {
		case "apt-get", "dnf", "yum", "apk", "pacman":
			for _, group := range browserDependencyPackageCandidates(manager) {
				plan.Packages = append(plan.Packages, group[0])
			}
		default:
			return EnvironmentInstallPlan{}, errors.New("当前系统无需安装浏览器依赖补丁")
		}
	case "common-dependencies":
		plan.Name = "常用环境依赖"
		switch manager {
		case "apt-get", "dnf", "yum", "apk", "pacman":
			plan.Packages = []string{"curl", "tar", "unzip"}
		default:
			return EnvironmentInstallPlan{}, errors.New("当前系统已内置常用环境依赖，无需安装")
		}
	case "fonts":
		plan.Name = "系统字体（CJK/Emoji）"
		switch manager {
		case "apt-get":
			plan.Packages = fontDependencyPackages("apt-get")
		case "dnf", "yum":
			plan.Packages = fontDependencyPackages("dnf")
		case "pacman":
			plan.Packages = fontDependencyPackages("pacman")
		case "apk":
			plan.Packages = fontDependencyPackages("apk")
		default:
			return EnvironmentInstallPlan{}, errors.New("当前系统已内置中文字体，无需在工作台内安装")
		}
	default:
		return EnvironmentInstallPlan{}, errors.New("该环境暂不支持工作台内安装")
	}
	return plan, nil
}

// rpmBrowserPackages are the fixed browser binary candidates probed on RPM
// hosts, in preference order. Only packages already present in the host's
// configured repositories are offered; the workbench never edits repositories.
var rpmBrowserPackages = []string{"chromium", "google-chrome-stable", "microsoft-edge-stable"}

// rpmBrowserPackageAvailable returns the first browser binary the configured
// repositories actually provide, or an empty string when none is available or
// the probe itself fails. The result is host policy and never browser input:
// it only decides whether the fixed package list may include the system
// browser binary. It is replaceable in tests.
var rpmBrowserPackageAvailable = func(manager string) string {
	if manager != "dnf" && manager != "yum" {
		return ""
	}
	if _, err := ResolveCommand(manager); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, candidate := range rpmBrowserPackages {
		args := []string{"list", "available", candidate}
		if manager == "dnf" {
			args = []string{"list", "--available", "--quiet", candidate}
		}
		if exec.CommandContext(ctx, manager, args...).Run() == nil {
			return candidate
		}
		if ctx.Err() != nil {
			return ""
		}
	}
	return ""
}

// browserDependencyPackagesForHost is deliberately limited to RPM hosts.
// Puppeteer often downloads Chromium itself there, but it still needs these
// host libraries and fonts to start. Keep this package list host-owned rather
// than accepting arbitrary browser input from the UI.
func browserDependencyPackagesForHost() []string {
	if runtime.GOOS != "linux" {
		return nil
	}
	for _, manager := range []string{"dnf", "yum"} {
		if _, err := ResolveCommand(manager); err == nil {
			return browserDependencyPackages(manager)
		}
	}
	return nil
}

func browserDependencyPackages(manager string) []string {
	if manager != "dnf" && manager != "yum" {
		return nil
	}
	groups := browserDependencyPackageCandidates(manager)
	packages := make([]string, 0, len(groups))
	for _, group := range groups {
		packages = append(packages, group[0])
	}
	return packages
}

// browserDependencyPackageCandidates lists every required runtime library
// with alternative names per package manager. Repository package names differ
// across distros and occasionally get renamed (for example the Debian -t64
// transition), so the workbench installs the first available candidate in
// each group and a group is reported as failed only when every candidate is
// unavailable.
func browserDependencyPackageCandidates(manager string) [][]string {
	switch manager {
	case "apt-get":
		return debianBrowserCorePackageCandidates()
	case "apk":
		return alpineBrowserCorePackageCandidates()
	case "pacman":
		return archBrowserCorePackageCandidates()
	default:
		return browserCorePackageCandidates()
	}
}

// browserPackageCandidates are the system browser binaries for non-RPM Linux
// package managers, in preference order. RPM hosts probe their configured
// repositories instead, because a browser binary often lives in EPEL or a
// vendor repository rather than the default repositories.
func browserPackageCandidates(manager string) [][]string {
	switch manager {
	case "apt-get":
		return [][]string{{"chromium", "chromium-browser"}}
	case "apk", "pacman":
		return [][]string{{"chromium"}}
	}
	return nil
}

// browserCorePackageCandidates are the runtime libraries a headless browser
// needs to start. A missing core library can break Puppeteer.
func browserCorePackageCandidates() [][]string {
	return [][]string{
		{"alsa-lib"},
		{"atk", "at-spi2-atk", "at-spi2-core"},
		{"cups-libs"},
		{"gtk3"},
		{"libXcomposite"},
		{"libXcursor"},
		{"libXdamage"},
		{"libXext"},
		{"libXi"},
		{"libXrandr"},
		{"libXScrnSaver", "libXss"},
		{"libXtst"},
		{"pango"},
		{"mesa-libgbm"},
		{"xorg-x11-utils"},
	}
}

// debianBrowserCorePackageCandidates are the Puppeteer-documented runtime
// libraries for Debian/Ubuntu. The -t64 variants cover Ubuntu 24.04's package
// renames.
func debianBrowserCorePackageCandidates() [][]string {
	return [][]string{
		{"libasound2", "libasound2t64"},
		{"libatk1.0-0", "libatk1.0-0t64", "libatk-bridge2.0-0", "libatk-bridge2.0-0t64", "libatspi2.0-0", "libatspi2.0-0t64"},
		{"libcups2", "libcups2t64"},
		{"libgtk-3-0", "libgtk-3-0t64"},
		{"libxcomposite1"},
		{"libxcursor1"},
		{"libxdamage1"},
		{"libxext6"},
		{"libxi6"},
		{"libxrandr2"},
		{"libxss1"},
		{"libxtst6"},
		{"libpango-1.0-0", "libpango-1.0-0t64"},
		{"libgbm1"},
		{"libnss3", "libnss3t64"},
		{"libnspr4", "libnspr4t64"},
		{"libxkbcommon0"},
		{"libdrm2"},
		{"libxshmfence1"},
		{"libxfixes3"},
		{"libx11-6"},
		{"libxcb1"},
		{"libdbus-1-3", "libdbus-1-3t64"},
		{"libglib2.0-0", "libglib2.0-0t64"},
		{"libcairo2"},
		{"fonts-liberation"},
		{"xdg-utils"},
	}
}

// alpineBrowserCorePackageCandidates are the runtime libraries a headless
// browser needs on Alpine (apk). Names follow the Alpine community packages.
func alpineBrowserCorePackageCandidates() [][]string {
	return [][]string{
		{"alsa-lib"},
		{"atk", "at-spi2-atk", "at-spi2-core"},
		{"cups-libs"},
		{"gtk3"},
		{"libxcomposite"},
		{"libxcursor"},
		{"libxdamage"},
		{"libxext"},
		{"libxi"},
		{"libxrandr"},
		{"libxscrnsaver", "libxss"},
		{"libxtst"},
		{"pango"},
		{"mesa-gbm", "mesa"},
		{"nss"},
		{"nspr"},
		{"libxkbcommon"},
		{"libdrm"},
		{"xshmfence", "libxshmfence"},
		{"libxfixes"},
		{"libx11"},
		{"libxcb"},
		{"dbus-libs", "dbus"},
		{"glib"},
		{"cairo"},
	}
}

// archBrowserCorePackageCandidates are the runtime libraries a headless
// browser needs on Arch Linux (pacman).
func archBrowserCorePackageCandidates() [][]string {
	return [][]string{
		{"alsa-lib"},
		{"atk", "at-spi2-atk", "at-spi2-core"},
		{"cups"},
		{"gtk3"},
		{"libxcomposite"},
		{"libxcursor"},
		{"libxdamage"},
		{"libxext"},
		{"libxi"},
		{"libxrandr"},
		{"libxss", "libxscrnsaver"},
		{"libxtst"},
		{"pango"},
		{"mesa", "libgbm"},
		{"nss"},
		{"nspr"},
		{"libxkbcommon"},
		{"libdrm"},
		{"libxshmfence"},
		{"libxfixes"},
		{"libx11"},
		{"libxcb"},
		{"dbus"},
		{"glib2"},
		{"cairo"},
		{"ttf-liberation"},
		{"xdg-utils"},
	}
}

// flattenedBrowserPlan lists the browser binary candidates followed by the
// first candidate of every runtime library group. It is only used for the
// install plan's Packages field; the install itself tries candidates one at a
// time.
func flattenedBrowserPlan(manager string) []string {
	var packages []string
	for _, group := range browserPackageCandidates(manager) {
		packages = append(packages, group...)
	}
	for _, group := range browserDependencyPackageCandidates(manager) {
		packages = append(packages, group[0])
	}
	return packages
}

// fontPackageGroups returns the Noto CJK/Emoji font candidates for each Linux
// package manager. Fonts are fully separate from the browser: missing fonts
// only affect text rendering in screenshots or PDFs, never browser startup.
func fontPackageGroups(manager string) [][]string {
	switch manager {
	case "apt-get":
		return [][]string{{"fonts-noto-cjk"}, {"fonts-noto-color-emoji"}}
	case "dnf", "yum":
		return [][]string{
			{"google-noto-serif-cjk-fonts", "google-noto-sans-cjk-fonts", "wqy-microhei-fonts", "wqy-zenhei-fonts"},
			{"google-noto-emoji-fonts", "google-noto-color-emoji-fonts"},
		}
	case "pacman":
		return [][]string{{"noto-fonts-cjk"}, {"noto-fonts-emoji"}}
	case "apk":
		return [][]string{{"font-noto-cjk"}, {"font-noto-emoji"}}
	}
	return nil
}

func fontDependencyPackages(manager string) []string {
	groups := fontPackageGroups(manager)
	packages := make([]string, 0, len(groups))
	for _, group := range groups {
		packages = append(packages, group[0])
	}
	return packages
}

func hostPackageManager() (string, error) {
	if runtime.GOOS == "darwin" {
		if _, err := ResolveCommand("brew"); err != nil {
			return "", errors.New("未检测到 Homebrew。请先由 macOS 管理员安装 Homebrew，随后即可在工作台内安装环境")
		}
		RefreshCommandEnvironment("brew")
		return "brew", nil
	}
	if runtime.GOOS == "windows" {
		for _, name := range []string{"winget", "choco"} {
			if _, err := ResolveCommand(name); err == nil {
				RefreshCommandEnvironment(name)
				return name, nil
			}
		}
		return "", errors.New("未检测到 WinGet 或 Chocolatey。请由 Windows 管理员预装其中之一，随后即可在工作台内安装环境")
	}
	if runtime.GOOS == "freebsd" {
		if _, err := ResolveCommand("pkg"); err != nil {
			return "", errors.New("未检测到 FreeBSD pkg 包管理器")
		}
		RefreshCommandEnvironment("pkg")
		return "pkg", nil
	}
	if runtime.GOOS != "linux" {
		return "", errors.New("工作台内安装目前支持 Linux、macOS、Windows 与 FreeBSD")
	}
	for _, name := range []string{"apt-get", "dnf", "yum", "apk", "pacman"} {
		if _, err := ResolveCommand(name); err == nil {
			RefreshCommandEnvironment(name)
			return name, nil
		}
	}
	return "", errors.New("未检测到受支持的 Linux 包管理器（APT、DNF、YUM、APK 或 Pacman）")
}

// InstallEnvironment performs a reviewed package installation on a supported
// host without opening an external browser.
func InstallEnvironment(ctx context.Context, checkID string) (string, error) {
	if strings.TrimSpace(checkID) == "node" {
		return InstallNodeWithNVM(ctx)
	}
	plan, err := EnvironmentInstallPlanForHost(checkID)
	if err != nil {
		return "", err
	}
	if checkID == "browser" && (plan.PackageManager == "dnf" || plan.PackageManager == "yum") {
		return installBrowserEnvironment(ctx, plan)
	}
	if checkID == "browser" && (plan.PackageManager == "apt-get" || plan.PackageManager == "apk" || plan.PackageManager == "pacman") {
		return installBrowserEnvironment(ctx, plan)
	}
	if checkID == "browser-dependencies" {
		return installBrowserDependencies(ctx, plan)
	}
	if checkID == "fonts" && (plan.PackageManager == "apt-get" || plan.PackageManager == "dnf" || plan.PackageManager == "yum" || plan.PackageManager == "pacman" || plan.PackageManager == "apk") {
		return installFontsEnvironment(ctx, plan)
	}
	// Refresh package metadata before every fixed install so a stale or
	// incomplete source never turns into a false "package not found".
	if strings.TrimSpace(checkID) != "node" {
		packageManagerPrecondition(ctx, plan.PackageManager)
	}
	text, runErr := runPackageCommand(ctx, plan.PackageManager, installArguments(plan.PackageManager, plan.Packages))
	if runErr == nil {
		RefreshCommandEnvironment(checkID)
		return fmt.Sprintf("已在当前主机安装 %s。请重新检查环境确认版本。", plan.Name), nil
	}
	return "", packageInstallFailure(plan.Name, text, runErr, ctx)
}

// installBrowserEnvironment installs only a browser binary. Runtime libraries
// are intentionally handled by installBrowserDependencies so users can repair
// a downloaded Puppeteer browser without installing another system browser.
func installBrowserEnvironment(ctx context.Context, plan EnvironmentInstallPlan) (string, error) {
	manager := plan.PackageManager
	if manager == "dnf" || manager == "yum" {
		prepareRPMRepositories(ctx, manager)
	} else {
		packageManagerPrecondition(ctx, manager)
	}
	candidates := [][]string{plan.Packages}
	switch manager {
	case "dnf", "yum":
		candidates = make([][]string, 0, len(rpmBrowserPackages))
		if plan.BrowserPackage != "" {
			candidates = append(candidates, []string{plan.BrowserPackage})
		}
		for _, candidate := range rpmBrowserPackages {
			if candidate != plan.BrowserPackage {
				candidates = append(candidates, []string{candidate})
			}
		}
	case "apt-get", "apk", "pacman":
		candidates = browserPackageCandidates(manager)
	}
	failed := make([]string, 0, len(candidates))
	for _, group := range candidates {
		installed, err := installCandidateGroup(ctx, manager, plan.Name, group)
		if err != nil {
			return "", err
		}
		if installed != "" {
			RefreshCommandEnvironment("browser")
			return fmt.Sprintf("已在当前主机安装浏览器（%s）。请重新检查环境确认版本。", installed), nil
		}
		if len(group) > 0 {
			failed = append(failed, group[0])
		}
	}
	return "", fmt.Errorf("服务器安装浏览器失败：以下软件包均未安装：%s。请检查软件源或网络后重试", strings.Join(failed, "、"))
}

func installBrowserDependencies(ctx context.Context, plan EnvironmentInstallPlan) (string, error) {
	manager := plan.PackageManager
	if manager == "dnf" || manager == "yum" {
		prepareRPMRepositories(ctx, manager)
	} else {
		packageManagerPrecondition(ctx, manager)
	}
	failed := make([]string, 0, len(browserDependencyPackageCandidates(manager)))
	for _, group := range browserDependencyPackageCandidates(manager) {
		installed, err := installCandidateGroup(ctx, manager, plan.Name, group)
		if err != nil {
			return "", err
		}
		if installed == "" {
			failed = append(failed, group[0])
		}
	}
	if len(failed) == 0 {
		return "已安装浏览器依赖补丁。请重新检查环境确认。", nil
	}
	return fmt.Sprintf("已安装部分浏览器依赖补丁；以下软件包未找到：%s。请检查软件源或网络后重试。", strings.Join(failed, "、")), nil
}

// installRPMBrowserEnvironment installs the fixed runtime libraries and fonts
// one package group at a time, then optionally the system browser binary.
// Repository metadata is refreshed first (makecache), each group tries its
// candidate names in order, and only a group where every candidate fails is
// reported at the end — one unavailable package never aborts the rest.
func installRPMBrowserEnvironment(ctx context.Context, plan EnvironmentInstallPlan) (string, error) {
	manager := plan.PackageManager
	// Preconditions, run before every install: enable EPEL and update the
	// system repository state. Both are best-effort; failures never block the
	// per-package attempts that follow.
	reposPrepared := prepareRPMRepositories(ctx, manager)
	// A stale metadata cache is another common cause of "No match for
	// argument". makecache is best-effort for the same reason.
	makecacheOutput, makecacheErr := runPackageCommand(ctx, manager, []string{"makecache"})
	if makecacheErr != nil {
		if ctx.Err() != nil {
			return "", errors.New("服务器安装超时，请检查网络与包管理器状态后重试")
		}
		if fatalPackageError(makecacheOutput) {
			return "", packageInstallFailure(plan.Name, makecacheOutput, makecacheErr, ctx)
		}
	}
	// RHEL-family default repositories have no chromium; it lives in EPEL.
	// Re-probe after the repository preparation, then install if available.
	if plan.BrowserPackage == "" {
		plan.BrowserPackage = rpmBrowserPackageAvailable(manager)
	}
	groups := browserDependencyPackageCandidates(manager)
	browserCandidates := rpmBrowserPackages
	if plan.BrowserPackage != "" {
		// The repository probe is only an optimization. Always keep the fixed
		// candidates behind it because dnf metadata can be stale immediately
		// after EPEL/CRB is enabled.
		browserCandidates = append([]string{plan.BrowserPackage}, browserCandidates...)
	}
	seenBrowsers := map[string]bool{}
	uniqueBrowsers := make([]string, 0, len(browserCandidates))
	for _, candidate := range browserCandidates {
		if candidate != "" && !seenBrowsers[candidate] {
			seenBrowsers[candidate] = true
			uniqueBrowsers = append(uniqueBrowsers, candidate)
		}
	}
	total := len(groups) + len(uniqueBrowsers)
	failed := make([]string, 0, 2)
	for _, group := range groups {
		installed, installErr := installCandidateGroup(ctx, manager, plan.Name, group)
		if installErr != nil {
			if len(failed) > 0 {
				return "", fmt.Errorf("%v（此前已完成安装，以下包未安装：%s）", installErr, strings.Join(failed, "、"))
			}
			return "", installErr
		}
		if installed == "" {
			failed = append(failed, group[0])
		}
	}
	browserPackage := ""
	for _, candidate := range uniqueBrowsers {
		installed, installErr := installCandidateGroup(ctx, manager, plan.Name, []string{candidate})
		if installErr != nil {
			if len(failed) > 0 {
				return "", fmt.Errorf("%v（此前已完成安装，以下包未安装：%s）", installErr, strings.Join(failed, "、"))
			}
			return "", installErr
		}
		if installed != "" {
			browserPackage = installed
			break
		}
		failed = append(failed, candidate)
	}
	RefreshCommandEnvironment(plan.CheckID)
	if len(failed) == 0 {
		return fmt.Sprintf("已在当前主机安装浏览器及依赖包（%s）。请重新检查环境确认版本。", browserPackage), nil
	}
	if browserPackage != "" {
		return fmt.Sprintf("已在当前主机安装浏览器及部分运行库（%s）；以下软件包未能安装：%s。请重新检查环境确认。", browserPackage, strings.Join(failed, "、")), nil
	}
	if len(failed) == total {
		return "", fmt.Errorf("服务器安装 %s 失败：以下软件包均未安装：%s。请检查软件源或网络后重试", plan.Name, strings.Join(failed, "、"))
	}
	browserNote := "当前软件源未提供 Chromium、Chrome 或 Edge 软件包"
	if reposPrepared {
		browserNote = "已由 ALemonX 启用 EPEL/CRB 并刷新软件源，但仍未找到 Chromium、Chrome 或 Edge 软件包"
	}
	return fmt.Sprintf("已在当前主机安装浏览器运行库；%s。仅当项目已自带或下载浏览器时才能进行 Puppeteer 自动化，请重新检查环境确认。", browserNote), nil
}

// installLinuxBrowserEnvironment installs the runtime libraries and the
// browser binary on apt/apk/pacman hosts one package group at a time. The
// same skip-fail-then-summarize semantics as the RPM path apply: a missing
// package never aborts the remaining groups.
func installLinuxBrowserEnvironment(ctx context.Context, plan EnvironmentInstallPlan) (string, error) {
	manager := plan.PackageManager
	packageManagerPrecondition(ctx, manager)
	dependencyGroups := browserDependencyPackageCandidates(manager)
	browserGroups := browserPackageCandidates(manager)
	total := len(dependencyGroups) + len(browserGroups)
	failed := make([]string, 0, 2)
	for _, group := range dependencyGroups {
		installed, installErr := installCandidateGroup(ctx, manager, plan.Name, group)
		if installErr != nil {
			if len(failed) > 0 {
				return "", fmt.Errorf("%v（此前已完成安装，以下包未安装：%s）", installErr, strings.Join(failed, "、"))
			}
			return "", installErr
		}
		if installed == "" {
			failed = append(failed, group[0])
		}
	}
	browserName := ""
	for _, group := range browserGroups {
		installed, installErr := installCandidateGroup(ctx, manager, plan.Name, group)
		if installErr != nil {
			if len(failed) > 0 {
				return "", fmt.Errorf("%v（此前已完成安装，以下包未安装：%s）", installErr, strings.Join(failed, "、"))
			}
			return "", installErr
		}
		if installed != "" {
			browserName = installed
			break
		}
		failed = append(failed, group[0])
	}
	RefreshCommandEnvironment(plan.CheckID)
	if len(failed) == 0 {
		return fmt.Sprintf("已在当前主机安装浏览器及依赖包（%s）。请重新检查环境确认版本。", browserName), nil
	}
	if browserName == "" && len(failed) == total {
		return "", fmt.Errorf("服务器安装 %s 失败：以下软件包均未安装：%s。请检查软件源或网络后重试", plan.Name, strings.Join(failed, "、"))
	}
	return fmt.Sprintf("已在当前主机安装浏览器运行库；以下软件包未能安装：%s（可能影响浏览器自动化功能）。Puppeteer 将使用项目自带的浏览器；如软件源确实缺少这些包，请检查软件源或网络后重试。请重新检查环境确认。", strings.Join(failed, "、")), nil
}

// installFontsEnvironment installs the optional Noto CJK/Emoji fonts. Fonts
// are fully separate from the browser: missing ones never affect browser
// startup, only text rendering in screenshots or PDFs.
func installFontsEnvironment(ctx context.Context, plan EnvironmentInstallPlan) (string, error) {
	manager := plan.PackageManager
	if manager == "dnf" || manager == "yum" {
		prepareRPMRepositories(ctx, manager)
	} else {
		packageManagerPrecondition(ctx, manager)
	}
	var failed []string
	for _, group := range fontPackageGroups(manager) {
		installed, installErr := installCandidateGroup(ctx, manager, plan.Name, group)
		if installErr != nil {
			if len(failed) > 0 {
				return "", fmt.Errorf("%v（此前已完成安装，以下字体包未安装：%s）", installErr, strings.Join(failed, "、"))
			}
			return "", installErr
		}
		if installed == "" {
			failed = append(failed, group[0])
		}
	}
	RefreshCommandEnvironment(plan.CheckID)
	if len(failed) == 0 {
		return "已安装系统字体（Noto CJK/Emoji）。请重新检查环境确认。", nil
	}
	return fmt.Sprintf("已安装部分字体；以下字体包未找到：%s（不影响浏览器与基本功能，仅影响部分文字渲染）。如软件源缺少这些包，可启用 EPEL 等仓库后重试。请重新检查环境确认。", strings.Join(failed, "、")), nil
}

// prepareRPMRepositories runs the fixed dnf/yum preconditions before every
// install: enable CRB (EL9-family, dnf only), enable EPEL, then update the
// repository state and packages. It is best-effort host policy: failures are
// skipped and reported by the per-package attempts that follow. The result
// reports whether EPEL was enabled and the update completed, so the caller
// can phrase the browser availability note accurately.
func prepareRPMRepositories(ctx context.Context, manager string) bool {
	if manager != "dnf" && manager != "yum" {
		return false
	}
	if manager == "dnf" {
		enableCRBRepository(ctx, manager)
	}
	output, runErr := runPackageCommand(ctx, manager, []string{"install", "-y", "epel-release"})
	if runErr != nil || ctx.Err() != nil || fatalPackageError(output) {
		return false
	}
	output, runErr = runPackageCommand(ctx, manager, []string{"update", "-y"})
	return runErr == nil && ctx.Err() == nil && !fatalPackageError(output)
}

// enableCRBRepository best-effort enables the CodeReady Builder (CRB) /
// PowerTools repository on EL9-family hosts. Many EPEL packages depend on it
// (chromium dependency resolution included), so it must be enabled before
// epel-release on dnf hosts. Every failure is skipped: hosts without CRB (or
// without the config-manager plugin) simply proceed with what they have.
func enableCRBRepository(ctx context.Context, manager string) {
	if manager != "dnf" {
		return
	}
	// dnf config-manager is provided by dnf-plugins-core.
	_, _ = runPackageCommand(ctx, manager, []string{"install", "-y", "dnf-plugins-core"})
	if ctx.Err() != nil {
		return
	}
	output, runErr := runPackageCommand(ctx, manager, []string{"config-manager", "--set-enabled", "crb"})
	if runErr == nil && ctx.Err() == nil && !fatalPackageError(output) {
		return
	}
	// AlmaLinux/Rocky 9 also ship a crb helper script; RHEL proper needs
	// subscription-manager instead and is left untouched.
	_, _ = runPackageCommand(ctx, "crb", []string{"enable"})
}

// packageManagerPrecondition refreshes package metadata before a fixed install
// on non-RPM package managers. Best-effort: failures are skipped so the
// install attempt still runs and reports what could not be found.
func packageManagerPrecondition(ctx context.Context, manager string) bool {
	var args []string
	switch manager {
	case "apt-get":
		args = []string{"update"}
	case "pacman":
		args = []string{"-Sy"}
	case "apk":
		args = []string{"update"}
	case "brew":
		args = []string{"update"}
	case "pkg":
		args = []string{"update", "-f"}
	default:
		return false
	}
	output, runErr := runPackageCommand(ctx, manager, args)
	if runErr != nil || ctx.Err() != nil || fatalPackageError(output) {
		return false
	}
	return true
}

// installCandidateGroup tries every candidate name in a group until one
// installs. A hard error (timeout, authorization) aborts; otherwise it returns
// the installed candidate name, or an empty string when every candidate was
// unavailable.
func installCandidateGroup(ctx context.Context, manager, name string, group []string) (string, error) {
	for _, candidate := range group {
		output, runErr := runPackageCommand(ctx, manager, installArguments(manager, []string{candidate}))
		if runErr == nil {
			return candidate, nil
		}
		if ctx.Err() != nil {
			return "", errors.New("服务器安装超时，请检查网络与包管理器状态后重试")
		}
		if fatalPackageError(output) {
			return "", packageInstallFailure(name, output, runErr, ctx)
		}
	}
	return "", nil
}

// fatalPackageError reports whether a package-manager failure is environmental
// (authorization/privilege) and must abort instead of being skipped like an
// ordinary unavailable package.
func fatalPackageError(text string) bool {
	lower := strings.ToLower(text)
	if runtime.GOOS == "windows" && (strings.Contains(lower, "access is denied") || strings.Contains(lower, "administrator")) {
		return true
	}
	return strings.Contains(lower, "a password is required") ||
		strings.Contains(lower, "no tty present") ||
		strings.Contains(lower, "not in the sudoers")
}

// runPackageCommand executes one fixed package-manager command with the same
// privilege rules as the rest of the workbench. It is replaceable in tests.
var runPackageCommand = func(ctx context.Context, manager string, args []string) (string, error) {
	program := manager
	if runtime.GOOS == "darwin" {
		if os.Geteuid() == 0 {
			return "", errors.New("Homebrew 不能以 root 运行；请使用实际 macOS 用户账户启动 AlemonX 服务")
		}
	} else if runtime.GOOS == "linux" && os.Geteuid() != 0 {
		if _, err := exec.LookPath("sudo"); err != nil {
			return "", errors.New("服务器当前不是 root，且未安装 sudo；请由服务器管理员完成安装")
		}
		args = append([]string{"-n", "--", manager}, args...)
		program = "sudo"
	}
	output, runErr := exec.CommandContext(ctx, program, args...).CombinedOutput()
	return strings.TrimSpace(string(output)), runErr
}

// packageInstallFailure converts a failed fixed package-manager run into the
// user-facing error, keeping the output bounded for the setup UI.
func packageInstallFailure(name, text string, runErr error, ctx context.Context) error {
	lower := strings.ToLower(text)
	if runtime.GOOS == "windows" && (strings.Contains(lower, "access is denied") || strings.Contains(lower, "administrator")) {
		return errors.New("Windows 包管理器需要管理员权限。请以管理员账户运行 AlemonX 服务后重试；线上工作台不会尝试弹出桌面 UAC 窗口")
	}
	if strings.Contains(lower, "a password is required") || strings.Contains(lower, "no tty present") || strings.Contains(lower, "not in the sudoers") {
		return errors.New("服务器需要管理员授权。为保持线上工作台安全，请由管理员以 root 运行服务，或仅为该固定包管理命令配置 sudo -n 后重试")
	}
	if ctx.Err() != nil {
		return errors.New("服务器安装超时，请检查网络与包管理器状态后重试")
	}
	if text == "" {
		text = runErr.Error()
	}
	if len(text) > 600 {
		text = text[:600] + "…"
	}
	return fmt.Errorf("服务器安装 %s 失败：%s", name, text)
}

func installArguments(manager string, packages []string) []string {
	switch manager {
	case "winget":
		return []string{"install", "--id", packages[0], "--exact", "--silent", "--accept-package-agreements", "--accept-source-agreements"}
	case "choco":
		return append([]string{"install", "-y", "--no-progress"}, packages...)
	case "pacman":
		return append([]string{"-S", "--noconfirm"}, packages...)
	case "apk":
		return append([]string{"add", "--no-cache"}, packages...)
	case "dnf":
		return append([]string{"install", "-y", "--allowerasing"}, packages...)
	case "brew":
		return append([]string{"install"}, packages...)
	default:
		return append([]string{"install", "-y"}, packages...)
	}
}
