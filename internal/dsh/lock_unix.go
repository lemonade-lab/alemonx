//go:build !windows

package dsh

import (
	"golang.org/x/sys/unix"
	"os"
)

func lockRuntime(file *os.File) error { return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB) }
func unlockRuntime(file *os.File)     { _ = unix.Flock(int(file.Fd()), unix.LOCK_UN); _ = file.Close() }
