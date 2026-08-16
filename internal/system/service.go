// Package system provides small cross-platform integrations for the setup app.
package system

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"alemonx/internal/workspace"
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
		// systemctl enable/disable manages the default.target.wants symlink;
		// the WantedBy line in the unit file stays in place either way.
		_, startupErr := os.Stat(filepath.Join(home, ".config", "systemd", "user", "default.target.wants", "alx.service"))
		startup := startupErr == nil
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
			if !startup {
				result.Summary = "服务未开启登录自启；异常退出仍会自动重启。启用无登录运行后，可在不登录的情况下开机启动。"
			} else {
				result.Summary = "服务会在登录后启动并在异常退出后重启；启用无登录运行后，重启或退出登录也会持续运行。"
			}
		}
		return result
	case "windows":
		startup := windowsScheduledTaskEnabled()
		summary := "已注册登录启动任务；建议使用 Docker 或系统服务承载需要无人值守运行的服务器部署。"
		if !startup {
			summary = "已注册登录启动任务，但当前处于禁用状态；可在服务设置中重新开启。"
		}
		return ServiceResilience{StartupEnabled: startup, Summary: summary}
	default:
		return ServiceResilience{Summary: "当前系统不支持工作台后台服务管理。"}
	}
}

// windowsScheduledTaskEnabled reports whether the ALemonX scheduled task is
// currently enabled. Query failures are treated as enabled so a localization
// or permission issue never makes the UI offer to re-enable an already
// registered startup task.
func windowsScheduledTaskEnabled() bool {
	output, err := exec.Command("schtasks", "/Query", "/TN", "ALemonX", "/FO", "LIST", "/V").CombinedOutput()
	if err != nil {
		return true
	}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Scheduled Task State:") {
			return strings.Contains(line, "Enabled")
		}
	}
	return true
}

// SetStartupEnabled toggles whether the installed background service starts
// automatically at login (macOS/Windows) or at login/boot (Linux). It never
// stops or starts the currently running service.
func SetStartupEnabled(enabled bool) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		path, err := launchAgentPath()
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", errors.New("尚未安装后台服务，请先安装后再设置开机自启")
		}
		text := string(data)
		if !strings.Contains(text, "<key>RunAtLoad</key>") {
			return "", errors.New("无法识别 LaunchAgent 的开机自启配置")
		}
		want := "<key>RunAtLoad</key><true/>"
		if !enabled {
			want = "<key>RunAtLoad</key><false/>"
		}
		if strings.Contains(text, want) {
			if enabled {
				return "开机自启已开启。", nil
			}
			return "开机自启已关闭。", nil
		}
		if enabled {
			text = strings.Replace(text, "<key>RunAtLoad</key><false/>", "<key>RunAtLoad</key><true/>", 1)
		} else {
			text = strings.Replace(text, "<key>RunAtLoad</key><true/>", "<key>RunAtLoad</key><false/>", 1)
		}
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			return "", err
		}
		if enabled {
			return "已开启登录自启：下次登录时 ALemonX 会自动启动。", nil
		}
		return "已关闭登录自启：下次登录时 ALemonX 不会自动启动，当前运行中的服务不受影响。", nil
	case "linux":
		if !ServiceInstalled() {
			return "", errors.New("尚未安装后台服务，请先安装后再设置开机自启")
		}
		action, verb, done := "enable", "开启", "已开启登录自启；登录后 ALemonX 会自动启动。"
		if !enabled {
			action, verb, done = "disable", "关闭", "已关闭登录自启；下次登录时 ALemonX 不会自动启动。"
		}
		if output, err := exec.Command("systemctl", "--user", action, "alx.service").CombinedOutput(); err != nil {
			return "", fmt.Errorf("%s登录自启失败：%s", verb, strings.TrimSpace(string(output)))
		}
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
		return done, nil
	case "windows":
		if !ServiceInstalled() {
			return "", errors.New("尚未安装后台服务，请先安装后再设置开机自启")
		}
		flag, verb, done := "/ENABLE", "开启", "已开启登录自启；登录后 ALemonX 会自动启动。"
		if !enabled {
			flag, verb, done = "/DISABLE", "关闭", "已关闭登录自启；下次登录时 ALemonX 不会自动启动。"
		}
		if output, err := exec.Command("schtasks", "/Change", "/TN", "ALemonX", flag).CombinedOutput(); err != nil {
			return "", fmt.Errorf("%s登录自启失败：%s", verb, strings.TrimSpace(string(output)))
		}
		return done, nil
	default:
		return "", fmt.Errorf("暂不支持在 %s 上管理后台服务", runtime.GOOS)
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
			return "后台服务已安装，目前已停止。" + registrationStatusSuffix(), nil
		}
		return "后台服务运行中。" + registrationStatusSuffix(), nil
	case "linux":
		output, err := exec.Command("systemctl", "--user", "is-active", "alx.service").CombinedOutput()
		if err != nil {
			return "后台服务未运行（" + strings.TrimSpace(string(output)) + "）。" + registrationStatusSuffix(), nil
		}
		return "后台服务运行中。" + registrationStatusSuffix(), nil
	case "windows":
		output, err := exec.Command("schtasks", "/Query", "/TN", "ALemonX", "/FO", "LIST").CombinedOutput()
		if err != nil {
			return "未安装后台服务。运行 alx install 进行安装。", nil
		}
		return "后台服务已注册。\n" + strings.TrimSpace(string(output)) + registrationStatusSuffix(), nil
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
	if err := reconcileServiceRegistration(); err != nil {
		return "", err
	}
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
	workspaceRoot, err := workspace.ResolveRoot("")
	if err != nil {
		workspaceRoot = ""
	}
	return installService(port, "0.0.0.0", workspaceRoot, false)
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
	return "后台服务已移除。alx 程序文件仍保留，便于以后重新安装。", nil
}

