//go:build windows

package robot

import "os"

func fileIdentity(os.FileInfo) (int64, uint64) {
	return 0, 0
}
