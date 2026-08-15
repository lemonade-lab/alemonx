// Package system provides small cross-platform integrations for the setup app.
package system

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const serviceName = "com.alemonjs.alx"

// ServiceResilience describes the persistent supervisor that owns the
// workbench. It is intentionally separate from ServiceStatus: a process can
// be running now without being configured to return after login or a crash.
type ServiceResilience struct {
	StartupEnabled  bool   `json:"startupEnabled"`
	KeepAlive       bool   `json:"keepAlive"`
	LingerSupported bool   `json:"lingerSupported"`
	LingerKnown     bool   `json:"lingerKnown"`
	LingerEnabled   bool   `json:"lingerEnabled"`
	Summary         string `json:"summary"`
}

// ServiceResilienceStatus exposes only supervisor configuration, never the
// service command line. On Linux, linger keeps the user systemd manager alive
// after logout so a headless deployment truly survives a reboot.
func ServiceResilienceStatus() ServiceResilience {
	if !ServiceInstalled() {
		return ServiceResilience{Summary: "尚未安装工作台后台服务。"}
	}
	switch runtime.GOOS {
	case "darwin":
		path, err := launchAgentPath()
		data, readErr := os.ReadFile(path)
		if err != nil || readErr != nil {
			return ServiceResilience{Summary: "无法读取 LaunchAgent 保活配置。"}
		}
		text := string(data)
		startup := strings.Contains(text, "<key>RunAtLoad</key><true/>")
		keepAlive := strings.Contains(text, "<key>KeepAlive</key><true/>")
		return ServiceResilience{StartupEnabled: startup, KeepAlive: keepAlive, Summary: "登录后自动启动；进程异常退出后由 LaunchAgent 拉起。"}
	case "linux":
		home, err := os.UserHomeDir()
		if err != nil {
			return ServiceResilience{Summary: "无法定位 systemd 用户服务。"}
		}
		data, readErr := os.ReadFile(filepath.Join(home, ".config", "systemd", "user", "alx.service"))
		if readErr != nil {
			return ServiceResilience{Summary: "无法读取 systemd 用户服务配置。"}
		}
		text := string(data)
		startup := strings.Contains(text, "WantedBy=default.target")
		keepAlive := strings.Contains(text, "Restart=on-failure") || strings.Contains(text, "Restart=always")
		result := ServiceResilience{StartupEnabled: startup, KeepAlive: keepAlive, LingerSupported: true, Summary: "用户 systemd 已配置为异常退出自动重启。"}
		output, lingerErr := exec.Command("loginctl", "show-user", strconv.Itoa(os.Getuid()), "-p", "Linger", "--value").Output()
		if lingerErr != nil {
			result.LingerSupported = false
			result.Summary = "用户 systemd 已配置保活；无法确认 logout 后是否继续运行。"
			return result
		}
		result.LingerKnown = true
		result.LingerEnabled = strings.EqualFold(strings.TrimSpace(string(output)), "yes")
		if result.LingerEnabled {
			result.Summary = "已启用无登录运行：重启后无需用户登录即可启动，并会在异常退出后自动重启。"
		} else {
			result.Summary = "服务会在登录后启动并在异常退出后重启；启用无登录运行后，重启或退出登录也会持续运行。"
		}
		return result
	case "windows":
		return ServiceResilience{StartupEnabled: true, Summary: "已注册登录启动任务；建议使用 Docker 或系统服务承载需要无人值守运行的服务器部署。"}
	default:
		return ServiceResilience{Summary: "当前系统不支持工作台后台服务管理。"}
	}
}

// EnableUserLinger is an explicit Linux-only action. It can require host
// authorization, so callers must obtain confirmation before invoking it.
func EnableUserLinger() (string, error) {
	if runtime.GOOS != "linux" {
		return "", errors.New("无登录运行仅适用于 Linux systemd")
	}
	current, err := user.Current()
	if err != nil || strings.TrimSpace(current.Username) == "" {
		return "", errors.New("无法识别当前 Linux 用户")
	}
	output, err := exec.Command("loginctl", "enable-linger", current.Username).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("启用无登录运行失败：%s。请由管理员执行 loginctl enable-linger %s", message, current.Username)
	}
	return "已启用无登录运行；Linux 重启或用户退出登录后，ALemonX 用户服务仍会自动启动。", nil
}

