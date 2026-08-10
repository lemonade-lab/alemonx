package system

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// NapCatAPTDependencyAction is intentionally host-owned. Setup plugins cannot
// declare arbitrary commands that receive an administrator password.
const NapCatAPTDependencyAction = "napcat-install-dependencies"

// ErrSudoPasswordInvalid lets the web layer rate-limit only incorrect
// credentials. Missing sudo rights and package-manager failures are not
// password guesses and must remain retryable after the user fixes the cause.
var ErrSudoPasswordInvalid = errors.New("sudo 密码无效，请确认后重试")

var napCatAPTDependencies = []string{"xvfb", "libnss3", "libgbm1"}

// napcatAPTCommand keeps the entire privileged invocation in host code. It is
// deliberately not assembled from a plugin manifest, browser input, or shell
// string.
func napcatAPTCommand(ctx context.Context, password []byte) *exec.Cmd {
	args := []string{"-S", "-k", "-p", "", "--", "apt-get", "install", "-y"}
	args = append(args, napCatAPTDependencies...)
	command := exec.CommandContext(ctx, "sudo", args...)
	// Allocate a separate buffer so the newline is never retained in the
	// caller's password slice.
	stdin := make([]byte, len(password)+1)
	copy(stdin, password)
	stdin[len(password)] = '\n'
	command.Stdin = bytes.NewReader(stdin)
	return command
}

// sudoCommand is replaceable only by package tests. The production path never
// invokes a shell and never accepts command arguments from a browser or plugin.
var sudoCommand = func(ctx context.Context, password []byte) ([]byte, error) {
	command := napcatAPTCommand(ctx, password)
	return command.CombinedOutput()
}

// InstallNapCatAPTDependencies installs the only root-level operation exposed
// by the workbench in this release. Password bytes are cleared before return.
func InstallNapCatAPTDependencies(ctx context.Context, password []byte) (string, error) {
	defer clearSecret(password)
	if runtime.GOOS != "linux" {
		return "", errors.New("NapCat 系统依赖安装仅支持 Linux")
	}
	if len(password) == 0 {
		return "", errors.New("请输入当前系统账户的 sudo 密码")
	}
	if _, err := exec.LookPath("apt-get"); err != nil {
		return "", errors.New("当前 Linux 发行版未检测到 APT。首版仅支持 Debian/Ubuntu；请在终端安装 xvfb、libnss3 和 libgbm1 后重试")
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		return "", errors.New("当前系统未安装 sudo，无法使用工作台密码授权。请由系统管理员安装 sudo 或在终端完成依赖安装")
	}
	output, err := sudoCommand(ctx, password)
	text := strings.TrimSpace(string(output))
	if err == nil {
		return "✓ 已安装 NapCat Linux 系统依赖（xvfb、libnss3、libgbm1）。", nil
	}
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "sorry, try again"), strings.Contains(lower, "incorrect password"), strings.Contains(lower, "authentication failure"):
		return "", ErrSudoPasswordInvalid
	case strings.Contains(lower, "not in the sudoers"), strings.Contains(lower, "is not allowed to run sudo"):
		return "", errors.New("当前系统账户没有 sudo 权限，请联系系统管理员或在终端完成依赖安装")
	case strings.Contains(lower, "could not get lock"), strings.Contains(lower, "unable to acquire the dpkg frontend lock"), strings.Contains(lower, "dpkg was interrupted"):
		return "", errors.New("APT 正被其他软件包操作占用。请等待系统更新完成后重试")
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "", errors.New("安装系统依赖超时，请检查网络和 APT 状态后重试")
	default:
		return "", fmt.Errorf("安装 NapCat 系统依赖失败%s", limitedSudoDiagnostic(text))
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

// NapCatAPTTimeout is intentionally finite so a blocked package manager does
// not leave an operation task waiting forever.
const NapCatAPTTimeout = 10 * time.Minute
