//go:build !windows

package resources

import (
	"golang.org/x/sys/unix"
	"os"
)

func lockDSHPackage(file *os.File) error { return unix.Flock(int(file.Fd()), unix.LOCK_EX) }
func unlockDSHPackage(file *os.File)     { _ = unix.Flock(int(file.Fd()), unix.LOCK_UN) }
