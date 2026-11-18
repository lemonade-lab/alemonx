package robot

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"alemonx/internal/packagemanager"
	"alemonx/internal/pm2config"
	"alemonx/internal/system"

	"github.com/goccy/go-yaml"
)

const generatedBunConfig = "[run]\nbun = true\n"
const runtimeSwitchBackupPrefix = ".alx-runtime-switch-"

type RuntimeSwitchPlan struct {
	Current        string       `json:"current"`
	Target         string       `json:"target"`
	Version        string       `json:"version,omitempty"`
	PackageManager string       `json:"packageManager"`
	Revision       string       `json:"revision"`
	Changes        []string     `json:"changes"`
	Blocked        []string     `json:"blocked"`
	Check          system.Check `json:"check"`
	Platform       string       `json:"platform"`
}

type runtimeSwitchFiles struct {
	plan     RuntimeSwitchPlan
	manifest []byte
	bunfig   []byte
	pm2      *pm2config.Options
}

func (m Manager) RuntimeSwitchPlan(ctx context.Context, root, target string) (RuntimeSwitchPlan, error) {
	_, files, err := m.planRuntimeSwitch(ctx, root, target)
	return files.plan, err
}

func (m Manager) planRuntimeSwitch(ctx context.Context, root, target string) (string, runtimeSwitchFiles, error) {
	var files runtimeSwitchFiles
	root, err := projectPath(root)
	if err != nil {
		return "", files, err
	}
	if target != "node" && target != "bun" {
		return "", files, errors.New("请选择 Node.js 或 Bun")
	}
	raw, err := readSwitchFile(root, "package.json")
	if err != nil {
		return "", files, err
	}
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(raw, &manifest); err != nil || manifest == nil {
		return "", files, errors.New("无法读取 package.json")
	}
	var declaredManager string
	_ = json.Unmarshal(manifest["packageManager"], &declaredManager)
	current := packagemanager.ProjectRuntime(root)
	plan := RuntimeSwitchPlan{Current: current, Target: target, Changes: []string{}, Blocked: []string{}}
	report := system.NewChecker().CheckProjectRuntime(target)
	plan.Platform, plan.Check = report.Platform, report.Checks[0]
	if check := report.Checks[0]; check.Status == "missing" || check.Status == "warning" {
		plan.Blocked = append(plan.Blocked, check.Suggestion)
	} else {
		plan.Version = check.Detail
	}
	if current == target {
		plan.Blocked = append(plan.Blocked, "项目已使用所选运行时。")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", files, err
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), runtimeSwitchBackupPrefix) {
			plan.Blocked = append(plan.Blocked, "发现上次切换的恢复目录 "+entry.Name()+"，请先完成恢复或清理后重新检查。")
		}
	}
	setValue := func(key, value string) {
		manifest[key], _ = json.Marshal(value)
	}
	setValue("alemonxRuntime", target)
	manifest["alemonxMigratedRuntime"] = json.RawMessage("true")
	if target == "bun" {
		manager := projectPackageManager(root)
		if manager != "bun" {
			if nodeManagerName(declaredManager) != manager {
				declaredManager = defaultNodePackageManager(manager)
			}
			setValue("alemonxNodePackageManager", declaredManager)
		}
		plan.PackageManager = "bun"
		if len(plan.Blocked) == 0 {
			version, versionErr := system.BunVersion(ctx)
			if versionErr != nil {
				plan.Check.Status, plan.Check.Suggestion = "warning", versionErr.Error()
				plan.Blocked = append(plan.Blocked, versionErr.Error())
			} else {
				setValue("packageManager", "bun@"+version)
			}
		}
	} else {
		var previous string
		_ = json.Unmarshal(manifest["alemonxNodePackageManager"], &previous)
		if nodeManagerName(previous) == "" {
			previous = defaultNodePackageManager("npm")
		}
		plan.PackageManager = nodeManagerName(previous)
		setValue("packageManager", previous)
		tool := "npm"
		if plan.PackageManager != "npm" {
			tool = "npx"
			if _, err := exec.LookPath(plan.PackageManager); err == nil {
				tool = plan.PackageManager
			}
		}
		if _, err := exec.LookPath(tool); err != nil {
			suggestion := "目标包管理器缺少 " + tool + "，请先修复 Node.js 的安装。"
			plan.Check.Status, plan.Check.Suggestion = "warning", suggestion
			plan.Blocked = append(plan.Blocked, suggestion)
		}
	}
	var scripts map[string]string
	if err := json.Unmarshal(manifest["scripts"], &scripts); err == nil {
		for key, value := range scripts {
			switch value {
			case "node index.js", "bun index.js":
				scripts[key] = target + " index.js"
			case "npx alemonc start", "bun x --bun alemonc start":
				if target == "bun" {
					scripts[key] = "bun x --bun alemonc start"
				} else {
					scripts[key] = "npx alemonc start"
				}
			}
		}
		manifest["scripts"], _ = json.Marshal(scripts)
	}
	plan.Changes = append(plan.Changes, "更新运行时、包管理器和标准启动脚本", "用目标包管理器重新安装依赖并验证安装结果")
	bunfig, err := readSwitchFile(root, "bunfig.toml")
	if err != nil {
		return "", files, err
	}
	if target == "bun" && bunfig == nil {
		files.bunfig = []byte(generatedBunConfig)
		plan.Changes = append(plan.Changes, "生成 Bun 脚本运行配置")
	} else if target == "node" && string(bunfig) == generatedBunConfig {
		files.bunfig = []byte{}
		plan.Changes = append(plan.Changes, "移除引导生成的 Bun 脚本运行配置")
	}
	pm2Config, err := readSwitchFile(root, "pm2.config.cjs")
	if err != nil {
		return "", files, err
	}
	if pm2Config != nil {
		line, _, _ := strings.Cut(string(pm2Config), "\n")
		options := pm2config.Options{}
		metadata, managed := strings.CutPrefix(line, pm2SettingsPrefix)
		if managed && json.Unmarshal([]byte(metadata), &options) == nil &&
			strings.TrimSpace(string(pm2Config)) == strings.TrimSpace(pm2config.ConfigWithOptions(root, options)) {
			files.pm2 = &options
			plan.Changes = append(plan.Changes, "保留 PM2 设置并更新解释器")
		} else if isManagedPM2Config(root, string(pm2Config)) {
			options = pm2config.DefaultOptions(root)
			if oldName := pm2ConfigAppName(root); oldName != "" {
				options.Name = oldName
			}
			files.pm2 = &options
			plan.Changes = append(plan.Changes, "升级旧版 PM2 模板并更新解释器")
		} else {
			plan.Blocked = append(plan.Blocked, "项目使用自定义 PM2 配置，请先在后台运行设置中确认解释器配置并迁移为工作台托管配置。")
		}
	}
	digest := sha256.New()
	for _, contents := range [][]byte{raw, bunfig, pm2Config, []byte(target)} {
		fmt.Fprintf(digest, "%d:", len(contents))
		digest.Write(contents)
	}
	plan.Revision = fmt.Sprintf("%x", digest.Sum(nil))
	files.plan = plan
	files.manifest, err = json.MarshalIndent(manifest, "", "  ")
	files.manifest = append(files.manifest, '\n')
	return root, files, err
}

