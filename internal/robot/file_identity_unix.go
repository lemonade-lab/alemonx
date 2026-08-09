//go:build !windows

package robot

import (
	"os"
	"syscall"
)

func fileIdentity(info os.FileInfo) (int64, uint64) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return 0, 0
	}
	return int64(stat.Dev), uint64(stat.Ino)
}
