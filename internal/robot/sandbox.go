package robot

import (
	"os"
	"path/filepath"
	"regexp"
)

// sandboxConfigPattern matches the top-level keys that must not be present in
// a testone sandbox config: login/platform switch alemonjs out of sandbox
// mode, and serverPort would make the sandbox bind the robot's application
// server port and collide with the original process.
var sandboxConfigPattern = regexp.MustCompile(`(?m)^\s*(login|platform|serverPort)\s*:.*$`)

// SandboxConfig prepares a temporary alemonjs configuration for the testone
// sandbox. The copy keeps the robot's port (or overrides it with port when
// non-zero) and every other setting but comments out login/platform/
// serverPort, so the runtime starts in no-login sandbox mode without modifying
// the user's alemon.config.yaml. The returned path is relative to the project
// root because alemonjs resolves CFG_PATH with path.join(process.cwd(), value)
// and Node's path.join concatenates absolute values under the cwd. The file
// lives in the project's .alx-testone directory; the cleanup func removes it
// (by absolute path) and must be called when the supervised process exits. An
// empty path and no-op cleanup mean no override was needed.
func (Manager) SandboxConfig(root string) (string, func(), error) {
	noop := func() {}
	project, err := projectPath(root)
	if err != nil {
		return "", noop, err
	}
	configFile := filepath.Join(project, "alemon.config.yaml")
	data, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", noop, nil
		}
		return "", noop, err
	}
	if !sandboxConfigPattern.Match(data) {
		return "", noop, nil
	}
	content := string(sandboxConfigPattern.ReplaceAll(data, []byte("# $1: 已由工作台临时禁用（测试台沙盒）")))
	dir := filepath.Join(project, ".alx-testone")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", noop, err
	}
	file, err := os.CreateTemp(dir, "alx-sandbox-*.yaml")
	if err != nil {
		return "", noop, err
	}
	absolutePath := file.Name()
	if _, err := file.Write([]byte(content)); err != nil {
		_ = file.Close()
		_ = os.Remove(absolutePath)
		return "", noop, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(absolutePath)
		return "", noop, err
	}
	relativePath, err := filepath.Rel(project, absolutePath)
	if err != nil {
		_ = os.Remove(absolutePath)
		return "", noop, err
	}
	return relativePath, func() {
		_ = os.Remove(absolutePath)
		_ = os.Remove(dir)
	}, nil
}