func defaultNodePackageManager(manager string) string {
	switch manager {
	case "yarn":
		return "yarn@" + packagemanager.YarnVersion
	case "pnpm":
		return "pnpm@" + packagemanager.PNPMVersion
	default:
		return "npm@10"
	}
}

func nodeManagerName(declaration string) string {
	name, _, _ := strings.Cut(declaration, "@")
	switch name {
	case "npm", "yarn", "pnpm":
		return name
	}
	return ""
}

func readSwitchFile(root, name string) ([]byte, error) {
	path := filepath.Join(root, name)
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s 必须是项目内的普通文件", name)
	}
	return os.ReadFile(path)
}

var runtimeSwitchMetadata = []string{"package.json", "bunfig.toml", "pm2.config.cjs", "bun.lock", "bun.lockb", "yarn.lock", "pnpm-lock.yaml", "package-lock.json", "npm-shrinkwrap.json", ".pnp.cjs", ".pnp.js", ".pnp.loader.mjs"}

// ApplyRuntimeSwitch is called while the web operation owns the project. It
// keeps the old installation until the new package manager finishes.
func (m Manager) ApplyRuntimeSwitch(ctx context.Context, root, target, revision string) (result RuntimeSwitchPlan, err error) {
	root, files, err := m.planRuntimeSwitch(ctx, root, target)
	if err != nil {
		return result, err
	}
	result = files.plan
	if revision != result.Revision {
		return result, errors.New("项目配置已变更，请重新检查切换预览")
	}
	if len(result.Blocked) != 0 {
		return result, errors.New(strings.Join(result.Blocked, "\n"))
	}

	original := make(map[string][]byte, len(runtimeSwitchMetadata))
	modes := make(map[string]fs.FileMode, len(runtimeSwitchMetadata))
	for _, name := range runtimeSwitchMetadata {
		data, readErr := readSwitchFile(root, name)
		if readErr != nil {
			return result, readErr
		}
		original[name] = data
		modes[name] = 0o644
		if info, err := os.Lstat(filepath.Join(root, name)); err == nil {
			modes[name] = info.Mode().Perm()
		}
	}
	patterns, err := switchWorkspacePatterns(root, original["package.json"])
	if err != nil {
		return result, err
	}
	modules, err := switchInstallDirectories(root, patterns)
	if err != nil {
		return result, err
	}
	backup, err := os.MkdirTemp(root, runtimeSwitchBackupPrefix)
	if err != nil {
		return result, err
	}
	var moved []string
	installStarted := false
	defer func() {
		if err != nil {
			var rollback error
			if installStarted {
				generated, scanErr := switchInstallDirectories(root, patterns)
				rollback = errors.Join(rollback, scanErr)
				for _, name := range generated {
					rollback = errors.Join(rollback, os.RemoveAll(filepath.Join(root, name)))
				}
				for _, name := range runtimeSwitchMetadata {
					rollback = errors.Join(rollback, restoreSwitchFile(root, name, original[name], modes[name]))
				}
			}
			for _, name := range moved {
				rollback = errors.Join(rollback, os.Rename(filepath.Join(backup, name), filepath.Join(root, name)))
			}
			if rollback != nil {
				err = errors.Join(err, fmt.Errorf("自动恢复未完成，原依赖仍保存在 %s：%w", backup, rollback))
				return
			}
		}
		_ = os.RemoveAll(backup)
	}()
	for name, data := range original {
		if data != nil {
			if err = os.WriteFile(filepath.Join(backup, name), data, 0o600); err != nil {
				return result, err
			}
		}
	}

	for _, name := range modules {
		destination := filepath.Join(backup, name)
		if err = os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return result, err
		}
		if err = os.Rename(filepath.Join(root, name), destination); err != nil {
			return result, fmt.Errorf("无法备份依赖目录 %s：%w", name, err)
		}
		moved = append(moved, name)
	}

	installStarted = true
	if err = writeFileAtomically(filepath.Join(root, "package.json"), files.manifest, modes["package.json"]); err != nil {
		return result, err
	}
	if files.bunfig != nil {
		data := files.bunfig
		if len(data) == 0 {
			data = nil
		}
		if err = restoreSwitchFile(root, "bunfig.toml", data, modes["bunfig.toml"]); err != nil {
			return result, err
		}
	}
	if files.pm2 != nil {
		if err = writeFileAtomically(filepath.Join(root, "pm2.config.cjs"), []byte(pm2config.ConfigWithOptions(root, *files.pm2)), modes["pm2.config.cjs"]); err != nil {
			return result, err
		}
	}
	for _, name := range []string{".pnp.cjs", ".pnp.js", ".pnp.loader.mjs"} {
		if err = restoreSwitchFile(root, name, nil, modes[name]); err != nil {
			return result, err
		}
	}
	if _, err = m.installRuntimeDependencies(root, "安装目标运行时的依赖"); err != nil {
		return result, err
	}
	return result, nil
}

