package robot

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func installLocalPackage(root, source string) (Result, error) {
	if err := ensurePackagesWorkspace(root); err != nil {
		return Result{}, err
	}
	directory := filepath.Join(root, "packages")
	if err := os.MkdirAll(directory, 0755); err != nil {
		if permissionError(err) {
			return Result{}, permissionAdvice("创建 packages 目录")
		}
		return Result{}, fmt.Errorf("无法创建 packages 目录：%w", err)
	}
	name := localPackageName(source)
	target := filepath.Join(directory, name)
	if _, err := os.Stat(target); err == nil {
		return Result{}, fmt.Errorf("本地插件包 %s 已存在，工具不会覆盖它", name)
	}
	if strings.HasPrefix(source, "git+") {
		repository, _, err := splitGitPackageSource(source)
		if err != nil {
			return Result{}, err
		}
		// Backpack plugins are always checked out from the distributable branch.
		// Do not let a catalog URL, API caller, or stale UI select a tag or a
		// development branch here.
		ref := "release"
		cloneRepository, mirrored := githubPackageMirror(repository)
		args := []string{"clone", "--depth", "1"}
		if ref != "" {
			args = append(args, "--branch", ref)
		}
		args = append(args, cloneRepository, target)
		output, err := run(root, "git", args...)
		if err != nil {
			return Result{Path: target, Output: output}, fmt.Errorf("下载本地插件包失败：%w", err)
		}
		version := ""
		if ref != "" {
			version = "（" + ref + "）"
		}
		origin := ""
		if mirrored {
			origin = "已识别 GitHub 仓库，使用 GHFast 镜像下载。\n"
		}
		return Result{Path: target, Output: origin + "已安装到 packages/" + name + version + "。" + defaultEnableLocalPackage(root, target) + "\n" + output}, nil
	}
	temporary, err := os.MkdirTemp("", "alx-package-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(temporary)
	output, err := run(temporary, "npm", "pack", source, "--json")
	if err != nil {
		return Result{Path: target, Output: output}, fmt.Errorf("下载 npm 插件包失败：%w", err)
	}
	archive, err := packedFilename(output)
	if err != nil {
		return Result{}, err
	}
	extracted := filepath.Join(temporary, "extracted")
	if err := os.MkdirAll(extracted, 0755); err != nil {
		return Result{}, err
	}
	if output, err = run(temporary, "tar", "-xzf", filepath.Join(temporary, archive), "-C", extracted); err != nil {
		return Result{Path: target, Output: output}, fmt.Errorf("解压 npm 插件包失败：%w", err)
	}
	if err := copyPath(filepath.Join(extracted, "package"), target); err != nil {
		return Result{}, fmt.Errorf("写入本地插件包失败：%w", err)
	}
	return Result{Path: target, Output: "已安装到 packages/" + name + "。" + defaultEnableLocalPackage(root, target)}, nil
}

// defaultEnableLocalPackage adds a freshly installed backpack package to the
// alemon.config.yaml apps array so anything that enters the backpack starts
// enabled. Failures are non-fatal: the package stays installed either way.
func defaultEnableLocalPackage(root, target string) string {
	name := ""
	if data, err := os.ReadFile(filepath.Join(target, "package.json")); err == nil {
		var manifest struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(data, &manifest) == nil && manifest.Name != "" {
			name = manifest.Name
		}
	}
	if name == "" {
		return ""
	}
	if _, err := (Manager{}).SetAppEnabled(root, name, true); err != nil {
		return ""
	}
	return "，并已默认启用（加入 apps）"
}

// Connection packages are normal project dependencies. They must not be
// unpacked into packages/: Yarn owns node_modules and package.json so the
// selected adapter can participate in the robot runtime.
func installConnectionPackage(root, source string) (Result, error) {
	return installProjectDependency(root, source, "连接")
}

func removeConnectionPackage(root, source string) (Result, error) {
	return removeProjectDependency(root, source, "连接")
}

// JS modules run with the robot project just like connection packages, but do
// not declare a platform or participate in login selection.
func installModulePackage(root, source string) (Result, error) {
	return installProjectDependency(root, source, "模块")
}

func removeModulePackage(root, source string) (Result, error) {
	return removeProjectDependency(root, source, "模块")
}

func installProjectDependency(root, source, kind string) (Result, error) {
	manager, args, err := connectionPackageCommand(root, "add", source)
	if err != nil {
		return Result{}, err
	}
	output, err := runNamedPackageManager(root, manager, args...)
	if err != nil {
		return Result{Path: root, Output: output}, fmt.Errorf("安装%s包失败：%w", kind, err)
	}
	return Result{Path: root, Output: "已添加" + kind + "依赖 " + source + "。\n" + output}, nil
}

func removeProjectDependency(root, source, kind string) (Result, error) {
	manager, args, err := connectionPackageCommand(root, "remove", source)
	if err != nil {
		return Result{}, err
	}
	output, err := runNamedPackageManager(root, manager, args...)
	if err != nil {
		return Result{Path: root, Output: output}, fmt.Errorf("卸载%s包失败：%w", kind, err)
	}
	return Result{Path: root, Output: "已移除" + kind + "依赖 " + source + "。\n" + output}, nil
}

// connectionPackageCommand follows the project itself: package.json's
// packageManager wins, and lock files are only the compatibility fallback in
// projectPackageManager. A workspace root needs an explicit flag for Yarn and
// PNPM; npm already installs into the root manifest by default.
func connectionPackageCommand(root, action, source string) (string, []string, error) {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return "", nil, fmt.Errorf("无法读取 package.json：%w", err)
	}
	var manifest struct {
		Workspaces json.RawMessage `json:"workspaces"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", nil, errors.New("package.json 格式无法识别")
	}
	manager := projectPackageManager(root)
	args := []string{action, source}
	if len(manifest.Workspaces) > 0 && string(manifest.Workspaces) != "null" {
		switch manager {
		case "yarn":
			args = append(args, "-W")
		case "pnpm":
			args = append(args, "-w")
		}
	}
	return manager, args, nil
}

func ensurePackagesWorkspace(root string) error {
	manifest := filepath.Join(root, "package.json")
	data, err := os.ReadFile(manifest)
	if err != nil {
		return fmt.Errorf("无法读取 package.json：%w", err)
	}
	var values map[string]any
	if err := json.Unmarshal(data, &values); err != nil {
		return errors.New("package.json 格式无法识别")
	}
	if err := addPackagesWorkspace(values); err != nil {
		return err
	}
	values["private"] = true
	updated, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(manifest, append(updated, '\n'), 0644); err != nil {
		if permissionError(err) {
			return permissionAdvice("保存 packages 工作区配置")
		}
		return fmt.Errorf("无法写入 packages 工作区配置：%w", err)
	}
	return nil
}

// setBackpackWorkspace keeps the root manifest in the only topology that can
// load local packages from packages/: a private workspace root containing
// packages/*. Unlike the generic package manifest editor it changes only this
// one workspace pattern, so independently configured workspaces are retained.
func setBackpackWorkspace(root string, enabled bool) (Result, error) {
	manifest := filepath.Join(root, "package.json")
	data, err := os.ReadFile(manifest)
	if err != nil {
		return Result{}, fmt.Errorf("无法读取 package.json：%w", err)
	}
	var values map[string]any
	if err := json.Unmarshal(data, &values); err != nil {
		return Result{}, errors.New("package.json 格式无法识别")
	}
	if enabled {
		if err := addPackagesWorkspace(values); err != nil {
			return Result{}, err
		}
		values["private"] = true
	} else {
		if err := removePackagesWorkspace(values); err != nil {
			return Result{}, err
		}
		values["private"] = false
	}
	updated, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(manifest, append(updated, '\n'), 0644); err != nil {
		if permissionError(err) {
			return Result{}, permissionAdvice("保存背包工作区配置")
		}
		return Result{}, fmt.Errorf("无法保存 package.json：%w", err)
	}
	if enabled {
		return Result{Path: manifest, Output: "背包已设为私有，并已启用 packages/* 工作空间。"}, nil
	}
	return Result{Path: manifest, Output: "背包已设为公开，并已移除 packages/* 工作空间。"}, nil
}

func addPackagesWorkspace(values map[string]any) error {
	workspaces, exists := values["workspaces"]
	if !exists || workspaces == nil {
		values["workspaces"] = []string{"packages/*"}
		return nil
	}
	switch current := workspaces.(type) {
	case []any:
		if workspaceContains(current, "packages/*") {
			return nil
		}
		values["workspaces"] = append(current, "packages/*")
		return nil
	case map[string]any:
		packages, err := workspacePackages(current)
		if err != nil {
			return err
		}
		if !workspaceContains(packages, "packages/*") {
			current["packages"] = append(packages, "packages/*")
		}
		values["workspaces"] = current
		return nil
	default:
		return errors.New("package.json 的 workspaces 格式暂不支持背包开关")
	}
}

func removePackagesWorkspace(values map[string]any) error {
	workspaces, exists := values["workspaces"]
	if !exists || workspaces == nil {
		return nil
	}
	switch current := workspaces.(type) {
	case []any:
		remaining := withoutWorkspace(current, "packages/*")
		if len(remaining) == 0 {
			delete(values, "workspaces")
		} else {
			values["workspaces"] = remaining
		}
		return nil
	case map[string]any:
		packages, err := workspacePackages(current)
		if err != nil {
			return err
		}
		remaining := withoutWorkspace(packages, "packages/*")
		if len(remaining) == 0 {
			// yarn's object form has no useful meaning without packages; remove
			// its associated nohoist metadata together with the last package.
			delete(values, "workspaces")
		} else {
			current["packages"] = remaining
			values["workspaces"] = current
		}
		return nil
	default:
		return errors.New("package.json 的 workspaces 格式暂不支持背包开关")
	}
}

func workspacePackages(workspaces map[string]any) ([]any, error) {
	value, exists := workspaces["packages"]
	if !exists || value == nil {
		return nil, nil
	}
	packages, ok := value.([]any)
	if !ok {
		return nil, errors.New("package.json 的 workspaces.packages 格式暂不支持背包开关")
	}
	return packages, nil
}

func workspaceContains(workspaces []any, target string) bool {
	for _, workspace := range workspaces {
		if workspace == target {
			return true
		}
	}
	return false
}

func withoutWorkspace(workspaces []any, target string) []any {
	remaining := make([]any, 0, len(workspaces))
	for _, workspace := range workspaces {
		if workspace != target {
			remaining = append(remaining, workspace)
		}
	}
	return remaining
}

func removeLocalPackage(root, source string) (Result, error) {
	name := localPackageName(source)
	target := filepath.Join(root, "packages", name)
	info, err := os.Stat(target)
	if os.IsNotExist(err) {
		return Result{}, fmt.Errorf("背包中没有 %s", name)
	}
	if err != nil || !info.IsDir() {
		return Result{}, errors.New("本地插件包目录无法访问")
	}
	if err := os.RemoveAll(target); err != nil {
		return Result{}, fmt.Errorf("移除本地插件包失败：%w", err)
	}
	return Result{Path: target, Output: "已从 packages 移除 " + name + "。"}, nil
}

// removeLocalPackageByName removes a package selected from the backpack UI.
// A failed install can leave a directory without a valid package.json; the
// backpack still lists it by its directory name, so deletion must accept the
// same identifier instead of requiring a parseable manifest. The requested
// name is validated before it is ever compared with directory names, so a
// request cannot escape packages/.
func removeLocalPackageByName(root, packageName string) (Result, error) {
	if !packageNamePattern.MatchString(packageName) {
		return Result{}, errors.New("本地插件包名无效")
	}
	directory := filepath.Join(root, "packages")
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return Result{}, fmt.Errorf("背包中没有 %s", packageName)
	}
	if err != nil {
		return Result{}, fmt.Errorf("无法读取背包：%w", err)
	}
	type candidate struct {
		name   string
		target string
	}
	candidates := make([]candidate, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		target := filepath.Join(directory, entry.Name())
		if name := packageManifestName(target); name != "" {
			candidates = append(candidates, candidate{name: name, target: target})
			continue
		}
		// Invalid leftover directory (failed install): the backpack shows the
		// directory name, so that identifier must be removable as well.
		candidates = append(candidates, candidate{name: entry.Name(), target: target})
		if strings.HasPrefix(entry.Name(), "@") {
			scopeEntries, scopeErr := os.ReadDir(target)
			if scopeErr != nil {
				continue
			}
			for _, child := range scopeEntries {
				if !child.IsDir() || strings.HasPrefix(child.Name(), ".") {
					continue
				}
				childTarget := filepath.Join(target, child.Name())
				name := entry.Name() + "/" + child.Name()
				if manifestName := packageManifestName(childTarget); manifestName != "" {
					name = manifestName
				}
				candidates = append(candidates, candidate{name: name, target: childTarget})
			}
		}
	}
	for _, item := range candidates {
		if item.name != packageName {
			continue
		}
		if err := os.RemoveAll(item.target); err != nil {
			return Result{}, fmt.Errorf("移除本地插件包失败：%w", err)
		}
		return Result{Path: item.target, Output: "已从背包移除 " + packageName + "。"}, nil
	}
	return Result{}, fmt.Errorf("背包中没有 %s", packageName)
}

// packageManifestName returns the package name declared by a directory's
// package.json, or an empty string when the manifest is missing or invalid.
func packageManifestName(directory string) string {
	data, err := os.ReadFile(filepath.Join(directory, "package.json"))
	if err != nil {
		return ""
	}
	var manifest struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(data, &manifest) != nil || manifest.Name == "" {
		return ""
	}
	return manifest.Name
}

// replaceLocalPackage downloads the selected published npm version before the
// previous directory is discarded. The old directory is kept as a temporary
// sibling and restored if the download or unpacking fails.
func replaceLocalPackage(root, packageName, version string) (Result, error) {
	if !packageNamePattern.MatchString(packageName) {
		return Result{}, errors.New("本地插件包名无效")
	}
	if !regexp.MustCompile(`^v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$`).MatchString(version) {
		return Result{}, errors.New("请选择有效的插件版本")
	}
	items, err := (Manager{}).LocalPackages(root)
	if err != nil {
		return Result{}, err
	}
	var previous LocalPackage
	for _, item := range items {
		if item.Name == packageName && item.Valid {
			previous = item
			break
		}
	}
	if previous.Path == "" {
		return Result{}, errors.New("背包中没有可更新的本地插件包")
	}
	backup := previous.Path + ".alx-backup"
	if _, err := os.Stat(backup); err == nil {
		return Result{}, errors.New("检测到未完成的插件更新，请先刷新背包后重试")
	}
	if err := os.Rename(previous.Path, backup); err != nil {
		return Result{}, fmt.Errorf("无法备份当前插件版本：%w", err)
	}
	result, installErr := installLocalPackage(root, packageName+"@"+strings.TrimPrefix(version, "v"))
	if installErr == nil {
		_ = os.RemoveAll(backup)
		return Result{Path: result.Path, Output: "已切换 " + packageName + " 到 v" + strings.TrimPrefix(version, "v") + "。\n" + result.Output}, nil
	}
	_ = os.RemoveAll(result.Path)
	if restoreErr := os.Rename(backup, previous.Path); restoreErr != nil {
		return Result{}, fmt.Errorf("更新失败且无法恢复原插件：%w", restoreErr)
	}
	return result, fmt.Errorf("切换版本失败，已恢复原插件：%w", installErr)
}

func switchLocalPackageVersion(root, packageName, version string, force bool) (Result, error) {
	items, err := (Manager{}).LocalPackages(root)
	if err != nil {
		return Result{}, err
	}
	for _, item := range items {
		if item.Name != packageName || !item.Valid {
			continue
		}
		if output, gitErr := gitRun(item.Path, "rev-parse", "--is-inside-work-tree"); gitErr == nil && strings.TrimSpace(output) == "true" {
			status, statusErr := releaseGitStatus(item.Path)
			if statusErr != nil {
				return Result{}, statusErr
			}
			if !isReleaseBranch(status.Branch) {
				return Result{}, errors.New("当前插件不在 release 发布分支")
			}
			if output, fetchErr := gitRun(item.Path, "fetch", "origin", status.Branch); fetchErr != nil {
				return Result{Path: item.Path, Output: output}, errors.New("无法获取插件发布分支")
			}
			status, statusErr = releaseGitStatus(item.Path)
			if statusErr != nil {
				return Result{}, statusErr
			}
			if !force && (status.Dirty || status.Ahead > 0) {
				return Result{}, releaseSyncConfirmationError(status)
			}
			target := "origin/" + status.Branch
			if version != "" && version != status.Branch {
				if !regexp.MustCompile(`^[0-9a-f]{7,40}$`).MatchString(version) {
					return Result{}, errors.New("请选择 release 分支中的有效提交")
				}
				resolved, resolveErr := gitRun(item.Path, "rev-parse", "--verify", version+"^{commit}")
				if resolveErr != nil {
					return Result{}, errors.New("找不到所选 release 提交")
				}
				resolved = strings.TrimSpace(resolved)
				if _, ancestorErr := gitRun(item.Path, "merge-base", "--is-ancestor", resolved, target); ancestorErr != nil {
					return Result{}, errors.New("所选提交不属于 release 分支")
				}
				target = resolved
			}
			var logs []string
			if force {
				resetOutput, resetErr := gitRun(item.Path, "reset", "--hard")
				if resetErr != nil {
					return Result{Path: item.Path, Output: resetOutput}, fmt.Errorf("无法丢弃插件的已跟踪修改：%w", resetErr)
				}
				cleanOutput, cleanErr := gitRun(item.Path, "clean", "-fd")
				if cleanErr != nil {
					return Result{Path: item.Path, Output: strings.TrimSpace(resetOutput + "\n" + cleanOutput)}, fmt.Errorf("无法清理插件的未跟踪文件：%w", cleanErr)
				}
				logs = append(logs, "已丢弃该插件工作区的本地 Git 修改。", resetOutput, cleanOutput)
			}
			output, checkoutErr := gitRun(item.Path, "checkout", "-B", status.Branch, target)
			logs = append(logs, output)
			if checkoutErr == nil {
				output, checkoutErr = gitRun(item.Path, "reset", "--hard", target)
				logs = append(logs, output)
			}
			output = strings.TrimSpace(strings.Join(logs, "\n"))
			if checkoutErr != nil {
				return Result{Path: item.Path, Output: output}, checkoutErr
			}
			return Result{Path: item.Path, Output: "已同步 " + status.Branch + " 发布分支。\n" + output}, nil
		}
		return replaceLocalPackage(root, packageName, version)
	}
	return Result{}, errors.New("背包中没有可切换版本的本地插件包")
}

type releaseStatus struct {
	Branch string
	Head   string
	Ahead  int
	Dirty  bool
}

func releaseGitStatus(path string) (releaseStatus, error) {
	branch, _ := gitRun(path, "symbolic-ref", "--quiet", "--short", "HEAD")
	head, headErr := gitRun(path, "rev-parse", "--short", "HEAD")
	if headErr != nil {
		return releaseStatus{}, errors.New("无法读取插件 Git 提交")
	}
	dirty, dirtyErr := gitRun(path, "status", "--porcelain")
	if dirtyErr != nil {
		return releaseStatus{}, errors.New("无法确认 Git 工作区状态")
	}
	status := releaseStatus{Branch: strings.TrimSpace(branch), Head: strings.TrimSpace(head), Dirty: strings.TrimSpace(dirty) != ""}
	if isReleaseBranch(status.Branch) {
		counts, err := gitRun(path, "rev-list", "--left-right", "--count", "origin/"+status.Branch+"..."+status.Branch)
		if err != nil {
			return releaseStatus{}, errors.New("无法比较本地与远程 release 分支")
		}
		var behind int
		if _, err := fmt.Sscanf(strings.TrimSpace(counts), "%d %d", &behind, &status.Ahead); err != nil {
			return releaseStatus{}, errors.New("无法识别 release 分支状态")
		}
	}
	return status, nil
}

func releaseSyncConfirmationError(status releaseStatus) error {
	reasons := make([]string, 0, 3)
	if !isReleaseBranch(status.Branch) {
		reasons = append(reasons, "当前不在 release 分支")
	}
	if status.Dirty {
		reasons = append(reasons, "存在未提交修改")
	}
	if status.Ahead > 0 {
		reasons = append(reasons, fmt.Sprintf("本地 release 分支领先远程 %d 个提交", status.Ahead))
	}
	return errors.New(strings.Join(reasons, "；") + "，请确认后强制同步")
}

func localPackageName(source string) string {
	value := strings.TrimPrefix(source, "git+")
	value = strings.SplitN(value, "#", 2)[0]
	value = strings.TrimSuffix(value, ".git")
	if index := strings.LastIndex(value, "@"); index > strings.LastIndex(value, "/") {
		value = value[:index]
	}
	value = filepath.Base(value)
	value = strings.TrimPrefix(value, "@")
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ReplaceAll(value, "@", "-")
	return value
}

var gitPackageRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

const githubPackageMirrorPrefix = "https://ghfast.top/"

// githubPackageMirror accelerates only public HTTPS GitHub package sources.
// SSH, Gitee, and URLs carrying credentials or queries retain the exact value
// chosen by the user, so a mirror never receives a secret or private URL.
func githubPackageMirror(repository string) (string, bool) {
	parsed, err := url.Parse(repository)
	if err != nil || parsed == nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "github.com") || parsed.User != nil || parsed.RawQuery != "" {
		return repository, false
	}
	return githubPackageMirrorPrefix + parsed.String(), true
}

// splitGitPackageSource accepts the npm-style git+URL#tag form. The ref is
// passed as a separate git --branch argument, never interpolated into a shell.
func splitGitPackageSource(source string) (repository, ref string, err error) {
	value := strings.TrimPrefix(strings.TrimSpace(source), "git+")
	parts := strings.SplitN(value, "#", 2)
	repository = strings.TrimSpace(parts[0])
	if repository == "" || !(strings.HasPrefix(repository, "https://") || strings.HasPrefix(repository, "ssh://") || strings.HasPrefix(repository, "git@")) {
		return "", "", errors.New("Git 插件地址无效")
	}
	if len(parts) == 2 {
		ref = strings.TrimSpace(parts[1])
	}
	if ref != "" && (!gitPackageRefPattern.MatchString(ref) || strings.Contains(ref, "..") || strings.HasPrefix(ref, "-")) {
		return "", "", errors.New("插件版本 tag 无效")
	}
	return repository, ref, nil
}

func packedFilename(output string) (string, error) {
	var entries []struct {
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal([]byte(output), &entries); err != nil || len(entries) == 0 || entries[0].Filename == "" {
		return "", errors.New("npm 插件包文件名无效")
	}
	return entries[0].Filename, nil
}