// ServiceRegistration describes the currently installed service definition.
type ServiceRegistration struct {
	Executable string
	Port       string
	Host       string
	Workspace  string
}

// ReadRegistration parses the installed service definition. The second return
// value reports whether a service is installed at all.
func ReadRegistration() (ServiceRegistration, bool, error) {
	switch runtime.GOOS {
	case "darwin":
		path, err := launchAgentPath()
		if err != nil {
			return ServiceRegistration{}, false, err
		}
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			return ServiceRegistration{}, false, nil
		}
		if err != nil {
			return ServiceRegistration{}, false, err
		}
		arguments, ok := parsePlistProgramArguments(string(data))
		if !ok {
			return ServiceRegistration{}, false, errors.New("无法解析 LaunchAgent 参数")
		}
		return registrationFromArguments(arguments), true, nil
	case "linux":
		home, err := os.UserHomeDir()
		if err != nil {
			return ServiceRegistration{}, false, err
		}
		data, err := os.ReadFile(filepath.Join(home, ".config", "systemd", "user", "alx.service"))
		if errors.Is(err, os.ErrNotExist) {
			return ServiceRegistration{}, false, nil
		}
		if err != nil {
			return ServiceRegistration{}, false, err
		}
		arguments, ok := parseExecStartArgs(string(data))
		if !ok {
			return ServiceRegistration{}, false, errors.New("无法解析 systemd ExecStart")
		}
		return registrationFromArguments(arguments), true, nil
	case "windows":
		return windowsRegistration()
	default:
		return ServiceRegistration{}, false, nil
	}
}

func registrationFromArguments(arguments []string) ServiceRegistration {
	registration := ServiceRegistration{}
	if len(arguments) > 0 {
		registration.Executable = strings.Trim(arguments[0], `"`)
	}
	values := flagValues(arguments, "--port", "--host", "--workspace")
	registration.Port = values["--port"]
	registration.Host = values["--host"]
	registration.Workspace = values["--workspace"]
	return registration
}

func flagValues(arguments []string, flags ...string) map[string]string {
	values := map[string]string{}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		for _, flag := range flags {
			if argument == flag && index+1 < len(arguments) {
				values[flag] = strings.Trim(arguments[index+1], `"`)
				index++
				break
			}
			if strings.HasPrefix(argument, flag+"=") {
				values[flag] = strings.TrimPrefix(argument, flag+"=")
			}
		}
	}
	return values
}

var plistProgramArgumentsPattern = regexp.MustCompile(`<key>ProgramArguments</key><array>(.*?)</array>`)
var plistStringPattern = regexp.MustCompile(`<string>([^<]*)</string>`)

