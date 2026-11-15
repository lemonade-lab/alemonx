package main

import (
	"errors"

	"golang.org/x/sys/windows"
)

func waitForDesktopExit(pid int) error {
	if pid <= 0 {
		return errors.New("桌面进程编号无效")
	}
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return nil
	}
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	result, err := windows.WaitForSingleObject(process, 30000)
	if err != nil {
		return err
	}
	if result != windows.WAIT_OBJECT_0 {
		return errors.New("等待桌面应用退出超时")
	}
	return nil
}