// ServiceStatus reports whether the user-level service is registered and running.
func ServiceStatus() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		path, err := launchAgentPath()
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(path); err != nil {
			return "未安装后台服务。运行 alx install 进行安装。", nil
		}
		uid := strconv.Itoa(os.Getuid())
		if err := exec.Command("launchctl", "print", "gui/"+uid+"/"+serviceName).Run(); err != nil {
			return "后台服务已安装，目前已停止。", nil
		}
		return "后台服务运行中。", nil
	case "linux":
		output, err := exec.Command("systemctl", "--user", "is-active", "alx.service").CombinedOutput()
		if err != nil {
			return "后台服务未运行（" + strings.TrimSpace(string(output)) + "）。", nil
		}
		return "后台服务运行中。", nil
	case "windows":
		output, err := exec.Command("schtasks", "/Query", "/TN", "ALemonX", "/FO", "LIST").CombinedOutput()
		if err != nil {
			return "未安装后台服务。运行 alx install 进行安装。", nil
		}
		return "后台服务已注册。\n" + strings.TrimSpace(string(output)), nil
	default:
		return "", fmt.Errorf("暂不支持在 %s 上管理后台服务", runtime.GOOS)
	}
}

// ServiceInstalled reports whether a user-level background-service definition
// exists, regardless of whether it is currently running.
func ServiceInstalled() bool {
	switch runtime.GOOS {
	case "darwin":
		return userLaunchAgentInstalled()
	case "linux":
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		_, err = os.Stat(filepath.Join(home, ".config", "systemd", "user", "alx.service"))
		return err == nil
	case "windows":
		return windowsScheduledTaskInstalled()
	default:
		return false
	}
}

func StartService() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		path, err := launchAgentPath()
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(path); err != nil {
			return "", errors.New("未安装后台服务，请先运行 alx install")
		}
		uid := strconv.Itoa(os.Getuid())
		_ = exec.Command("launchctl", "bootout", "gui/"+uid+"/"+serviceName).Run()
		if output, err := exec.Command("launchctl", "bootstrap", "gui/"+uid, path).CombinedOutput(); err != nil {
			return "", fmt.Errorf("启动后台服务失败：%s", strings.TrimSpace(string(output)))
		}
		return "后台服务已启动。", nil
	case "linux":
		if home, homeErr := os.UserHomeDir(); homeErr == nil {
			_ = ensureSystemdUserUnitKillMode(filepath.Join(home, ".config", "systemd", "user", "alx.service"))
		}
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
		if output, err := exec.Command("systemctl", "--user", "start", "alx.service").CombinedOutput(); err != nil {
			return "", fmt.Errorf("启动后台服务失败：%s", strings.TrimSpace(string(output)))
		}
		return "后台服务已启动。", nil
	case "windows":
		if output, err := exec.Command("schtasks", "/Run", "/TN", "ALemonX").CombinedOutput(); err != nil {
			return "", fmt.Errorf("启动后台服务失败：%s", strings.TrimSpace(string(output)))
		}
		return "后台服务已启动。", nil
	default:
		return "", fmt.Errorf("暂不支持在 %s 上管理后台服务", runtime.GOOS)
	}
}

func StopService() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		uid := strconv.Itoa(os.Getuid())
		if output, err := exec.Command("launchctl", "bootout", "gui/"+uid+"/"+serviceName).CombinedOutput(); err != nil {
			// launchctl returns this when the LaunchAgent is installed but not
			// currently loaded. Stopping an already stopped managed service is
			// intentionally idempotent.
			if strings.Contains(string(output), "No such process") {
				return "后台服务当前未运行。", nil
			}
			return "", fmt.Errorf("停止后台服务失败：%s", strings.TrimSpace(string(output)))
		}
		return "后台服务已停止；登录启动配置仍然保留。", nil
	case "linux":
		if output, err := exec.Command("systemctl", "--user", "stop", "alx.service").CombinedOutput(); err != nil {
			return "", fmt.Errorf("停止后台服务失败：%s", strings.TrimSpace(string(output)))
		}
		return "后台服务已停止；登录启动配置仍然保留。", nil
	case "windows":
		if output, err := exec.Command("schtasks", "/End", "/TN", "ALemonX").CombinedOutput(); err != nil {
			return "", fmt.Errorf("停止后台服务失败：%s", strings.TrimSpace(string(output)))
		}
		return "后台服务已停止；登录启动配置仍然保留。", nil
	default:
		return "", fmt.Errorf("暂不支持在 %s 上管理后台服务", runtime.GOOS)
	}
}

