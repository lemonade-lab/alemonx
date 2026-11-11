//go:build !windows

package workspace

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockRobotList(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX)
}
