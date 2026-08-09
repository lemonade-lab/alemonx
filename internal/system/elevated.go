package system

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// RunWithPrivileges runs one already-approved local operation with the
// operating system's native administrator prompt.  It never changes ownership
// or stores credentials: every call requires a new authorization.
func RunWithPrivileges(directory string, values map[string]string, name string, args ...string) (string, error) {
	script := privilegedScript(directory, values, name, args)
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		// osascript delegates authentication to macOS.  The command is built
		// from individually quoted arguments rather than user-provided shell.
		statement := "do shell script " + strconv.Quote(script) + " with administrator privileges"
		command = exec.Command("osascript", "-e", statement)
	case "linux":
		// pkexec is the standard Polkit entry point on desktop Linux.
		command = exec.Command("pkexec", "/bin/sh", "-lc", script)
	case "windows":
		return runWindowsElevated(directory, values, name, args...)
	default:
		return "", fmt.Errorf("当前系统不支持权限提升：%s", runtime.GOOS)
	}
	output, err := command.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if runtime.GOOS == "linux" && errors.Is(err, exec.ErrNotFound) {
			return "此 Linux 系统没有可用的 pkexec 权限服务。请将机器人项目放到当前用户拥有的目录（例如 ~/alemonjs），或安装并启用 polkit/pkexec 后重试。", fmt.Errorf("Linux 缺少 pkexec：%w", err)
		}
		if text == "" {
			text = "用户取消了系统权限授权，或系统拒绝了本次提升。"
		}
		return text, fmt.Errorf("需要管理员权限才能完成此操作：%w", err)
	}
	return text, nil
}

// RunWithPrivilegesInput executes one fixed program through the native
// administrator prompt while preserving the plugin's stdin/stdout JSON
// contract. The caller creates neither a shell command nor arbitrary args.
func RunWithPrivilegesInput(directory, name string, args []string, input []byte) ([]byte, error) {
	inputFile, err := os.CreateTemp("", "alx-elevated-input-*.json")
	if err != nil {
		return nil, fmt.Errorf("无法准备权限操作：%w", err)
	}
	inputPath := inputFile.Name()
	defer os.Remove(inputPath)
	if _, err := inputFile.Write(input); err != nil {
		_ = inputFile.Close()
		return nil, fmt.Errorf("无法准备权限操作：%w", err)
	}
	if err := inputFile.Close(); err != nil {
		return nil, fmt.Errorf("无法准备权限操作：%w", err)
	}
	outputFile, err := os.CreateTemp("", "alx-elevated-output-*.json")
	if err != nil {
		return nil, fmt.Errorf("无法准备权限操作：%w", err)
	}
	outputPath := outputFile.Name()
	if err := outputFile.Close(); err != nil {
		return nil, fmt.Errorf("无法准备权限操作：%w", err)
	}
	defer os.Remove(outputPath)
	errorFile, err := os.CreateTemp("", "alx-elevated-error-*.txt")
	if err != nil {
		return nil, fmt.Errorf("无法准备权限操作：%w", err)
	}
	errorPath := errorFile.Name()
	if err := errorFile.Close(); err != nil {
		return nil, fmt.Errorf("无法准备权限操作：%w", err)
	}
	defer os.Remove(errorPath)

	var runErr error
	switch runtime.GOOS {
	case "darwin":
		script := privilegedIOScript(directory, name, args, inputPath, outputPath, errorPath)
		statement := "do shell script " + strconv.Quote(script) + " with administrator privileges"
		_, runErr = exec.Command("osascript", "-e", statement).CombinedOutput()
	case "linux":
		script := privilegedIOScript(directory, name, args, inputPath, outputPath, errorPath)
		_, runErr = exec.Command("pkexec", "/bin/sh", "-lc", script).CombinedOutput()
	case "windows":
		runErr = runWindowsElevatedIO(directory, name, args, inputPath, outputPath, errorPath)
	default:
		return nil, fmt.Errorf("当前系统不支持权限提升：%s", runtime.GOOS)
	}
	output, readErr := os.ReadFile(outputPath)
	if readErr != nil {
		return nil, fmt.Errorf("无法读取权限操作结果：%w", readErr)
	}
	if runErr != nil {
		diagnostics, _ := os.ReadFile(errorPath)
		message := strings.TrimSpace(string(diagnostics))
		if message == "" {
			message = "用户取消了系统权限授权，或系统拒绝了本次提升。"
		}
		return output, fmt.Errorf("需要管理员权限才能完成此操作：%s", message)
	}
	return output, nil
}

func privilegedIOScript(directory, name string, args []string, inputPath, outputPath, errorPath string) string {
	parts := []string{"cd -- " + shellQuote(directory), "exec", shellQuote(name)}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ") + " < " + shellQuote(inputPath) + " > " + shellQuote(outputPath) + " 2> " + shellQuote(errorPath)
}