// RestartForeground schedules the currently foreground-run instance to start
// again after it has released its listening port. It deliberately preserves
// the original command line so explicit bind, port, and development options
// survive the restart.
func RestartForeground(port string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("无法定位当前 alx：%w", err)
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	args := append([]string(nil), os.Args[1:]...)
	if !hasPortArgument(args) {
		args = append(args, "--port", port)
	}
	if runtime.GOOS == "windows" {
		quotedArgs := make([]string, len(args))
		for index, argument := range args {
			quotedArgs[index] = powershellQuote(argument)
		}
		script := strings.Join([]string{
			"Start-Sleep -Milliseconds 500",
			"Start-Process -FilePath " + powershellQuote(executable) + " -ArgumentList @(" + strings.Join(quotedArgs, ",") + ")",
		}, "; ")
		return exec.Command("powershell.exe", "-NoProfile", "-WindowStyle", "Hidden", "-Command", script).Start()
	}
	command := append([]string{"-c", "sleep 0.5; exec \"$@\"", "alx-foreground-restart", executable}, args...)
	return exec.Command("/bin/sh", command...).Start()
}

// PrepareService registers a background-service definition without starting
// it. It is used when the current foreground instance owns the listening port:
// the caller can close that instance first and then schedule StartService.
func PrepareService(port string) (string, error) {
	return installService(port, "0.0.0.0", false)
}

// ScheduleServiceStart starts a previously registered service after a short
// delay, allowing the foreground process to release its HTTP port first.
func ScheduleServiceStart() error {
	switch runtime.GOOS {
	case "darwin":
		path, err := launchAgentPath()
		if err != nil {
			return err
		}
		uid := strconv.Itoa(os.Getuid())
		script := "sleep 0.6; launchctl bootstrap gui/" + uid + " " + shellQuote(path) + " >/dev/null 2>&1 || true; launchctl kickstart -k gui/" + uid + "/" + serviceName
		return exec.Command("/bin/sh", "-c", script).Start()
	case "linux":
		return exec.Command("/bin/sh", "-c", "sleep 0.6; systemctl --user start alx.service").Start()
	case "windows":
		script := "Start-Sleep -Milliseconds 600; schtasks.exe /Run /TN 'ALemonX'"
		return exec.Command("powershell.exe", "-NoProfile", "-WindowStyle", "Hidden", "-Command", script).Start()
	default:
		return fmt.Errorf("暂不支持在 %s 上注册后台服务", runtime.GOOS)
	}
}

func hasPortArgument(arguments []string) bool {
	for index, argument := range arguments {
		if argument == "--port" && index+1 < len(arguments) {
			return true
		}
		if strings.HasPrefix(argument, "--port=") {
			return true
		}
	}
	return false
}

func RestartService() (string, error) {
	if _, err := StopService(); err != nil {
		return "", err
	}
	return StartService()
}

func UninstallService() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		uid := strconv.Itoa(os.Getuid())
		_ = exec.Command("launchctl", "bootout", "gui/"+uid+"/"+serviceName).Run()
		path, err := launchAgentPath()
		if err != nil {
			return "", err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return "", err
		}
	case "linux":
		_ = exec.Command("systemctl", "--user", "disable", "--now", "alx.service").Run()
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if err := os.Remove(filepath.Join(home, ".config", "systemd", "user", "alx.service")); err != nil && !os.IsNotExist(err) {
			return "", err
		}
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	case "windows":
		_ = exec.Command("schtasks", "/Delete", "/TN", "ALemonX", "/F").Run()
	default:
		return "", fmt.Errorf("暂不支持在 %s 上管理后台服务", runtime.GOOS)
	}
	return "后台服务已移除。alx 命令文件仍保留，便于以后重新安装。", nil
}

// InstallService registers the current binary as a user-level background
// service bound to the given host.
func InstallService(port, host string) (string, error) {
	return installService(port, host, true)
}

