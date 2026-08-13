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
)

const defaultServiceLogLines = 200
const maxServiceLogLines = 10000

// StreamServiceLogs writes recent managed-service logs to output. Follow keeps
// the command attached to the platform log stream until the caller interrupts
// it (Ctrl+C). Foreground launches intentionally keep writing to their own
// terminal and therefore have no persisted history to display here.
func StreamServiceLogs(lines int, follow bool, output io.Writer) error {
	var err error
	lines, err = validatedServiceLogLines(lines)
	if err != nil {
		return err
	}
	command, err := serviceLogCommand(lines, follow)
	if err != nil {
		return err
	}
	command.Stdout = output
	command.Stderr = output
	return command.Run()
}

func validatedServiceLogLines(lines int) (int, error) {
	if lines <= 0 {
		return defaultServiceLogLines, nil
	}
	if lines > maxServiceLogLines {
		return 0, fmt.Errorf("日志行数不能超过 %d", maxServiceLogLines)
	}
	return lines, nil
}

func serviceLogCommand(lines int, follow bool) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "darwin":
		path, err := serviceLogPath()
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return nil, errors.New("尚未生成后台服务日志；请先启动 AlemonX 后台服务。前台运行日志仅显示在启动它的终端中")
			}
			return nil, err
		}
		args := []string{"-n", strconv.Itoa(lines)}
		if follow {
			args = append(args, "-f")
		}
		args = append(args, path)
		return exec.Command("tail", args...), nil
	case "linux":
		args := []string{"--user", "--unit", "alx.service", "--lines", strconv.Itoa(lines), "--no-pager"}
		if follow {
			args = append(args, "--follow")
		}
		return exec.Command("journalctl", args...), nil
	case "windows":
		path, err := serviceLogPath()
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return nil, errors.New("尚未生成 Windows 后台服务日志；请重新安装并启动后台服务后再查看。前台运行日志仅显示在启动它的终端中")
			}
			return nil, err
		}
		script := "Get-Content -LiteralPath " + powershellQuote(path) + " -Tail " + strconv.Itoa(lines)
		if follow {
			script += " -Wait"
		}
		return exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script), nil
	default:
		return nil, fmt.Errorf("%s 暂无受管服务日志；前台运行日志显示在启动它的终端中", runtime.GOOS)
	}
}

func serviceLogPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Logs", "alx.log"), nil
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(base, "alx", "alx.log"), nil
	default:
		return "", errors.New("当前系统不使用文件式 AlemonX 服务日志")
	}
}
