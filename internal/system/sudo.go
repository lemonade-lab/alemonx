package system

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ErrSudoPasswordInvalid lets the host rate-limit only incorrect credentials.
// A plugin never receives password bytes; it only declares the fixed native
// command that the host may execute after an approved authorization intent.
var ErrSudoPasswordInvalid = errors.New("sudo 密码无效，请确认后重试")

// PrivilegedCommandTimeout bounds a native system operation without baking a
// product-specific package manager or dependency list into the host.
const PrivilegedCommandTimeout = 10 * time.Minute

type sudoPasswordReader struct {
	secret []byte
	offset int
}

func newSudoPasswordReader(password []byte) *sudoPasswordReader {
	secret := make([]byte, len(password)+1)
	copy(secret, password)
	secret[len(password)] = '\n'
	return &sudoPasswordReader{secret: secret}
}

func (reader *sudoPasswordReader) Read(target []byte) (int, error) {
	if reader.offset >= len(reader.secret) {
		reader.clear()
		return 0, io.EOF
	}
	count := copy(target, reader.secret[reader.offset:])
	reader.offset += count
	if reader.offset >= len(reader.secret) {
		reader.clear()
	}
	return count, nil
}

func (reader *sudoPasswordReader) clear() {
	for index := range reader.secret {
		reader.secret[index] = 0
	}
}

// sudoCommand is replaceable in tests. Production never invokes a shell and
// never reads program/arguments from the browser: callers supply values from
// an installed system-plugin manifest after host-side validation.
var sudoCommand = func(ctx context.Context, password []byte, program string, args []string) ([]byte, error) {
	commandArgs := []string{"-S", "-k", "-p", "", "--", program}
	commandArgs = append(commandArgs, args...)
	command := exec.CommandContext(ctx, "sudo", commandArgs...)
	reader := newSudoPasswordReader(password)
	command.Stdin = reader
	defer reader.clear()
	return command.CombinedOutput()
}

// RunSudoCommand executes one fixed, manifest-declared native command. It is
// intentionally generic: the host does not know whether a plugin installs
// graphics libraries, configures a service, or prepares another runtime.
func RunSudoCommand(ctx context.Context, password []byte, program string, args []string) (string, error) {
	defer clearSecret(password)
	program = strings.TrimSpace(program)
	if program == "" || strings.ContainsAny(program, "/\\\x00\r\n") {
		return "", errors.New("系统权限操作命令无效")
	}
	if _, err := exec.LookPath(program); err != nil {
		return "", fmt.Errorf("当前系统未找到所需命令 %s", program)
	}
	var (
		output []byte
		err    error
	)
	if os.Geteuid() == 0 {
		// The host already runs as root; sudo is neither required nor always
		// installed (common on minimal container images).
		output, err = exec.CommandContext(ctx, program, append([]string(nil), args...)...).CombinedOutput()
	} else {
		if len(password) == 0 {
			return "", errors.New("请输入当前系统账户的 sudo 密码")
		}
		if _, lookErr := exec.LookPath("sudo"); lookErr != nil {
			return "", errors.New("当前系统未安装 sudo，无法使用工作台密码授权")
		}
		output, err = sudoCommand(ctx, password, program, append([]string(nil), args...))
	}
	text := strings.TrimSpace(string(output))
	if err == nil {
		return "✓ 系统操作已完成。", nil
	}
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "sorry, try again"), strings.Contains(lower, "incorrect password"), strings.Contains(lower, "authentication failure"):
		return "", ErrSudoPasswordInvalid
	case strings.Contains(lower, "not in the sudoers"), strings.Contains(lower, "is not allowed to run sudo"):
		return "", errors.New("当前系统账户没有 sudo 权限")
	case strings.Contains(lower, "could not get lock"), strings.Contains(lower, "unable to acquire the dpkg frontend lock"), strings.Contains(lower, "dpkg was interrupted"):
		return "", errors.New("系统包管理器正被其他操作占用，请稍后重试")
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "", errors.New("系统操作超时，请检查网络和系统状态后重试")
	default:
		return "", fmt.Errorf("系统操作失败%s", limitedSudoDiagnostic(text))
	}
}

func clearSecret(secret []byte) {
	for index := range secret {
		secret[index] = 0
	}
}

func limitedSudoDiagnostic(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if len(text) > 500 {
		text = text[:500] + "…"
	}
	return "：" + text
}