func installService(port, host string, start bool) (string, error) {
	value, err := strconv.ParseUint(port, 10, 16)
	if err != nil || value == 0 {
		return "", errors.New("端口应为 1 到 65535 的数字")
	}
	if strings.TrimSpace(host) == "" {
		return "", errors.New("监听地址不能为空")
	}
	if strings.ContainsAny(host, " \t\"'&|;<>$`") {
		return "", errors.New("监听地址包含不安全的字符")
	}
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("无法定位当前程序：%w", err)
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	installed, note, err := installCommand(executable)
	if err != nil {
		return "", err
	}
	executable = installed
	var result string
	switch runtime.GOOS {
	case "darwin":
		result, err = installLaunchAgent(executable, host, port, start)
	case "linux":
		result, err = installSystemdUserService(executable, host, port, start)
	case "windows":
		result, err = installScheduledTask(executable, host, port, start)
	default:
		return "", fmt.Errorf("暂不支持在 %s 上注册后台服务", runtime.GOOS)
	}
	if err != nil {
		return "", err
	}
	return "alx 命令已安装到：" + executable + "\n" + note + "\n" + result, nil
}

func installCommand(source string) (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	directory := filepath.Join(home, ".local", "bin")
	if runtime.GOOS == "windows" {
		directory = filepath.Join(home, "AppData", "Local", "alx")
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", "", fmt.Errorf("无法创建 alx 命令目录：%w", err)
	}
	name := "alx"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	target := filepath.Join(directory, name)
	if filepath.Clean(source) != filepath.Clean(target) {
		input, err := os.Open(source)
		if err != nil {
			return "", "", err
		}
		defer input.Close()
		output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
		if err != nil {
			return "", "", fmt.Errorf("无法安装 alx 命令：%w", err)
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		if copyErr != nil {
			return "", "", copyErr
		}
		if closeErr != nil {
			return "", "", closeErr
		}
	}
	note := "现在可使用 alx open 打开引导。"
	if !pathContains(directory) {
		note = "请将 " + directory + " 加入 PATH 后，可直接使用 alx 命令。"
	}
	return target, note, nil
}

func pathContains(directory string) bool {
	for _, item := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.Clean(item) == filepath.Clean(directory) {
			return true
		}
	}
	return false
}

func OpenBrowser(port string) error {
	url := "http://127.0.0.1:" + port
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", url)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("无法打开浏览器：%w", err)
	}
	return nil
}

