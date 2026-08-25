package redis

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"alemonx/internal/system"
)

// NativeStatus describes the host Redis service without taking ownership of
// its process. Native Redis is deliberately managed by systemd, not ALemonX.
type NativeStatus struct {
	Supported bool
	Installed bool
	Running   bool
	Enabled   bool
	Service   string
}

func inspectNative(port int) NativeStatus {
	status := NativeStatus{Supported: runtime.GOOS == "linux"}
	if !status.Supported {
		return status
	}
	if _, err := exec.LookPath("redis-server"); err != nil {
		return status
	}
	status.Installed = true
	if _, err := exec.LookPath("systemctl"); err != nil {
		status.Running, _ = probeRedis(fmt.Sprintf("127.0.0.1:%d", port), nativeProbeTimeout)
		return status
	}
	for _, service := range []string{"redis-server", "redis"} {
		if commandOK("systemctl", "is-active", "--quiet", service) {
			status.Service = service
			status.Running = true
			status.Enabled = commandOK("systemctl", "is-enabled", "--quiet", service)
			break
		}
		if commandOK("systemctl", "is-enabled", "--quiet", service) {
			status.Service = service
			status.Enabled = true
		}
	}
	if status.Service == "" {
		status.Service = "redis-server"
	}
	if !status.Running {
		status.Running, _ = probeRedis(fmt.Sprintf("127.0.0.1:%d", port), nativeProbeTimeout)
	}
	return status
}

const nativeProbeTimeout = 250 * 1000 * 1000 // 250ms

func commandOK(name string, args ...string) bool {
	return exec.Command(name, args...).Run() == nil
}

func runNativePrivileged(name string, args ...string) ([]byte, error) {
	if os.Geteuid() == 0 {
		return exec.Command(name, args...).CombinedOutput()
	}
	return system.RunWithPrivilegesInput("", name, args, nil, nil)
}

func installNative() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("独立 Redis 安装目前仅支持 Linux")
	}
	if _, err := exec.LookPath("redis-server"); err != nil {
		manager, err := findPackageManager()
		if err != nil {
			return err
		}
		for _, command := range manager.commands() {
			output, runErr := runNativePrivileged(command.name, command.args...)
			if runErr != nil {
				return fmt.Errorf("安装 Redis 失败：%s：%w", strings.TrimSpace(string(output)), runErr)
			}
		}
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("Redis 已安装，但当前 Linux 没有 systemd，无法配置开机自启")
	}
	var lastOutput string
	for _, service := range []string{"redis-server", "redis"} {
		output, err := runNativePrivileged("systemctl", "enable", "--now", service)
		lastOutput = strings.TrimSpace(string(output))
		if err == nil {
			return nil
		}
	}
	if lastOutput == "" {
		lastOutput = "未找到 Redis systemd 服务单元"
	}
	return fmt.Errorf("Redis 已安装，但无法启用 systemd 服务：%s", lastOutput)
}

func startNativeService(service string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("独立 Redis 服务目前仅支持 Linux")
	}
	if service == "" {
		service = "redis-server"
	}
	output, err := runNativePrivileged("systemctl", "start", service)
	if err != nil {
		return fmt.Errorf("启动独立 Redis 失败：%s：%w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

type nativePackageManager string

func findPackageManager() (nativePackageManager, error) {
	for _, name := range []string{"apt-get", "dnf", "yum", "pacman", "apk"} {
		if _, err := exec.LookPath(name); err == nil {
			return nativePackageManager(name), nil
		}
	}
	return "", fmt.Errorf("未找到 apt、dnf、yum、pacman 或 apk，无法自动安装 Redis")
}

type nativeCommand struct {
	name string
	args []string
}

func (manager nativePackageManager) commands() []nativeCommand {
	switch manager {
	case "apt-get":
		return []nativeCommand{{"apt-get", []string{"update"}}, {"apt-get", []string{"install", "-y", "redis-server"}}}
	case "dnf":
		return []nativeCommand{{"dnf", []string{"install", "-y", "redis"}}}
	case "yum":
		return []nativeCommand{{"yum", []string{"install", "-y", "redis"}}}
	case "pacman":
		return []nativeCommand{{"pacman", []string{"-Sy", "--noconfirm", "redis"}}}
	case "apk":
		return []nativeCommand{{"apk", []string{"add", "redis"}}}
	default:
		return nil
	}
}
