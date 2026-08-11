package system

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// PickerKind is deliberately small: a system plugin may ask the host to let a
// user select files or directories, but cannot turn the workbench into an
// arbitrary command launcher.
type PickerKind string

const (
	PickerDirectory PickerKind = "directory"
	PickerFile      PickerKind = "file"
)

// PickerRequest is host-owned after manifest validation. Callers never pass a
// path, command, or shell expression to this API.
type PickerRequest struct {
	Kind     PickerKind
	Title    string
	Multiple bool
}

// Choose opens the operating system's native file or directory picker. It is
// intended for an interactive local desktop workbench, never a remote server.
func Choose(request PickerRequest) ([]string, error) {
	if request.Kind != PickerDirectory && request.Kind != PickerFile {
		return nil, errors.New("不支持的系统选择器类型")
	}
	title := strings.TrimSpace(request.Title)
	if title == "" {
		if request.Kind == PickerDirectory {
			title = "选择目录"
		} else {
			title = "选择文件"
		}
	}
	if len([]rune(title)) > 120 || strings.ContainsAny(title, "\x00\r\n") {
		return nil, errors.New("系统选择器标题无效")
	}
	command, err := pickerCommand(request.Kind, title, request.Multiple)
	if err != nil {
		return nil, err
	}
	output, err := command.Output()
	paths := strings.FieldsFunc(strings.TrimSpace(string(output)), func(r rune) bool { return r == '\n' || r == '\r' })
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("当前系统缺少可用的%s选择器", pickerKindLabel(request.Kind))
		}
		return nil, fmt.Errorf("未完成%s选择", pickerKindLabel(request.Kind))
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("未选择%s", pickerKindLabel(request.Kind))
	}
	return paths, nil
}

// ChooseDirectories remains for host callers that predate the typed picker
// API. New code should use Choose.
func ChooseDirectories() ([]string, error) {
	return Choose(PickerRequest{Kind: PickerDirectory, Title: "选择机器人目录", Multiple: true})
}

func pickerKindLabel(kind PickerKind) string {
	if kind == PickerDirectory {
		return "目录"
	}
	return "文件"
}

func pickerCommand(kind PickerKind, title string, multiple bool) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "darwin":
		chooser := "choose file"
		if kind == PickerDirectory {
			chooser = "choose folder"
		}
		// The title and mode are passed as argv rather than interpolated into
		// AppleScript, so a manifest string cannot change the script itself.
		script := "on run argv\nset chosenItems to " + chooser + " with prompt (item 1 of argv) with multiple selections allowed ((item 2 of argv) is \"true\")\nset paths to {}\nrepeat with chosenItem in chosenItems\nset end of paths to POSIX path of chosenItem\nend repeat\nset AppleScript's text item delimiters to linefeed\nreturn paths as text\nend run"
		return exec.Command("osascript", "-e", script, title, strconv.FormatBool(multiple)), nil
	case "windows":
		if kind == PickerDirectory {
			return exec.Command("powershell", "-NoProfile", "-Command", "Add-Type -AssemblyName System.Windows.Forms; $d=New-Object System.Windows.Forms.FolderBrowserDialog; if($d.ShowDialog() -eq 'OK'){[Console]::Write($d.SelectedPath)}"), nil
		}
		return exec.Command("powershell", "-NoProfile", "-Command", "Add-Type -AssemblyName System.Windows.Forms; $d=New-Object System.Windows.Forms.OpenFileDialog; $d.Multiselect=$args[0] -eq 'true'; if($d.ShowDialog() -eq 'OK'){[Console]::Write(($d.FileNames -join [Environment]::NewLine))}", strconv.FormatBool(multiple)), nil
	case "linux":
		arguments := []string{"--file-selection", "--title=" + title}
		if kind == PickerDirectory {
			arguments = append(arguments, "--directory")
		}
		if multiple {
			arguments = append(arguments, "--multiple", "--separator=\n")
		}
		return exec.Command("zenity", arguments...), nil
	default:
		return nil, fmt.Errorf("当前系统暂不支持%s选择", pickerKindLabel(kind))
	}
}
