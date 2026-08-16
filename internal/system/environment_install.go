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
		plan.Name = "浏览器及依赖包"
		switch manager {
		case "winget":
			plan.Packages = []string{"Google.Chrome"}
		case "choco":
			plan.Packages = []string{"googlechrome"}
		case "brew":
			plan.Packages = []string{"--cask", "google-chrome"}
		case "dnf", "yum":
			// RHEL-family repositories do not always ship a browser binary
			// (el9 needs EPEL or a vendor repository). Probe the configured
			// repositories and include the first available browser package;
			// the fixed runtime library and font list is always installed so
			// Puppeteer can start the browser it downloads per project.
			packages := browserDependencyPackages(manager)
			if browser := rpmBrowserPackageAvailable(manager); browser != "" {
				plan.BrowserPackage = browser
				packages = append([]string{browser}, packages...)
			}
			plan.Packages = packages
		case "apt-get", "apk", "pacman":
			plan.Packages = []string{"chromium"}
		default:
			plan.Packages = []string{"chromium"}
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
	packages := make([]string, 0, len(browserDependencyPackageCandidates()))
	for _, group := range browserDependencyPackageCandidates() {
		packages = append(packages, group[0])
	}
	return packages
}

// browserDependencyPackageCandidates lists every required runtime library and
// font with alternative names, because repository package names differ across
// RHEL-family distros and occasionally get renamed. The workbench installs the
// first available candidate in each group; a group is reported as failed only
// when every candidate is unavailable.
func browserDependencyPackageCandidates() [][]string {
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
		{"ipa-gothic-fonts", "vlgothic-fonts"},
		{"xorg-x11-fonts-100dpi", "xorg-x11-fonts-ISO8859-1-100dpi"},
		{"xorg-x11-fonts-75dpi", "xorg-x11-fonts-ISO8859-1-75dpi"},
		{"xorg-x11-utils"},
		{"xorg-x11-fonts-cyrillic"},
		{"xorg-x11-fonts-Type1"},
		{"xorg-x11-fonts-misc"},
		{"wqy-microhei-fonts", "wqy-zenhei-fonts", "google-noto-sans-cjk-fonts"},
	}
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
	if runtime.GOOS != "linux" {
		return "", errors.New("工作台内安装目前支持 Linux、macOS 与 Windows")
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
		return InstallManagedNode(ctx)
	}
	plan, err := EnvironmentInstallPlanForHost(checkID)
	if err != nil {
		return "", err
	}
	if checkID == "browser" && (plan.PackageManager == "dnf" || plan.PackageManager == "yum") {
		return installRPMBrowserEnvironment(ctx, plan)
	}
	text, runErr := runPackageCommand(ctx, plan.PackageManager, installArguments(plan.PackageManager, plan.Packages))
	if runErr == nil {
		RefreshCommandEnvironment(checkID)
		return fmt.Sprintf("已在当前主机安装 %s。请重新检查环境确认版本。", plan.Name), nil
	}
	return "", packageInstallFailure(plan.Name, text, runErr, ctx)
}

// installRPMBrowserEnvironment installs the fixed runtime libraries and fonts
// one package group at a time, then optionally the system browser binary.
// Repository metadata is refreshed first (makecache), each group tries its
// candidate names in order, and only a group where every candidate fails is
// reported at the end — one unavailable package never aborts the rest.
func installRPMBrowserEnvironment(ctx context.Context, plan EnvironmentInstallPlan) (string, error) {
	manager := plan.PackageManager
	groups := browserDependencyPackageCandidates()
	// Precondition: a stale or incomplete metadata cache is a common cause of
	// "No match for argument". makecache is best-effort; if it fails (network
	// or repository config) the per-package attempts still run and the summary
	// reports what could not be found.
	makecacheOutput, makecacheErr := runPackageCommand(ctx, manager, []string{"makecache"})
	if makecacheErr != nil {
		if ctx.Err() != nil {
			return "", errors.New("服务器安装超时，请检查网络与包管理器状态后重试")
		}
		if fatalPackageError(makecacheOutput) {
			return "", packageInstallFailure(plan.Name, makecacheOutput, makecacheErr, ctx)
		}
	}
	total := len(groups)
	if plan.BrowserPackage != "" {
		total++
	}
	failed := make([]string, 0, 2)
	for _, group := range groups {
		installed := false
		for _, candidate := range group {
			output, runErr := runPackageCommand(ctx, manager, installArguments(manager, []string{candidate}))
			if runErr == nil {
				installed = true
				break
			}
			if ctx.Err() != nil {
				return "", errors.New("服务器安装超时，请检查网络与包管理器状态后重试")
			}
			if fatalPackageError(output) {
				return "", packageInstallFailure(plan.Name, output, runErr, ctx)
			}
		}
		if !installed {
			failed = append(failed, group[0])
		}
	}
	if plan.BrowserPackage != "" {
		output, runErr := runPackageCommand(ctx, manager, installArguments(manager, []string{plan.BrowserPackage}))
		if runErr != nil {
			if ctx.Err() != nil {
				return "", errors.New("服务器安装超时，请检查网络与包管理器状态后重试")
			}
			if fatalPackageError(output) {
				return "", packageInstallFailure(plan.Name, output, runErr, ctx)
			}
			failed = append(failed, plan.BrowserPackage)
		}
	}
	RefreshCommandEnvironment(plan.CheckID)
	if len(failed) == 0 {
		if plan.BrowserPackage == "" {
			return "已在当前主机安装浏览器运行库与字体；当前软件源未提供 Chromium、Chrome 或 Edge 软件包，Puppeteer 将使用项目自带的浏览器。请重新检查环境确认。", nil
		}
		return fmt.Sprintf("已在当前主机安装浏览器及依赖包（%s）。请重新检查环境确认版本。", plan.BrowserPackage), nil
	}
	if len(failed) == total {
		return "", fmt.Errorf("服务器安装 %s 失败：以下软件包均未安装：%s。请检查软件源或网络后重试", plan.Name, strings.Join(failed, "、"))
	}
	return fmt.Sprintf("已在当前主机安装浏览器运行库与字体；以下软件包未能安装：%s。Puppeteer 将使用项目自带的浏览器，缺失包可能影响部分自动化功能；如软件源确实缺少这些包，可启用 EPEL 等仓库后重试。请重新检查环境确认。", strings.Join(failed, "、")), nil
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
