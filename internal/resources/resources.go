// Package resources provides access to the embedded runtime resources
// directory. The resources root is embedded into the binary together with the
// frontend dist and contains:
//
//	templates/            AlemonJS project templates (materialized to the
//	                      workspace so users can edit them)
//	packages/yarn/        The bundled Yarn classic. Its node_modules is
//	                      installed at build time and embedded so projects can
//	                      always be created and managed without npm.
//
// Embedded tools are materialized into <workspace>/packages/<name> on first
// use. Heavier optional tools such as PM2 are not embedded: they are
// provisioned on demand with the embedded Yarn into the same stable
// <workspace>/packages/<name> location, so the tool never relies on npx and
// never wanders into an ephemeral cache.
package resources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"alemonx/internal/workspace"
)

const (
	versionMarker    = ".alemonx-version"
	nvmBundleVersion = "0.40.7"
)

// Tool describes one runtime package and the entry point used to run it.
type Tool struct {
	// Entry is the package-relative JavaScript entry executed by node.
	Entry string
	// OnDemand marks tools that are installed with the embedded Yarn on first
	// use instead of being embedded into the binary.
	OnDemand bool
	// PackageName and PackageVersion are used for on-demand installation.
	PackageName    string
	PackageVersion string
	// BundleVersion identifies the embedded/provisioned bundle for the
	// .alemonx-version marker; bump it when the tool definition changes.
	BundleVersion string
}

var tools = map[string]Tool{
	"yarn": {
		Entry:         filepath.ToSlash("node_modules/yarn/bin/yarn.js"),
		BundleVersion: "1",
	},
	"pm2": {
		Entry:          filepath.ToSlash("node_modules/pm2/bin/pm2"),
		OnDemand:       true,
		PackageName:    "pm2",
		PackageVersion: "^5.4.3",
		BundleVersion:  "1",
	},
}

var (
	mu              sync.Mutex
	embedded        fs.FS
	workspaceRoot   string
	materialized    = map[string]string{}
	provisionErrors = map[string]string{}

	// provisionRunner executes an install command inside a directory. It is a
	// variable so tests can substitute a stub that never touches the network.
	provisionRunner = func(directory, command string, args ...string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, command, args...)
		cmd.Dir = directory
		output, err := cmd.CombinedOutput()
		if err != nil {
			message := strings.TrimSpace(string(output))
			if message != "" {
				return fmt.Errorf("%w：%s", err, message)
			}
			return err
		}
		return nil
	}
)

// Init records the embedded resources root and the runtime workspace root.
// It must be called once at startup before ToolCommand can serve tools.
// Without Init every resolver falls back to the legacy behaviour
// (npx/corepack), which keeps tests and embedders safe.
func Init(root fs.FS, layout workspace.Layout) {
	mu.Lock()
	defer mu.Unlock()
	embedded = root
	workspaceRoot = layout.Root
	materialized = map[string]string{}
	provisionErrors = map[string]string{}
}

// ToolCommand returns the command line used to run a tool:
// the local node binary plus the package entry, for example
// `node <workspace>/packages/pm2/node_modules/pm2/bin/pm2`.
// The second return value is the leading arguments and the bool reports
// whether the tool is available. On-demand tools are provisioned (with the
// embedded Yarn) before the first use.
func ToolCommand(name string) (string, []string, bool) {
	tool, ok := tools[name]
	if !ok {
		return "", nil, false
	}
	mu.Lock()
	defer mu.Unlock()
	if embedded == nil || workspaceRoot == "" {
		return "", nil, false
	}
	dir, err := toolDirectory(name, tool)
	if err != nil {
		return "", nil, false
	}
	entry := filepath.Join(dir, tool.Entry)
	if info, err := os.Stat(entry); err != nil || info.IsDir() {
		return "", nil, false
	}
	return "node", []string{entry}, true
}

