package system

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// EnvironmentInstallPlan is a reviewed, fixed package-manager operation. It
// never contains browser input other than the small prerequisite identifier.
type EnvironmentInstallPlan struct {
	CheckID        string
	Name           string
	PackageManager string
	Packages       []string
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
			plan.Packages = append([]string{"chromium"}, browserDependencyPackages(manager)...)
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
	return []string{
		"alsa-lib", "atk", "cups-libs", "gtk3", "libXcomposite", "libXcursor", "libXdamage",
		"libXext", "libXi", "libXrandr", "libXScrnSaver", "libXtst", "pango", "mesa-libgbm",
		"ipa-gothic-fonts", "xorg-x11-fonts-100dpi", "xorg-x11-fonts-75dpi", "xorg-x11-utils",
		"xorg-x11-fonts-cyrillic", "xorg-x11-fonts-Type1", "xorg-x11-fonts-misc", "wqy-microhei-fonts",
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
	args := installArguments(plan.PackageManager, plan.Packages)
	program := plan.PackageManager
	if runtime.GOOS == "darwin" {
		if os.Geteuid() == 0 {
			return "", errors.New("Homebrew 不能以 root 运行；请使用实际 macOS 用户账户启动 AlemonX 服务")
		}
	} else if runtime.GOOS == "linux" && os.Geteuid() != 0 {
		if _, err := exec.LookPath("sudo"); err != nil {
			return "", errors.New("服务器当前不是 root，且未安装 sudo；请由服务器管理员完成安装")
		}
		args = append([]string{"-n", "--", program}, args...)
		program = "sudo"
	}
	output, runErr := exec.CommandContext(ctx, program, args...).CombinedOutput()
	text := strings.TrimSpace(string(output))
	if runErr == nil {
		RefreshCommandEnvironment(checkID)
		return fmt.Sprintf("已在当前主机安装 %s。请重新检查环境确认版本。", plan.Name), nil
	}
	lower := strings.ToLower(text)
	if runtime.GOOS == "windows" && (strings.Contains(lower, "access is denied") || strings.Contains(lower, "administrator")) {
		return "", errors.New("Windows 包管理器需要管理员权限。请以管理员账户运行 AlemonX 服务后重试；线上工作台不会尝试弹出桌面 UAC 窗口")
	}
	if strings.Contains(lower, "a password is required") || strings.Contains(lower, "no tty present") || strings.Contains(lower, "not in the sudoers") {
		return "", errors.New("服务器需要管理员授权。为保持线上工作台安全，请由管理员以 root 运行服务，或仅为该固定包管理命令配置 sudo -n 后重试")
	}
	if ctx.Err() != nil {
		return "", errors.New("服务器安装超时，请检查网络与包管理器状态后重试")
	}
	if text == "" {
		text = runErr.Error()
	}
	if len(text) > 600 {
		text = text[:600] + "…"
	}
	return "", fmt.Errorf("服务器安装 %s 失败：%s", plan.Name, text)
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