func runWindowsElevatedIO(directory, name string, args []string, inputPath, outputPath, errorPath string) error {
	scriptFile, err := os.CreateTemp("", "alx-elevated-plugin-*.ps1")
	if err != nil {
		return err
	}
	scriptPath := scriptFile.Name()
	defer os.Remove(scriptPath)
	arguments := make([]string, len(args))
	for index, arg := range args {
		arguments[index] = powershellQuote(arg)
	}
	lines := []string{"$ErrorActionPreference = 'Stop'"}
	if directory != "" {
		lines = append(lines, "Set-Location -LiteralPath "+powershellQuote(directory))
	}
	lines = append(lines,
		"Get-Content -Raw -LiteralPath "+powershellQuote(inputPath)+" | & "+powershellQuote(name)+" @( "+strings.Join(arguments, ",")+" ) 1> "+powershellQuote(outputPath)+" 2> "+powershellQuote(errorPath),
		"exit $LASTEXITCODE",
	)
	if _, err := scriptFile.WriteString(strings.Join(lines, "\r\n")); err != nil {
		_ = scriptFile.Close()
		return err
	}
	if err := scriptFile.Close(); err != nil {
		return err
	}
	launcher := "$p = Start-Process -FilePath 'powershell.exe' -ArgumentList @('-NoProfile','-ExecutionPolicy','Bypass','-File'," + powershellQuote(scriptPath) + ") -Verb RunAs -Wait -PassThru; exit $p.ExitCode"
	return exec.Command("powershell.exe", "-NoProfile", "-Command", launcher).Run()
}

func runWindowsElevated(directory string, values map[string]string, name string, args ...string) (string, error) {
	outputFile, err := os.CreateTemp("", "alx-elevated-output-*.txt")
	if err != nil {
		return "", fmt.Errorf("无法准备权限操作：%w", err)
	}
	outputPath := outputFile.Name()
	defer os.Remove(outputPath)
	if err := outputFile.Chmod(0666); err != nil {
		return "", fmt.Errorf("无法准备权限操作：%w", err)
	}
	if err := outputFile.Close(); err != nil {
		return "", fmt.Errorf("无法准备权限操作：%w", err)
	}
	scriptFile, err := os.CreateTemp("", "alx-elevated-command-*.ps1")
	if err != nil {
		return "", fmt.Errorf("无法准备权限操作：%w", err)
	}
	scriptPath := scriptFile.Name()
	defer os.Remove(scriptPath)
	if _, err := scriptFile.WriteString(windowsScript(directory, values, name, args, outputPath)); err != nil {
		return "", fmt.Errorf("无法准备权限操作：%w", err)
	}
	if err := scriptFile.Close(); err != nil {
		return "", fmt.Errorf("无法准备权限操作：%w", err)
	}
	launcher := "$p = Start-Process -FilePath 'powershell.exe' -ArgumentList @('-NoProfile','-ExecutionPolicy','Bypass','-File'," + powershellQuote(scriptPath) + ") -Verb RunAs -Wait -PassThru; exit $p.ExitCode"
	command := exec.Command("powershell.exe", "-NoProfile", "-Command", launcher)
	launcherOutput, runErr := command.CombinedOutput()
	data, readErr := os.ReadFile(outputPath)
	text := strings.TrimSpace(string(data))
	if text == "" {
		text = strings.TrimSpace(string(launcherOutput))
	}
	if runErr != nil {
		if text == "" {
			text = "用户取消了 Windows UAC 授权，或系统拒绝了本次提升。"
		}
		return text, fmt.Errorf("需要管理员权限才能完成此操作：%w", runErr)
	}
	if readErr != nil {
		return "", fmt.Errorf("无法读取提升操作输出：%w", readErr)
	}
	return text, nil
}

func windowsScript(directory string, values map[string]string, name string, args []string, outputPath string) string {
	lines := []string{"$ErrorActionPreference = 'Stop'"}
	if directory != "" {
		lines = append(lines, "Set-Location -LiteralPath "+powershellQuote(directory))
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		lines = append(lines, "$env:"+key+" = "+powershellQuote(values[key]))
	}
	arguments := make([]string, len(args))
	for index, arg := range args {
		arguments[index] = powershellQuote(arg)
	}
	lines = append(lines,
		"$output = & "+powershellQuote(name)+" @("+strings.Join(arguments, ",")+") 2>&1",
		"$exitCode = $LASTEXITCODE",
		"[System.IO.File]::WriteAllText("+powershellQuote(outputPath)+", ($output | Out-String -Width 4096))",
		"exit $exitCode",
	)
	return strings.Join(lines, "\r\n")
}

func powershellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func privilegedScript(directory string, values map[string]string, name string, args []string) string {
	parts := make([]string, 0, len(args)+len(values)+4)
	if directory != "" {
		parts = append(parts, "cd -- "+shellQuote(directory), "exec")
	}
	parts = append(parts, "env")
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, shellQuote(key+"="+values[key]))
	}
	parts = append(parts, shellQuote(name))
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}
