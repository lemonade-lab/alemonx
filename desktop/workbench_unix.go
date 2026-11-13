//go:build !windows

package main

import "os/exec"

func configureWorkbenchProcess(*exec.Cmd) {}
