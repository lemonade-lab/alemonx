package system

import (
	"context"
	"errors"
	"fmt"
	"io"
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

// These lists are reviewed host policy, not plugin or browser input. They
// cover the Linux QQ runtime, the NapCat Shell files and the user-space
// launcher managed by the Go runner.
var napCatAPTDependencies = []string{
	"xvfb",
	"libnss3", "libgbm1", "libglib2.0-0", "libatk1.0-0", "libatspi2.0-0", "libgtk-3-0", "libasound2",
}

var napCatDNFDependencies = []string{
	"xorg-x11-server-Xvfb",
	"nss", "mesa-libgbm", "glib2", "atk", "at-spi2-atk", "gtk3", "alsa-lib",
}

// sudoPasswordReader keeps the one-time password out of strings and wipes the
// temporary newline-terminated input buffer as soon as sudo has consumed it.
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

// napcatAPTCommand keeps the entire privileged invocation in host code. It is
// deliberately not assembled from a plugin manifest, browser input, or shell
// string.
func napcatAPTCommand(ctx context.Context, password []byte) *exec.Cmd {
	args := []string{"-S", "-k", "-p", "", "--", "apt-get", "install", "-y"}
	args = append(args, napCatAPTDependencies...)
	command := exec.CommandContext(ctx, "sudo", args...)
	command.Stdin = newSudoPasswordReader(password)
	return command
}

func napcatDNFCommand(ctx context.Context, password []byte) *exec.Cmd {
	args := []string{"-S", "-k", "-p", "", "--", "dnf", "install", "--allowerasing", "-y"}
	args = append(args, napCatDNFDependencies...)
	command := exec.CommandContext(ctx, "sudo", args...)
	command.Stdin = newSudoPasswordReader(password)
	return command
}

// sudoCommand is replaceable only by package tests. The production path never
// invokes a shell and never accepts command arguments from a browser or plugin.
var sudoCommand = func(ctx context.Context, password []byte) ([]byte, error) {
	command := napcatAPTCommand(ctx, password)
	if _, err := exec.LookPath("apt-get"); err != nil {
		command = napcatDNFCommand(ctx, password)
	}
	reader, _ := command.Stdin.(*sudoPasswordReader)
	if reader != nil {
		defer reader.clear()
	}
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
	if _, aptErr := exec.LookPath("apt-get"); aptErr != nil {
		if _, dnfErr := exec.LookPath("dnf"); dnfErr != nil {
			return "", errors.New("当前 Linux 未检测到 APT 或 DNF；工作台暂不能自动安装 NapCat 系统依赖")
		}
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		return "", errors.New("当前系统未安装 sudo，无法使用工作台密码授权。请由系统管理员安装 sudo 或在终端完成依赖安装")
	}
	output, err := sudoCommand(ctx, password)
	text := strings.TrimSpace(string(output))
	if err == nil {
		return "✓ 已安装 NapCat Linux 系统依赖，正在继续自动安装。", nil
	}
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "sorry, try again"), strings.Contains(lower, "incorrect password"), strings.Contains(lower, "authentication failure"):
		return "", ErrSudoPasswordInvalid
	case strings.Contains(lower, "not in the sudoers"), strings.Contains(lower, "is not allowed to run sudo"):
		return "", errors.New("当前系统账户没有 sudo 权限，请联系系统管理员或在终端完成依赖安装")
	case strings.Contains(lower, "could not get lock"), strings.Contains(lower, "unable to acquire the dpkg frontend lock"), strings.Contains(lower, "dpkg was interrupted"):
		return "", errors.New("系统包管理器正被其他操作占用。请等待系统更新完成后重试")
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "", errors.New("安装系统依赖超时，请检查网络和系统包管理器状态后重试")
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