func parsePlistProgramArguments(text string) ([]string, bool) {
	match := plistProgramArgumentsPattern.FindStringSubmatch(text)
	if len(match) < 2 {
		return nil, false
	}
	items := plistStringPattern.FindAllStringSubmatch(match[1], -1)
	arguments := make([]string, 0, len(items))
	for _, item := range items {
		arguments = append(arguments, xmlUnescape(item[1]))
	}
	if len(arguments) == 0 {
		return nil, false
	}
	return arguments, true
}

func xmlUnescape(value string) string {
	return strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`).Replace(value)
}

func parseExecStartArgs(text string) ([]string, bool) {
	var line string
	for _, candidate := range strings.Split(text, "\n") {
		candidate = strings.TrimSpace(candidate)
		if strings.HasPrefix(candidate, "ExecStart=") {
			line = strings.TrimSpace(strings.TrimPrefix(candidate, "ExecStart="))
			break
		}
	}
	if line == "" {
		return nil, false
	}
	arguments := splitShellArgs(line)
	if len(arguments) == 0 {
		return nil, false
	}
	return arguments, true
}

// splitShellArgs splits a systemd ExecStart command line, honoring single
// quotes (the quoting style installSystemdUserService writes).
func splitShellArgs(line string) []string {
	var arguments []string
	var current strings.Builder
	inQuote := false
	for index := 0; index < len(line); index++ {
		switch line[index] {
		case '\'':
			inQuote = !inQuote
		case ' ', '\t':
			if inQuote {
				current.WriteByte(line[index])
			} else if current.Len() > 0 {
				arguments = append(arguments, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(line[index])
		}
	}
	if current.Len() > 0 {
		arguments = append(arguments, current.String())
	}
	return arguments
}

func windowsRegistration() (ServiceRegistration, bool, error) {
	output, err := exec.Command("schtasks", "/Query", "/TN", "ALemonX", "/FO", "LIST", "/V").CombinedOutput()
	if err != nil {
		return ServiceRegistration{}, false, nil
	}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Task To Run:") {
			continue
		}
		command := strings.TrimSpace(strings.TrimPrefix(line, "Task To Run:"))
		registration := ServiceRegistration{
			Executable: windowsTaskExecutable(command),
			Port:       commandFlagValue(command, "--port"),
			Host:       commandFlagValue(command, "--host"),
			Workspace:  commandFlagValue(command, "--workspace"),
		}
		return registration, true, nil
	}
	return ServiceRegistration{}, false, nil
}

var windowsTaskExecutablePattern = regexp.MustCompile(`""(.*?)" serve `)

func windowsTaskExecutable(command string) string {
	match := windowsTaskExecutablePattern.FindStringSubmatch(command)
	if len(match) < 2 {
		return ""
	}
	return strings.Trim(match[1], `"`)
}

func commandFlagValue(command, flag string) string {
	quoted := regexp.MustCompile(regexp.QuoteMeta(flag) + ` "([^"]*)"`).FindStringSubmatch(command)
	if len(quoted) == 2 {
		return quoted[1]
	}
	plain := regexp.MustCompile(regexp.QuoteMeta(flag) + ` ([^ "]+)`).FindStringSubmatch(command)
	if len(plain) == 2 {
		return plain[1]
	}
	return ""
}

// reconcileServiceRegistration keeps the registered service in sync with the
// binary the user is actually running: when the registered executable is
// missing or differs from the current one, the service definition is recreated
// with the current binary, keeping the registered port/host and workspace.
func reconcileServiceRegistration() error {
	registration, installed, err := ReadRegistration()
	if err != nil || !installed {
		return nil
	}
	current, err := os.Executable()
	if err != nil {
		return nil
	}
	if resolved, resolveErr := filepath.EvalSymlinks(current); resolveErr == nil {
		current = resolved
	}
	if filepath.Clean(current) == filepath.Clean(registration.Executable) {
		if _, statErr := os.Stat(registration.Executable); statErr != nil {
			return fmt.Errorf("服务注册的程序已不存在（%s）。如果程序已移动，请从新位置重新执行 alx install", registration.Executable)
		}
		return nil
	}
	workspaceRoot := registration.Workspace
	if workspaceRoot == "" {
		if resolved, resolveErr := workspace.ResolveRoot(""); resolveErr == nil {
			workspaceRoot = resolved
		}
	}
	if _, err := installService(registration.Port, registration.Host, workspaceRoot, false); err != nil {
		return fmt.Errorf("检测到程序位置变化，尝试按当前程序重新注册失败：%w", err)
	}
	return nil
}

func registrationStatusSuffix() string {
	registration, installed, err := ReadRegistration()
	if err != nil || !installed {
		return ""
	}
	lines := []string{"服务程序：" + registration.Executable}
	if registration.Workspace != "" {
		lines = append(lines, "工作区："+registration.Workspace)
	}
	if current, err := os.Executable(); err == nil {
		if resolved, resolveErr := filepath.EvalSymlinks(current); resolveErr == nil {
			current = resolved
		}
		if filepath.Clean(current) != filepath.Clean(registration.Executable) {
			lines = append(lines, "注意：当前运行的 alx 与注册程序不同（当前："+current+"）。如需更新注册，请从新位置重新执行 alx install。")
		}
	}
	return "\n" + strings.Join(lines, "\n")
}

// InstallService registers the binary the user is currently running as a
// user-level background service bound to the given host. The workspace root
// is pinned into the service command line so the service always uses the same
// working directory the user chose when installing.
func InstallService(port, host, workspaceRoot string) (string, error) {
	return installService(port, host, workspaceRoot, true)
}

func installService(port, host, workspaceRoot string, start bool) (string, error) {
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
	if strings.TrimSpace(workspaceRoot) != "" {
		if absolute, absErr := filepath.Abs(workspaceRoot); absErr == nil {
			workspaceRoot = filepath.Clean(absolute)
		}
	}
	var result string
	switch runtime.GOOS {
	case "darwin":
		result, err = installLaunchAgent(executable, host, port, workspaceRoot, start)
	case "linux":
		result, err = installSystemdUserService(executable, host, port, workspaceRoot, start)
	case "windows":
		result, err = installScheduledTask(executable, host, port, workspaceRoot, start)
	default:
		return "", fmt.Errorf("暂不支持在 %s 上注册后台服务", runtime.GOOS)
	}
	if err != nil {
		return "", err
	}
	message := "已注册后台服务。\n程序：" + executable
	if workspaceRoot != "" {
		message += "\n工作区：" + workspaceRoot
	}
	return message + "\n" + result, nil
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

func installLaunchAgent(executable, host, port, workspaceRoot string, start bool) (string, error) {
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
	arguments := []string{executable, "serve", "--port", port, "--host", host}
	if workspaceRoot != "" {
		arguments = append(arguments, "--workspace", workspaceRoot)
	}
	argumentStrings := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		argumentStrings = append(argumentStrings, "<string>"+xmlEscape(argument)+"</string>")
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>Label</key><string>%s</string><key>ProgramArguments</key><array>%s</array><key>RunAtLoad</key><true/><key>KeepAlive</key><true/><key>StandardOutPath</key><string>%s</string><key>StandardErrorPath</key><string>%s</string></dict></plist>
`, serviceName, strings.Join(argumentStrings, ""), xmlEscape(logs), xmlEscape(logs))
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

func installSystemdUserService(executable, host, port, workspaceRoot string, start bool) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	directory := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(directory, "alx.service")
	execStart := shellQuote(executable) + " serve --port " + shellQuote(port) + " --host " + shellQuote(host)
	if workspaceRoot != "" {
		execStart += " --workspace " + shellQuote(workspaceRoot)
	}
	content := fmt.Sprintf("[Unit]\nDescription=ALemonX\nStartLimitIntervalSec=120\nStartLimitBurst=5\n[Service]\nExecStart=%s\nRestart=on-failure\nRestartSec=3\nKillMode=process\n[Install]\nWantedBy=default.target\n", execStart)
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

func installScheduledTask(executable, host, port, workspaceRoot string, start bool) (string, error) {
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
	command := `cmd.exe /d /s /c ""` + executable + `" serve --port ` + port + ` --host ` + host
	if workspaceRoot != "" {
		command += ` --workspace "` + strings.ReplaceAll(workspaceRoot, `"`, `""`) + `"`
	}
	command += ` >> "` + logs + `" 2>&1"`
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
