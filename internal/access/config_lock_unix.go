//go:build !windows

package access

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockConfigFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX)
}
