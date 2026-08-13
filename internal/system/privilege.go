package system

import (
	"errors"
	"os"
	"strings"
	"sync"
)

// PrivilegedMode controls whether the generic system-plugin authorization
// broker may show a platform-native prompt. It deliberately contains no
// plugin IDs, actions, release tags, hashes, or product policy.
type PrivilegedMode string

const (
	PrivilegedModeEnabled  PrivilegedMode = "enabled"
	PrivilegedModeLocal    PrivilegedMode = "local"
	PrivilegedModeDisabled PrivilegedMode = "disabled"
)

type PrivilegeStatus struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`
	Reason  string `json:"reason,omitempty"`
	Version string `json:"policyVersion"`
}

var privilegeRuntime = struct {
	sync.RWMutex
	mode   PrivilegedMode
	reason string
}{mode: PrivilegedModeDisabled, reason: "尚未初始化系统权限模式"}

// ConfigurePrivilegedMode is process-wide because the workbench listener is
// configured before plugins are loaded. Enabled is the normal managed-server
// mode; local is an optional desktop-only hardening mode.
func ConfigurePrivilegedMode(bind string, production bool) error {
	requested := strings.ToLower(strings.TrimSpace(os.Getenv("ALX_PRIVILEGED_MODE")))
	if requested == "" {
		requested = string(PrivilegedModeEnabled)
	}
	mode := PrivilegedMode(requested)
	if mode != PrivilegedModeEnabled && mode != PrivilegedModeLocal && mode != PrivilegedModeDisabled {
		return errors.New("ALX_PRIVILEGED_MODE 仅支持 enabled、local 或 disabled")
	}
	if mode == PrivilegedModeLocal && !literalLoopback(bind) {
		return errors.New("ALX_PRIVILEGED_MODE=local 仅允许监听 127.0.0.1 或 ::1")
	}
	reason := ""
	if mode == PrivilegedModeDisabled {
		reason = "当前部署已禁用系统权限操作"
	}
	privilegeRuntime.Lock()
	privilegeRuntime.mode, privilegeRuntime.reason = mode, reason
	privilegeRuntime.Unlock()
	return nil
}

func literalLoopback(bind string) bool { return bind == "127.0.0.1" || bind == "::1" }

func CurrentPrivilegeStatus() PrivilegeStatus {
	privilegeRuntime.RLock()
	defer privilegeRuntime.RUnlock()
	return PrivilegeStatus{Enabled: privilegeRuntime.mode != PrivilegedModeDisabled, Mode: string(privilegeRuntime.mode), Reason: privilegeRuntime.reason, Version: "broker-v2"}
}
