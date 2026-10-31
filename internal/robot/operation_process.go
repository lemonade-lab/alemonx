package robot

import (
	"alemonx/internal/processutil"
	"os/exec"
)

func configureCancellableCommand(cmd *exec.Cmd) { processutil.ConfigureCancellableCommand(cmd) }
