// Package system provides small cross-platform integrations for the setup app.
package system

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const serviceName = "com.alemonjs.alx"

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
	return installService(port, false)
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

// InstallService registers the current binary as a user-level background service.
func InstallService(port string) (string, error) {
	return installService(port, true)
}

func installService(port string, start bool) (string, error) {
	value, err := strconv.ParseUint(port, 10, 16)
	if err != nil || value == 0 {
		return "", errors.New("端口应为 1 到 65535 的数字")
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
		result, err = installLaunchAgent(executable, port, start)
	case "linux":
		result, err = installSystemdUserService(executable, port, start)
	case "windows":
		result, err = installScheduledTask(executable, port, start)
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

func installLaunchAgent(executable, port string, start bool) (string, error) {
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
<plist version="1.0"><dict><key>Label</key><string>%s</string><key>ProgramArguments</key><array><string>%s</string><string>serve</string><string>--port</string><string>%s</string></array><key>RunAtLoad</key><true/><key>KeepAlive</key><true/><key>StandardOutPath</key><string>%s</string><key>StandardErrorPath</key><string>%s</string></dict></plist>
`, serviceName, xmlEscape(executable), xmlEscape(port), xmlEscape(logs), xmlEscape(logs))
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
	return "已注册后台服务。登录后会自动运行，访问地址：http://127.0.0.1:" + port, nil
}

func launchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", serviceName+".plist"), nil
}

func installSystemdUserService(executable, port string, start bool) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	directory := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(directory, "alx.service")
	content := fmt.Sprintf("[Unit]\nDescription=ALemonX\n[Service]\nExecStart=%s serve --port %s\nRestart=on-failure\n[Install]\nWantedBy=default.target\n", shellQuote(executable), port)
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
	return "已注册 systemd 用户服务，访问地址：http://127.0.0.1:" + port, nil
}

func installScheduledTask(executable, port string, start bool) (string, error) {
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
	command := `cmd.exe /d /s /c ""` + executable + `" serve --port ` + port + ` >> "` + logs + `" 2>&1"`
	if output, err := exec.Command("schtasks", "/Create", "/TN", "ALemonX", "/SC", "ONLOGON", "/TR", command, "/F").CombinedOutput(); err != nil {
		return "", fmt.Errorf("注册计划任务失败：%s", strings.TrimSpace(string(output)))
	}
	if !start {
		return "已注册登录启动任务；当前前台实例关闭后会自动启动。", nil
	}
	if output, err := exec.Command("schtasks", "/Run", "/TN", "ALemonX").CombinedOutput(); err != nil {
		return "", fmt.Errorf("启动后台服务失败：%s", strings.TrimSpace(string(output)))
	}
	return "已注册登录启动任务，访问地址：http://127.0.0.1:" + port, nil
}

func xmlEscape(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(value)
}
func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", `"'"'`) + "'" }
