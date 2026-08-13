package system

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// OpenDesktopTarget opens an explicit local file/directory or HTTP(S) URL in
// the operating system's normal desktop handler. It never invokes a shell.
func OpenDesktopTarget(target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return errors.New("请选择要打开的文件、目录或链接")
	}
	if parsed, err := url.Parse(target); err == nil && parsed.Scheme != "" {
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return errors.New("仅支持打开 HTTP 或 HTTPS 链接")
		}
	} else {
		absolute, err := filepath.Abs(target)
		if err != nil {
			return errors.New("打开目标无效")
		}
		if _, err := os.Stat(absolute); err != nil {
			return fmt.Errorf("打开目标不存在：%w", err)
		}
		target = absolute
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", target)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("无法打开目标：%w", err)
	}
	return nil
}

func DesktopClipboardAvailable() bool {
	for _, command := range desktopClipboardCommands(false) {
		if _, err := exec.LookPath(command[0]); err == nil {
			return true
		}
	}
	return false
}

func ReadDesktopClipboard() (string, error) {
	for _, value := range desktopClipboardCommands(false) {
		if _, err := exec.LookPath(value[0]); err == nil {
			output, runErr := exec.Command(value[0], value[1:]...).Output()
			if runErr != nil {
				return "", runErr
			}
			return string(output), nil
		}
	}
	return "", errors.New("当前系统没有可用的剪贴板服务")
}

func WriteDesktopClipboard(text string) error {
	for _, value := range desktopClipboardCommands(true) {
		if _, err := exec.LookPath(value[0]); err == nil {
			command := exec.Command(value[0], value[1:]...)
			command.Stdin = bytes.NewBufferString(text)
			return command.Run()
		}
	}
	return errors.New("当前系统没有可用的剪贴板服务")
}

func desktopClipboardCommands(write bool) [][]string {
	switch runtime.GOOS {
	case "darwin":
		if write {
			return [][]string{{"pbcopy"}}
		}
		return [][]string{{"pbpaste"}}
	case "windows":
		if write {
			return [][]string{{"clip"}}
		}
		return [][]string{{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "Get-Clipboard -Raw"}}
	default:
		if write {
			return [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}}
		}
		return [][]string{{"wl-paste", "--no-newline"}, {"xclip", "-selection", "clipboard", "-o"}}
	}
}

func SendDesktopNotification(title, message string) error {
	title, message = strings.TrimSpace(title), strings.TrimSpace(message)
	if title == "" {
		title = "AlemonX"
	}
	if len(title) > 120 || len(message) > 1000 {
		return errors.New("通知内容过长")
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("osascript", "-e", "display notification "+appleScriptQuote(message)+" with title "+appleScriptQuote(title))
	case "linux":
		command = exec.Command("notify-send", title, message)
	default:
		return errors.New("当前平台暂未提供系统通知服务")
	}
	if err := command.Run(); err != nil {
		return fmt.Errorf("发送系统通知失败：%w", err)
	}
	return nil
}

func appleScriptQuote(value string) string {
	return "\"" + strings.ReplaceAll(strings.ReplaceAll(value, "\\", "\\\\"), "\"", "\\\"") + "\""
}
