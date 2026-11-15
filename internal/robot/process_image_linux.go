//go:build linux

package robot

import (
	"os"
	"strconv"
)

func processImagePath(pid int) string {
	path, _ := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	return path
}
