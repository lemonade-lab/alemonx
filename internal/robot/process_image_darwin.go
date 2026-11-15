//go:build darwin

package robot

import (
	"bytes"
	"unsafe"

	"golang.org/x/sys/unix"
)

func processImagePath(pid int) string {
	const procInfoCallPIDInfo = 2
	const procPIDPathInfo = 11
	var path [4096]byte
	_, _, errno := unix.Syscall6(unix.SYS_PROC_INFO, procInfoCallPIDInfo, uintptr(pid), procPIDPathInfo, 0, uintptr(unsafe.Pointer(&path[0])), uintptr(len(path)))
	if errno != 0 {
		return ""
	}
	end := bytes.IndexByte(path[:], 0)
	if end < 0 {
		end = len(path)
	}
	return string(path[:end])
}
