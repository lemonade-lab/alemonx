//go:build windows

package system

import (
	"context"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows Chromium builds can open a normal window for --version, including
// forwarding the launch to an existing browser. Read the file without running it.
func browserVersion(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var handle windows.Handle
	size, err := windows.GetFileVersionInfoSize(path, &handle)
	if err != nil {
		return "", err
	}
	if size == 0 {
		return "", fmt.Errorf("browser version resource is empty")
	}
	data := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&data[0])); err != nil {
		return "", err
	}
	var info *windows.VS_FIXEDFILEINFO
	var length uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&data[0]), `\`, unsafe.Pointer(&info), &length); err != nil {
		return "", err
	}
	if info == nil || length < uint32(unsafe.Sizeof(*info)) || info.Signature != 0xFEEF04BD {
		return "", fmt.Errorf("browser version resource is invalid")
	}
	return fmt.Sprintf("%d.%d.%d.%d", info.FileVersionMS>>16, info.FileVersionMS&0xffff,
		info.FileVersionLS>>16, info.FileVersionLS&0xffff), nil
}
