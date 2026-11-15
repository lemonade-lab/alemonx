//go:build darwin || linux

package main

import (
	"errors"
	"syscall"
	"time"
)

func waitForDesktopExit(pid int) error {
	if pid <= 0 {
		return errors.New("桌面进程编号无效")
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("等待桌面应用退出超时")
}