func restoreSwitchFile(root, name string, data []byte, mode fs.FileMode) error {
	path := filepath.Join(root, name)
	if data == nil {
		err := os.Remove(path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	return writeFileAtomically(path, data, mode)
}

func switchWorkspacePatterns(root string, manifest []byte) ([]string, error) {
	var declared struct{ Workspaces json.RawMessage }
	if err := json.Unmarshal(manifest, &declared); err != nil {
		return nil, err
	}
	var patterns []string
	if len(declared.Workspaces) > 0 && json.Unmarshal(declared.Workspaces, &patterns) != nil {
		var object struct{ Packages []string }
		if err := json.Unmarshal(declared.Workspaces, &object); err != nil {
			return nil, fmt.Errorf("无法识别项目工作区：%w", err)
		}
		patterns = object.Packages
	}
	data, err := readSwitchFile(root, "pnpm-workspace.yaml")
	if err != nil {
		return nil, err
	}
	if data != nil {
		var pnpm struct {
			Packages []string `yaml:"packages"`
		}
		if err := yaml.Unmarshal(data, &pnpm); err != nil {
			return nil, fmt.Errorf("无法识别 pnpm 工作区：%w", err)
		}
		patterns = append(patterns, pnpm.Packages...)
	}
	return patterns, nil
}

func switchInstallDirectories(root string, patterns []string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(location string, entry fs.DirEntry, err error) error {
		if err != nil || location == root {
			return err
		}
		if entry.Name() == "node_modules" {
			relative, err := filepath.Rel(root, location)
			if err != nil {
				return err
			}
			workspace := filepath.ToSlash(filepath.Dir(relative))
			if workspace == "." || runtimeSwitchWorkspaceMatches(workspace, patterns) && exists(filepath.Join(filepath.Dir(location), "package.json")) {
				found = append(found, relative)
			}
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() && (len(patterns) == 0 || strings.HasPrefix(entry.Name(), ".")) {
			return filepath.SkipDir
		}
		return nil
	})
	return found, err
}

func runtimeSwitchWorkspaceMatches(directory string, patterns []string) bool {
	matched := false
	for _, pattern := range patterns {
		value, exclude := strings.CutPrefix(pattern, "!")
		value = strings.TrimSuffix(strings.TrimPrefix(value, "./"), "/")
		if matchWorkspaceParts(strings.Split(value, "/"), strings.Split(directory, "/")) {
			matched = !exclude
		}
	}
	return matched
}

func matchWorkspaceParts(pattern, directory []string) bool {
	if len(pattern) == 0 {
		return len(directory) == 0
	}
	if pattern[0] == "**" {
		return matchWorkspaceParts(pattern[1:], directory) ||
			len(directory) > 0 && matchWorkspaceParts(pattern, directory[1:])
	}
	if len(directory) == 0 {
		return false
	}
	matches, _ := path.Match(pattern[0], directory[0])
	return matches && matchWorkspaceParts(pattern[1:], directory[1:])
}

// A stopped PM2 registration keeps its original interpreter. Replace it
// only when the user explicitly starts or restarts the migrated project.
func (m Manager) preparePM2RuntimeChange(root, target string) (bool, error) {
	var manifest struct {
		Migrated bool `json:"alemonxMigratedRuntime"`
	}
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil || json.Unmarshal(data, &manifest) != nil || !manifest.Migrated {
		return false, nil
	}
	configName := pm2ConfigAppName(root)
	if configName == "" {
		return false, nil
	}
	if target != "" {
		var err error
		target, err = pm2ProcessTarget(target)
		if err != nil {
			return false, err
		}
	}
	processes, err := m.pm2ProjectProcesses(context.Background(), root, false)
	if err != nil {
		return false, err
	}
	wantBun := packagemanager.UsesBunRuntime(root)
	replace := []string{}
	for _, process := range processes {
		if process.Name != configName || (target != "" && target != fmt.Sprint(process.ID) && target != process.Name) {
			continue
		}
		base := strings.ToLower(filepath.Base(process.Interpreter))
		if wantBun == (base == "bun" || base == "bun.exe") {
			continue
		}
		if process.Status != "stopped" && process.Status != "errored" {
			return false, errors.New("请先手动停止旧运行时的 PM2 进程，再启动新运行时")
		}
		replace = append(replace, fmt.Sprint(process.ID))
	}
	name, args := pm2Launcher(root)
	for _, id := range replace {
		if _, err := run(root, name, append(args, "delete", id)...); err != nil {
			return false, fmt.Errorf("无法替换旧的 PM2 运行时登记：%w", err)
		}
	}
	return len(replace) > 0, nil
}
