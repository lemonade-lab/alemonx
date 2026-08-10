package system

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

//go:embed privilege_policy.json
var embeddedPrivilegePolicy []byte

// PrivilegedMode is deliberately process-wide: the listener is chosen before
// the HTTP server and plugin registry are created.
type PrivilegedMode string

const (
	PrivilegedModeLocal    PrivilegedMode = "local"
	PrivilegedModeDisabled PrivilegedMode = "disabled"
)

type PrivilegeStatus struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`
	Reason  string `json:"reason,omitempty"`
	Version string `json:"policyVersion"`
}

type privilegePolicyFile struct {
	Version    string               `json:"version"`
	Operations []privilegeOperation `json:"operations"`
}

// privilegeOperation is host policy, never downloaded plugin metadata.
// A future approved network release adds a complete record here in the alx
// repository before its runner can receive an administrator token.
type privilegeOperation struct {
	PluginID      string   `json:"pluginId"`
	Action        string   `json:"action"`
	Platform      string   `json:"platform"`
	Tag           string   `json:"tag"`
	Asset         string   `json:"asset"`
	ArchiveSHA256 string   `json:"archiveSha256"`
	RunnerSHA256  string   `json:"runnerSha256"`
	RunnerArgs    []string `json:"runnerArgs,omitempty"`
	Prompt        string   `json:"prompt"`
	Actions       []string `json:"actions,omitempty"`
}

// PluginPrivilegeIdentity contains only host-resolved installation data. No
// caller-supplied path, command, or privilege level is accepted.
type PluginPrivilegeIdentity struct {
	PluginID        string
	Action          string
	Tag             string
	Asset           string
	ArchiveSHA256   string
	RunnerPath      string
	RunnerArgs      []string
	DeclaredActions []string
}

var privilegeRuntime = struct {
	sync.RWMutex
	mode   PrivilegedMode
	reason string
	policy privilegePolicyFile
}{mode: PrivilegedModeDisabled, reason: "尚未初始化系统权限模式"}

func init() {
	var policy privilegePolicyFile
	if err := json.Unmarshal(embeddedPrivilegePolicy, &policy); err != nil || validatePrivilegePolicy(policy) != nil {
		privilegeRuntime.reason = "宿主权限策略无效"
		return
	}
	privilegeRuntime.policy = policy
}

func validatePrivilegePolicy(policy privilegePolicyFile) error {
	if strings.TrimSpace(policy.Version) == "" {
		return errors.New("缺少策略版本")
	}
	seen := make(map[string]struct{}, len(policy.Operations))
	for _, operation := range policy.Operations {
		if strings.TrimSpace(operation.PluginID) == "" || strings.TrimSpace(operation.Action) == "" || strings.TrimSpace(operation.Platform) == "" || strings.TrimSpace(operation.Tag) == "" || strings.TrimSpace(operation.Asset) == "" || len(operation.ArchiveSHA256) != 64 || len(operation.RunnerSHA256) != 64 {
			return errors.New("权限策略记录不完整")
		}
		if _, err := hex.DecodeString(operation.ArchiveSHA256); err != nil {
			return errors.New("权限策略中的安装包哈希无效")
		}
		if _, err := hex.DecodeString(operation.RunnerSHA256); err != nil {
			return errors.New("权限策略中的执行器哈希无效")
		}
		key := strings.Join([]string{operation.PluginID, operation.Action, operation.Platform, operation.Tag, operation.Asset, strings.ToLower(operation.ArchiveSHA256), strings.ToLower(operation.RunnerSHA256), strings.Join(operation.RunnerArgs, "\x00")}, "\x00")
		if _, exists := seen[key]; exists {
			return errors.New("权限策略包含重复操作")
		}
		seen[key] = struct{}{}
	}
	return nil
}

// ConfigurePrivilegedMode must be called by the process entrypoint before it
// exposes HTTP. Production defaults to disabled; local desktop mode requires
// a literal loopback listener and is never inferred from reverse-proxy headers.
func ConfigurePrivilegedMode(bind string, production bool) error {
	requested := strings.ToLower(strings.TrimSpace(os.Getenv("ALX_PRIVILEGED_MODE")))
	if requested == "" {
		if production {
			requested = string(PrivilegedModeDisabled)
		} else {
			requested = string(PrivilegedModeLocal)
		}
	}
	mode := PrivilegedMode(requested)
	if mode != PrivilegedModeLocal && mode != PrivilegedModeDisabled {
		return errors.New("ALX_PRIVILEGED_MODE 仅支持 local 或 disabled")
	}
	reason := ""
	if mode == PrivilegedModeLocal && !literalLoopback(bind) {
		return errors.New("ALX_PRIVILEGED_MODE=local 仅允许监听 127.0.0.1 或 ::1")
	}
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
	return PrivilegeStatus{Enabled: privilegeRuntime.mode == PrivilegedModeLocal, Mode: string(privilegeRuntime.mode), Reason: privilegeRuntime.reason, Version: privilegeRuntime.policy.Version}
}

// AuthorizePluginPrivilege binds a request to both the immutable host policy
// and the exact release binary installed by the plugin manager.
func AuthorizePluginPrivilege(identity PluginPrivilegeIdentity) error {
	status := CurrentPrivilegeStatus()
	if !status.Enabled {
		return errors.New(status.Reason)
	}
	return CheckPluginPrivilege(identity)
}

// CheckPluginPrivilege verifies the immutable release binding without
// considering whether the current HTTP deployment is allowed to prompt.
func CheckPluginPrivilege(identity PluginPrivilegeIdentity) error {
	if strings.TrimSpace(identity.RunnerPath) == "" || strings.TrimSpace(identity.Tag) == "" || strings.TrimSpace(identity.Asset) == "" || len(identity.ArchiveSHA256) != 64 {
		return errors.New("高权限操作仅允许已验证的正式插件 Release")
	}
	runnerDigest, err := fileSHA256(identity.RunnerPath)
	if err != nil {
		return fmt.Errorf("无法校验插件执行器：%w", err)
	}
	platform := runtime.GOOS + "-" + runtime.GOARCH
	privilegeRuntime.RLock()
	operations := append([]privilegeOperation(nil), privilegeRuntime.policy.Operations...)
	privilegeRuntime.RUnlock()
	for _, operation := range operations {
		if operation.PluginID == identity.PluginID && operation.Action == identity.Action && operation.Platform == platform && operation.Tag == identity.Tag && operation.Asset == identity.Asset && strings.EqualFold(operation.ArchiveSHA256, identity.ArchiveSHA256) && strings.EqualFold(operation.RunnerSHA256, runnerDigest) && sameStrings(operation.RunnerArgs, identity.RunnerArgs) {
			if !sameActionSet(identity.DeclaredActions, approvedActions(operations, operation)) {
				return errors.New("插件声明的系统权限操作与宿主审核策略不一致")
			}
			return nil
		}
	}
	return errors.New("当前插件版本未被宿主权限策略审核，已降级为只读/预演")
}

func approvedActions(operations []privilegeOperation, binding privilegeOperation) []string {
	actions := make([]string, 0, len(operations))
	for _, operation := range operations {
		if operation.PluginID == binding.PluginID && operation.Platform == binding.Platform && operation.Tag == binding.Tag && operation.Asset == binding.Asset && strings.EqualFold(operation.ArchiveSHA256, binding.ArchiveSHA256) && strings.EqualFold(operation.RunnerSHA256, binding.RunnerSHA256) && sameStrings(operation.RunnerArgs, binding.RunnerArgs) {
			actions = append(actions, operation.Action)
		}
	}
	return actions
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameActionSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]struct{}, len(left))
	for _, action := range left {
		if strings.TrimSpace(action) == "" {
			return false
		}
		seen[action] = struct{}{}
	}
	if len(seen) != len(left) {
		return false
	}
	for _, action := range right {
		if _, ok := seen[action]; !ok {
			return false
		}
	}
	return true
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
