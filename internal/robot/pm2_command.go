package robot

import (
	"os/exec"
	"path/filepath"
	"strings"
)

func usesPM2Socket(cmd *exec.Cmd) bool {
	for index, arg := range cmd.Args {
		base := strings.ToLower(filepath.Base(arg))
		base = strings.TrimSuffix(strings.TrimSuffix(base, ".exe"), ".cmd")
		if base == "pm2" || strings.HasPrefix(base, "pm2@") {
			return true
		}
		if index > 0 && arg == "run" {
			for _, script := range cmd.Args[index+1:] {
				if strings.HasPrefix(script, "-") {
					continue
				}
				return script == "start" || script == "stop" || script == "delete"
			}
		}
	}
	return false
}
