//go:build darwin

package robot

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// Darwin's Unix socket address has 104 bytes including the terminating NUL.
const pm2DarwinSocketLimit = 103

func applyPM2SocketEnvironment(cmd *exec.Cmd) error {
	env := cmd.Environ()
	values := make(map[string]string)
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		values[key] = value
	}
	home := values["PM2_HOME"]
	if home == "" {
		userHome := values["HOME"]
		if userHome == "" {
			var err error
			userHome, err = os.UserHomeDir()
			if err != nil {
				return nil
			}
		}
		home = filepath.Join(userHome, ".pm2")
	}
	if !filepath.IsAbs(home) {
		home = filepath.Join(cmd.Dir, home)
	}
	for key, name := range map[string]string{
		"PM2_DAEMON_RPC_PORT": "rpc.sock", "PM2_DAEMON_PUB_PORT": "pub.sock",
		"PM2_INTERACTOR_RPC_PORT": "interactor.sock",
	} {
		socket := values[key]
		if socket == "" {
			socket = filepath.Join(home, name)
		} else if !filepath.IsAbs(socket) {
			socket = filepath.Join(cmd.Dir, socket)
		}
		if len(socket) <= pm2DarwinSocketLimit {
			continue
		}
		alias, err := shortPM2Socket(socket)
		if err != nil {
			return fmt.Errorf("无法缩短 PM2 socket 路径：%w", err)
		}
		env = append(env, key+"="+alias)
	}
	cmd.Env = env
	return nil
}

func shortPM2Socket(socket string) (string, error) {
	directory, name := filepath.Dir(socket), filepath.Base(socket)
	// Node/libuv can already be running on a truncated address. Keep using
	// that socket when present so switching runtimes reaches the same daemon.
	if full, err := os.Lstat(socket); err != nil || full.Mode()&os.ModeSocket == 0 {
		// Older libuv builds can use all 104 bytes without a trailing NUL.
		for _, limit := range []int{pm2DarwinSocketLimit + 1, pm2DarwinSocketLimit} {
			if len(socket) <= limit {
				continue
			}
			truncated := socket[:limit]
			if filepath.Dir(truncated) != directory {
				continue
			}
			if info, err := os.Lstat(truncated); err == nil && info.Mode()&os.ModeSocket != 0 {
				name = filepath.Base(truncated)
				break
			}
		}
	}
	// Keep aliases available for the daemon and future workbench processes.
	// Use a private directory so another account cannot replace an alias.
	base := fmt.Sprintf("/tmp/alx-pm2-%d", os.Getuid())
	if err := os.Mkdir(base, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	if !ownedPrivatePM2Directory(base) {
		return "", fmt.Errorf("PM2 socket 缩短目录不可用：%s", base)
	}
	digest := sha256.Sum256([]byte(directory))
	link := filepath.Join(base, fmt.Sprintf("%x", digest[:12]))
	if err := os.Symlink(directory, link); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	if destination, err := os.Readlink(link); err != nil || destination != directory {
		return "", errors.New("PM2 socket 路径别名与目标目录不一致")
	}
	alias := filepath.Join(link, name)
	if len(alias) > pm2DarwinSocketLimit {
		return "", errors.New("PM2 socket 文件名过长")
	}
	return alias, nil
}

func ownedPrivatePM2Directory(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return false
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	return ok && owner.Uid == uint32(os.Getuid())
}
