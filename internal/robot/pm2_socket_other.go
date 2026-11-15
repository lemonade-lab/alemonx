//go:build !darwin

package robot

import "os/exec"

func applyPM2SocketEnvironment(*exec.Cmd) error { return nil }