func installLaunchAgent(executable, host, port string, start bool) (string, error) {
	path, err := launchAgentPath()
	if err != nil {
		return "", err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	logs := filepath.Join(home, "Library", "Logs", "alx.log")
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>Label</key><string>%s</string><key>ProgramArguments</key><array><string>%s</string><string>serve</string><string>--port</string><string>%s</string><string>--host</string><string>%s</string></array><key>RunAtLoad</key><true/><key>KeepAlive</key><true/><key>StandardOutPath</key><string>%s</string><key>StandardErrorPath</key><string>%s</string></dict></plist>
`, serviceName, xmlEscape(executable), xmlEscape(port), xmlEscape(host), xmlEscape(logs), xmlEscape(logs))
	if err := os.WriteFile(path, []byte(plist), 0644); err != nil {
		return "", err
	}
	if !start {
		return "已注册后台服务；当前前台实例关闭后会自动启动。", nil
	}
	uid := strconv.Itoa(os.Getuid())
	_ = exec.Command("launchctl", "bootout", "gui/"+uid+"/"+serviceName).Run()
	if output, err := exec.Command("launchctl", "bootstrap", "gui/"+uid, path).CombinedOutput(); err != nil {
		return "", fmt.Errorf("注册 LaunchAgent 失败：%s", strings.TrimSpace(string(output)))
	}
	if output, err := exec.Command("launchctl", "kickstart", "-k", "gui/"+uid+"/"+serviceName).CombinedOutput(); err != nil {
		return "", fmt.Errorf("启动后台服务失败：%s", strings.TrimSpace(string(output)))
	}
	return "已注册后台服务。登录后会自动运行，访问地址：http://" + displayHost(host) + ":" + port, nil
}

func launchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", serviceName+".plist"), nil
}

func installSystemdUserService(executable, host, port string, start bool) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	directory := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(directory, "alx.service")
	content := fmt.Sprintf("[Unit]\nDescription=ALemonX\nStartLimitIntervalSec=120\nStartLimitBurst=5\n[Service]\nExecStart=%s serve --port %s --host %s\nRestart=on-failure\nRestartSec=3\nKillMode=process\n[Install]\nWantedBy=default.target\n", shellQuote(executable), port, host)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	if output, err := exec.Command("systemctl", "--user", "daemon-reload").CombinedOutput(); err != nil {
		return "", fmt.Errorf("刷新 systemd 配置失败：%s", strings.TrimSpace(string(output)))
	}
	arguments := []string{"--user", "enable", "alx.service"}
	if start {
		arguments = append(arguments[:2], append([]string{"--now"}, arguments[2:]...)...)
	}
	if output, err := exec.Command("systemctl", arguments...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("启动后台服务失败：%s", strings.TrimSpace(string(output)))
	}
	if !start {
		return "已注册 systemd 用户服务；当前前台实例关闭后会自动启动。", nil
	}
	return "已注册 systemd 用户服务，访问地址：http://" + displayHost(host) + ":" + port, nil
}

// ensureSystemdUserUnitKillMode upgrades an existing ALemonX user unit so a
// service restart no longer reaps detached plugin background processes.
// systemd's default KillMode=control-group kills the entire service cgroup,
// which includes NapCat/LLBot processes that the plugins started in their own
// process groups. KillMode=process keeps only the service process itself in
// scope for the restart.
func ensureSystemdUserUnitKillMode(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	text := string(data)
	if !strings.Contains(text, "RestartSec=") {
		text = strings.Replace(text, "Restart=on-failure", "Restart=on-failure\nRestartSec=3", 1)
	}
	if !strings.Contains(text, "StartLimitIntervalSec=") {
		text = strings.Replace(text, "Description=ALemonX", "Description=ALemonX\nStartLimitIntervalSec=120\nStartLimitBurst=5", 1)
	}
	if strings.Contains(text, "KillMode=process") {
		return os.WriteFile(path, []byte(text), 0644)
	}
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines)+1)
	inService := false
	inserted := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[Service]" {
			inService = true
		} else if strings.HasPrefix(trimmed, "[") {
			inService = false
		}
		out = append(out, line)
		if inService && !inserted && strings.HasPrefix(trimmed, "ExecStart=") {
			out = append(out, "KillMode=process")
			inserted = true
		}
	}
	if !inserted {
		return nil // Unrecognized layout; never corrupt a hand-written unit.
	}
	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0644)
}

func installScheduledTask(executable, host, port string, start bool) (string, error) {
	logs, err := serviceLogPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(logs), 0755); err != nil {
		return "", fmt.Errorf("无法创建服务日志目录：%w", err)
	}
	// The task scheduler does not retain an application's stdout/stderr. Run
	// through cmd.exe so `alx logs` can expose the same diagnostics as macOS
	// and Linux managed services.
	command := `cmd.exe /d /s /c ""` + executable + `" serve --port ` + port + ` --host ` + host + ` >> "` + logs + `" 2>&1"`
	if output, err := exec.Command("schtasks", "/Create", "/TN", "ALemonX", "/SC", "ONLOGON", "/TR", command, "/F").CombinedOutput(); err != nil {
		return "", fmt.Errorf("注册计划任务失败：%s", strings.TrimSpace(string(output)))
	}
	if !start {
		return "已注册登录启动任务；当前前台实例关闭后会自动启动。", nil
	}
	if output, err := exec.Command("schtasks", "/Run", "/TN", "ALemonX").CombinedOutput(); err != nil {
		return "", fmt.Errorf("启动后台服务失败：%s", strings.TrimSpace(string(output)))
	}
	return "已注册登录启动任务，访问地址：http://" + displayHost(host) + ":" + port, nil
}

// displayHost presents a wildcard bind address as the local loopback address
// in user-facing messages.
func displayHost(host string) string {
	if host == "0.0.0.0" || host == "::" {
		return "127.0.0.1"
	}
	return host
}

func xmlEscape(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(value)
}
func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", `"'"'`) + "'" }