// MaterializeNVM writes the reviewed, embedded NVM bundle to target. The
// target is versioned by its caller, so existing Node versions are never
// overwritten by a bundle refresh. NVM is intentionally not installed into a
// shell profile: callers source nvm.sh only in the process that needs it.
func MaterializeNVM(target string) (bool, error) {
	mu.Lock()
	defer mu.Unlock()
	if embedded == nil {
		return false, errors.New("嵌入资源未初始化")
	}
	if nvmBundleComplete(target) {
		return false, nil
	}
	if _, err := os.Stat(target); err == nil {
		return false, errors.New("内置 NVM 目录不完整，请删除后重新安装")
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return false, err
	}
	staging, err := os.MkdirTemp(filepath.Dir(target), ".nvm-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(staging)
	if err := copyEmbeddedDirectory("nvm/v"+nvmBundleVersion, staging); err != nil {
		return false, fmt.Errorf("物化内置 NVM 失败：%w", err)
	}
	if err := os.WriteFile(filepath.Join(staging, versionMarker), []byte(nvmBundleVersion+"\n"), 0o600); err != nil {
		return false, err
	}
	if err := os.Chmod(filepath.Join(staging, "nvm-exec"), 0o700); err != nil {
		return false, err
	}
	if err := os.Rename(staging, target); err != nil {
		if nvmBundleComplete(target) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func nvmBundleComplete(target string) bool {
	for _, name := range []string{"nvm.sh", "nvm-exec", "LICENSE.md", versionMarker} {
		info, err := os.Stat(filepath.Join(target, name))
		if err != nil || info.IsDir() {
			return false
		}
	}
	data, err := os.ReadFile(filepath.Join(target, versionMarker))
	return err == nil && strings.TrimSpace(string(data)) == nvmBundleVersion
}

func copyEmbeddedDirectory(source, target string) error {
	if err := fs.WalkDir(embedded, source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative := strings.TrimPrefix(path, source)
		relative = strings.TrimPrefix(relative, "/")
		output := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(output, 0o700)
		}
		data, err := fs.ReadFile(embedded, path)
		if err != nil {
			return err
		}
		return os.WriteFile(output, data, 0o600)
	}); err != nil {
		return err
	}
	return nil
}

// toolDirectory returns the stable workspace directory for a tool,
// materializing embedded packages or provisioning on-demand tools.
// The caller must hold mu.
func toolDirectory(name string, tool Tool) (string, error) {
	dir := filepath.Join(workspaceRoot, "packages", name)
	if !tool.OnDemand {
		if materialized[name] != "" {
			return materialized[name], nil
		}
		if err := materializePackage(name, dir); err != nil {
			return "", err
		}
		_ = writeVersionMarker(dir, tool)
		if data, readErr := os.ReadFile(filepath.Join(dir, versionMarker)); readErr == nil && tool.BundleVersion != "" &&
			strings.TrimSpace(string(data)) != tool.BundleVersion {
			log.Printf("内置 %s 有更新版本（副本 v%s，内置 v%s）；不会自动覆盖，如需更新请删除工作区中的副本目录后重新使用。", name, strings.TrimSpace(string(data)), tool.BundleVersion)
		}
		materialized[name] = dir
		return dir, nil
	}
	return ensureProvisioned(dir, tool)
}

// ensureProvisioned installs an on-demand tool with the embedded Yarn into
// the fixed workspace/packages/<name> directory. The caller must hold mu.
func ensureProvisioned(dir string, tool Tool) (string, error) {
	entry := filepath.Join(dir, tool.Entry)
	if info, err := os.Stat(entry); err == nil && !info.IsDir() {
		return dir, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("无法创建工具目录 %s：%w", dir, err)
	}
	packagePath := filepath.Join(dir, "package.json")
	if _, err := os.Stat(packagePath); errors.Is(err, os.ErrNotExist) {
		manifest, marshalErr := json.MarshalIndent(map[string]any{
			"name":         "@alemonx/" + tool.PackageName + "-runtime",
			"version":      "1.0.0",
			"private":      true,
			"dependencies": map[string]string{tool.PackageName: tool.PackageVersion},
		}, "", "  ")
		if marshalErr != nil {
			return "", marshalErr
		}
		if writeErr := os.WriteFile(packagePath, append(manifest, '\n'), 0o644); writeErr != nil {
			return "", writeErr
		}
	}
	yarnCommand, yarnArgs, ok := embeddedYarnCommand()
	if !ok {
		return "", errors.New("内置 Yarn 不可用，无法安装 " + tool.PackageName)
	}
	installArgs := append(append([]string{}, yarnArgs...), "install", "--ignore-scripts", "--non-interactive")
	if err := provisionRunner(dir, yarnCommand, installArgs...); err != nil {
		provisionErrors[tool.PackageName] = err.Error()
		return "", fmt.Errorf("使用内置 Yarn 安装 %s 失败：%w", tool.PackageName, err)
	}
	if info, err := os.Stat(entry); err != nil || info.IsDir() {
		provisionErrors[tool.PackageName] = "安装后入口缺失"
		return "", fmt.Errorf("%s 安装后入口缺失：%w", tool.PackageName, err)
	}
	delete(provisionErrors, tool.PackageName)
	_ = writeVersionMarker(dir, tool)
	return dir, nil
}

func writeVersionMarker(dir string, tool Tool) error {
	if tool.BundleVersion == "" {
		return nil
	}
	return os.WriteFile(filepath.Join(dir, versionMarker), []byte(tool.BundleVersion), 0o644)
}

// Outdated reports whether a materialized copy was produced by an older bundle
// definition. It never modifies the copy; callers decide how to surface it.
func Outdated(name string) (bool, string) {
	tool, ok := tools[name]
	if !ok {
		return false, ""
	}
	mu.Lock()
	defer mu.Unlock()
	if workspaceRoot == "" {
		return false, ""
	}
	data, err := os.ReadFile(filepath.Join(workspaceRoot, "packages", name, versionMarker))
	if err != nil {
		return false, ""
	}
	current := strings.TrimSpace(string(data))
	if current == "" || current == tool.BundleVersion {
		return false, current
	}
	return true, current
}

// LastProvisionError returns the recorded failure reason of an on-demand
// installation, or "" when the tool is available or was never attempted.
func LastProvisionError(name string) string {
	tool, ok := tools[name]
	if !ok {
		return ""
	}
	mu.Lock()
	defer mu.Unlock()
	return provisionErrors[tool.PackageName]
}

// Names returns the supported tool names in sorted order.
func Names() []string {
	mu.Lock()
	defer mu.Unlock()
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Installed reports whether a tool's entry already exists in the workspace
// without triggering materialization or provisioning.
func Installed(name string) bool {
	mu.Lock()
	defer mu.Unlock()
	if workspaceRoot == "" {
		return false
	}
	tool, ok := tools[name]
	if !ok {
		return false
	}
	_, err := os.Stat(filepath.Join(workspaceRoot, "packages", name, tool.Entry))
	return err == nil
}

// embeddedYarnCommand materializes the embedded Yarn and returns its node
// invocation. The caller must hold mu.
func embeddedYarnCommand() (string, []string, bool) {
	tool := tools["yarn"]
	dir, err := toolDirectory("yarn", tool)
	if err != nil {
		return "", nil, false
	}
	entry := filepath.Join(dir, tool.Entry)
	if info, err := os.Stat(entry); err != nil || info.IsDir() {
		return "", nil, false
	}
	return "node", []string{entry}, true
}

// materializePackage copies the embedded packages/<name> tree into target,
// keeping files that already exist (a partially materialized bundle is
// completed instead of replaced).
func materializePackage(name, target string) error {
	if embedded == nil {
		return errors.New("嵌入资源未初始化")
	}
	source := filepath.ToSlash("packages/" + name)
	if err := fs.WalkDir(embedded, source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative := strings.TrimPrefix(path, source)
		relative = strings.TrimPrefix(relative, "/")
		output := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(output, 0o755)
		}
		if _, err := os.Stat(output); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		data, err := fs.ReadFile(embedded, path)
		if err != nil {
			return err
		}
		return os.WriteFile(output, data, 0o644)
	}); err != nil {
		return fmt.Errorf("物化内置包 %s 失败：%w", name, err)
	}
	return nil
}

// Bundled reports whether a named tool is part of the supported runtime
// tools, regardless of whether it has been materialized or provisioned yet.
func Bundled(name string) bool {
	_, ok := tools[name]
	return ok
}
