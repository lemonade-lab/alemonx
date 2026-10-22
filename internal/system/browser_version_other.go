//go:build !windows

package system

import (
	"context"
	"os/exec"
	"strings"
)

func browserVersion(ctx context.Context, path string) (string, error) {
	output, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(strings.Split(normalizeCommandOutput(output), "\n")[0]), nil
}
