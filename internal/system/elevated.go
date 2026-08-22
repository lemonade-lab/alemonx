package system

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// RunWithPrivilegesInput starts alx itself through the operating system's
// native elevation UI. The elevated process then invokes the declared runner
// directly. No plugin shell script, redirection script, or browser-controlled
// command line is involved in the authorization path.
func RunWithPrivilegesInput(directory, name string, args []string, input []byte, environment []string) ([]byte, error) {
	inputPath, outputPath, errorPath, cleanup, err := privilegedIOFiles(input)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	environmentPath, cleanupEnvironment, err := privilegedEnvironmentFile(environment)
	if err != nil {
		return nil, err
	}
	defer cleanupEnvironment()
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("无法定位主应用权限助手：%w", err)
	}
	helperArgs := []string{"__alx-privileged-run", "--directory", directory, "--input", inputPath, "--output", outputPath, "--error", errorPath, "--program", name}
	if environmentPath != "" {
		helperArgs = append(helperArgs, "--environment", environmentPath)
	}
	helperArgs = append(helperArgs, "--")
	helperArgs = append(helperArgs, args...)
	if err := runElevatedHelper(executable, helperArgs); err != nil {
		return readPrivilegedResult(outputPath, errorPath, err)
	}
	output, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("无法读取权限操作结果：%w", err)
	}
	return output, nil
}

func privilegedEnvironmentFile(environment []string) (string, func(), error) {
	if len(environment) == 0 {
		return "", func() {}, nil
	}
	for _, value := range environment {
		if !validEnvironmentValue(value) {
			return "", nil, errors.New("权限操作环境变量无效")
		}
	}
	file, err := os.CreateTemp("", "alx-elevated-environment-*.json")
	if err != nil {
		return "", nil, fmt.Errorf("无法准备权限操作环境：%w", err)
	}
	path := file.Name()
	data, err := json.Marshal(environment)
	if err == nil {
		_, err = file.Write(data)
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return "", nil, fmt.Errorf("无法准备权限操作环境：%w", err)
	}
	return path, func() { _ = os.Remove(path) }, nil
}

func validEnvironmentValue(value string) bool {
	key, _, ok := strings.Cut(value, "=")
	if !ok || key == "" || strings.ContainsAny(value, "\x00\r\n") {
		return false
	}
	for index, char := range key {
		if char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || index > 0 && char >= '0' && char <= '9' {
			continue
		}
		return false
	}
	return true
}

func privilegedIOFiles(input []byte) (string, string, string, func(), error) {
	inputFile, err := os.CreateTemp("", "alx-elevated-input-*.json")
	if err != nil {
		return "", "", "", nil, fmt.Errorf("无法准备权限操作：%w", err)
	}
	inputPath := inputFile.Name()
	if _, err = inputFile.Write(input); err != nil {
		_ = inputFile.Close()
		_ = os.Remove(inputPath)
		return "", "", "", nil, fmt.Errorf("无法准备权限操作：%w", err)
	}
	if err = inputFile.Close(); err != nil {
		_ = os.Remove(inputPath)
		return "", "", "", nil, fmt.Errorf("无法准备权限操作：%w", err)
	}
	outputFile, err := os.CreateTemp("", "alx-elevated-output-*.json")
	if err != nil {
		_ = os.Remove(inputPath)
		return "", "", "", nil, fmt.Errorf("无法准备权限操作：%w", err)
	}
	outputPath := outputFile.Name()
	if err = outputFile.Close(); err != nil {
		_ = os.Remove(inputPath)
		_ = os.Remove(outputPath)
		return "", "", "", nil, fmt.Errorf("无法准备权限操作：%w", err)
	}
	errorFile, err := os.CreateTemp("", "alx-elevated-error-*.txt")
	if err != nil {
		_ = os.Remove(inputPath)
		_ = os.Remove(outputPath)
		return "", "", "", nil, fmt.Errorf("无法准备权限操作：%w", err)
	}
	errorPath := errorFile.Name()
	if err = errorFile.Close(); err != nil {
		_ = os.Remove(inputPath)
		_ = os.Remove(outputPath)
		_ = os.Remove(errorPath)
		return "", "", "", nil, fmt.Errorf("无法准备权限操作：%w", err)
	}
	return inputPath, outputPath, errorPath, func() {
		_ = os.Remove(inputPath)
		_ = os.Remove(outputPath)
		_ = os.Remove(errorPath)
	}, nil
}

func readPrivilegedResult(outputPath, errorPath string, runErr error) ([]byte, error) {
	output, _ := os.ReadFile(outputPath)
	diagnostics, _ := os.ReadFile(errorPath)
	message := strings.TrimSpace(string(diagnostics))
	if message == "" {
		message = "用户取消了系统权限授权，或系统拒绝了本次提升。"
	}
	return output, fmt.Errorf("需要管理员权限才能完成此操作：%s", message)
}

func runElevatedHelper(executable string, args []string) error {
	switch runtime.GOOS {
	case "darwin":
		// macOS has no public Go API for the administrator prompt. osascript
		// asks the native system UI to run this fixed alx helper; the plugin
		// runner is never interpolated into a shell script of its own.
		parts := make([]string, 0, len(args)+1)
		parts = append(parts, shellQuote(executable))
		for _, arg := range args {
			parts = append(parts, shellQuote(arg))
		}
		statement := "do shell script " + strconv.Quote(strings.Join(parts, " ")) + " with administrator privileges"
		_, err := exec.Command("osascript", "-e", statement).CombinedOutput()
		return err
	case "linux":
		if _, err := exec.LookPath("pkexec"); err != nil {
			return fmt.Errorf("Linux 缺少 pkexec 权限服务：%w", err)
		}
		commandArgs := append([]string{executable}, args...)
		command := exec.Command("pkexec", commandArgs...)
		_, err := command.CombinedOutput()
		return err
	case "windows":
		return runWindowsElevatedHelper(executable, args)
	default:
		return fmt.Errorf("当前系统不支持权限提升：%s", runtime.GOOS)
	}
}

func runWindowsElevatedHelper(executable string, args []string) error {
	quoted := make([]string, 0, len(args)+1)
	quoted = append(quoted, powershellQuote(executable))
	for _, arg := range args {
		quoted = append(quoted, powershellQuote(arg))
	}
	launcher := "$p = Start-Process -FilePath " + quoted[0] + " -ArgumentList @("
	if len(quoted) > 1 {
		launcher += strings.Join(quoted[1:], ",")
	}
	launcher += ") -Verb RunAs -Wait -PassThru; exit $p.ExitCode"
	return exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", launcher).Run()
}

func powershellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// RunPrivilegedHelper is called only by this executable's private
// __alx-privileged-run mode. Keeping the transport in Go makes native
// elevation deterministic and removes the former /bin/sh -lc bridge.
func RunPrivilegedHelper(arguments []string) int {
	flags := flag.NewFlagSet("__alx-privileged-run", flag.ContinueOnError)
	flags.SetOutput(ioDiscard{})
	directory := flags.String("directory", "", "")
	inputPath := flags.String("input", "", "")
	outputPath := flags.String("output", "", "")
	errorPath := flags.String("error", "", "")
	environmentPath := flags.String("environment", "", "")
	program := flags.String("program", "", "")
	if err := flags.Parse(arguments); err != nil || strings.TrimSpace(*inputPath) == "" || strings.TrimSpace(*outputPath) == "" || strings.TrimSpace(*errorPath) == "" || strings.TrimSpace(*program) == "" {
		return 2
	}
	input, err := os.ReadFile(*inputPath)
	if err != nil {
		_ = os.WriteFile(*errorPath, []byte(err.Error()), 0o600)
		return 1
	}
	command := exec.Command(*program, flags.Args()...)
	command.Dir = *directory
	if *environmentPath != "" {
		data, readErr := os.ReadFile(*environmentPath)
		var environment []string
		if readErr != nil || json.Unmarshal(data, &environment) != nil {
			_ = os.WriteFile(*errorPath, []byte("权限操作环境无效"), 0o600)
			return 1
		}
		for _, value := range environment {
			if !validEnvironmentValue(value) {
				_ = os.WriteFile(*errorPath, []byte("权限操作环境无效"), 0o600)
				return 1
			}
		}
		command.Env = append(os.Environ(), environment...)
	}
	command.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err = command.Run()
	_ = os.WriteFile(*outputPath, stdout.Bytes(), 0o600)
	if stderr.Len() > 0 {
		_ = os.WriteFile(*errorPath, stderr.Bytes(), 0o600)
	}
	if err != nil {
		if stderr.Len() == 0 {
			_ = os.WriteFile(*errorPath, []byte(err.Error()), 0o600)
		}
		return 1
	}
	return 0
}

// ioDiscard avoids exposing private helper parsing errors on a privileged
// terminal. The caller receives a structured failure through the result file.
type ioDiscard struct{}

func (ioDiscard) Write(value []byte) (int, error) { return len(value), nil }
