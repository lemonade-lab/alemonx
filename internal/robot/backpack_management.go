package robot

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// LocalPackageStatus is a local, side-effect-free health snapshot. It never
// fetches a remote: checking a backpack must not unexpectedly use the network.
type LocalPackageStatus struct {
	Name             string   `json:"name"`
	Enabled          bool     `json:"enabled"`
	Source           string   `json:"source"`
	Branch           string   `json:"branch,omitempty"`
	Repository       string   `json:"repository,omitempty"`
	Dirty            bool     `json:"dirty,omitempty"`
	Ahead            int      `json:"ahead,omitempty"`
	WorkspaceEnabled bool     `json:"workspaceEnabled"`
	ConfigComplete   bool     `json:"configComplete"`
	Issues           []string `json:"issues"`
}

type LocalPackageChange struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type LocalPackageStash struct {
	Ref       string `json:"ref"`
	CreatedAt int64  `json:"createdAt"`
	Message   string `json:"message"`
}

type localPackageGitConfiguration struct {
	Repository string `json:"repository"`
	Branch     string `json:"branch"`
}

// ConfigureLocalPackageGit attaches a trusted backpack directory to a remote
// without replacing its files. For a non-Git directory it creates local refs
// for the chosen remote branch but deliberately does not check it out: the
// existing plugin code remains runnable until the user explicitly switches a
// version.
func (m Manager) ConfigureLocalPackageGit(root, name, raw string) (Result, error) {
	item, err := m.localPackage(root, name)
	if err != nil {
		return Result{}, err
	}
	if !item.Valid {
		return Result{}, errors.New("背包插件缺少有效 package.json")
	}
	var config localPackageGitConfiguration
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return Result{}, errors.New("Git 地址配置无效")
	}
	config.Repository = strings.TrimSpace(config.Repository)
	config.Branch = strings.TrimSpace(config.Branch)
	if err := validLocalPackageGitRemote(config.Repository); err != nil {
		return Result{}, err
	}
	if !isGitBranch(config.Branch) {
		return Result{}, errors.New("请选择有效的 Git 分支")
	}
	inside, gitErr := gitRun(item.Path, "rev-parse", "--is-inside-work-tree")
	newRepository := gitErr != nil || strings.TrimSpace(inside) != "true"
	if newRepository {
		if output, initErr := gitRun(item.Path, "init"); initErr != nil {
			return Result{Path: item.Path, Output: output}, fmt.Errorf("初始化插件 Git 工作区失败：%w", initErr)
		}
	}
	if output, remoteErr := gitRun(item.Path, "remote", "get-url", "origin"); remoteErr == nil && strings.TrimSpace(output) != "" {
		if output, setErr := gitRun(item.Path, "remote", "set-url", "origin", config.Repository); setErr != nil {
			return Result{Path: item.Path, Output: output}, fmt.Errorf("更新插件 Git 地址失败：%w", setErr)
		}
	} else if output, addErr := gitRun(item.Path, "remote", "add", "origin", config.Repository); addErr != nil {
		return Result{Path: item.Path, Output: output}, fmt.Errorf("保存插件 Git 地址失败：%w", addErr)
	}
	if !newRepository {
		branch, _ := gitRun(item.Path, "symbolic-ref", "--quiet", "--short", "HEAD")
		if isGitBranch(branch) && strings.TrimSpace(branch) != config.Branch {
			return Result{}, errors.New("Git 地址已保存；现有 Git 插件请在版本页切换分支")
		}
	}
	refspec := "+refs/heads/" + config.Branch + ":refs/remotes/origin/" + config.Branch
	output, fetchErr := gitRun(item.Path, "fetch", "origin", refspec)
	if fetchErr != nil {
		return Result{Path: item.Path, Output: output}, fmt.Errorf("Git 地址已保存，但无法拉取 %s 分支：%w", config.Branch, fetchErr)
	}
	if newRepository {
		if branchOutput, branchErr := gitRun(item.Path, "branch", "-f", config.Branch, "origin/"+config.Branch); branchErr != nil {
			return Result{Path: item.Path, Output: branchOutput}, fmt.Errorf("无法建立本地 Git 分支：%w", branchErr)
		}
		if headOutput, headErr := gitRun(item.Path, "symbolic-ref", "HEAD", "refs/heads/"+config.Branch); headErr != nil {
			return Result{Path: item.Path, Output: headOutput}, fmt.Errorf("无法设置插件 Git 分支：%w", headErr)
		}
	}
	return Result{Path: item.Path, Output: "已保存 Git 地址并拉取 " + config.Branch + " 分支。现有插件文件未被覆盖；请在版本页选择“切换”或“强制切换”后再替换代码。"}, nil
}

func validLocalPackageGitRemote(repository string) error {
	if repository == "" || strings.ContainsAny(repository, " \r\n\t") || strings.HasPrefix(repository, "-") {
		return errors.New("请填写有效的 Git 地址")
	}
	if strings.HasPrefix(repository, "git@") {
		if !strings.Contains(repository, ":") || strings.ContainsAny(repository, " ") {
			return errors.New("请填写有效的 SSH Git 地址")
		}
		return nil
	}
	if !strings.HasPrefix(repository, "https://") && !strings.HasPrefix(repository, "ssh://") && !strings.HasPrefix(repository, "file://") {
		return errors.New("Git 地址仅支持 HTTPS、SSH 或本地 file 地址")
	}
	return nil
}

func (m Manager) localPackage(root, name string) (LocalPackage, error) {
	items, err := m.LocalPackages(root)
	if err != nil {
		return LocalPackage{}, err
	}
	for _, item := range items {
		if item.Name == name {
			return item, nil
		}
	}
	return LocalPackage{}, errors.New("背包中没有这个本地插件包")
}

func (m Manager) LocalPackageStatus(root, name string) (LocalPackageStatus, error) {
	item, err := m.localPackage(root, name)
	if err != nil {
		return LocalPackageStatus{}, err
	}
	status := LocalPackageStatus{Name: item.Name, Source: "directory", Issues: []string{}}
	if !item.Valid {
		status.Issues = append(status.Issues, "缺少有效 package.json")
		return status, nil
	}
	enabled, err := m.EnabledApps(root)
	if err != nil {
		return LocalPackageStatus{}, err
	}
	status.Enabled = containsString(enabled, item.Name)
	project, err := projectPath(root)
	if err != nil {
		return LocalPackageStatus{}, err
	}
	status.WorkspaceEnabled = packageWorkspaceEnabled(filepath.Join(project, "package.json"))
	if !status.WorkspaceEnabled {
		status.Issues = append(status.Issues, "未启用 packages 工作空间")
	}
	if inside, gitErr := gitRun(item.Path, "rev-parse", "--is-inside-work-tree"); gitErr == nil && strings.TrimSpace(inside) == "true" {
		status.Source = "git"
		branch, _ := gitRun(item.Path, "symbolic-ref", "--quiet", "--short", "HEAD")
		status.Branch = strings.TrimSpace(branch)
		repository, _ := gitRun(item.Path, "remote", "get-url", "origin")
		status.Repository = strings.TrimSpace(repository)
		if !isGitBranch(status.Branch) {
			status.Issues = append(status.Issues, "当前未检出有效 Git 分支")
		}
		dirty, _ := backpackGitStatus(item.Path)
		status.Dirty = strings.TrimSpace(dirty) != ""
		if status.Dirty {
			status.Issues = append(status.Issues, "存在本地修改")
		}
		if release, releaseErr := releaseGitStatus(item.Path); releaseErr == nil {
			status.Ahead = release.Ahead
			if status.Ahead > 0 {
				status.Issues = append(status.Issues, fmt.Sprintf("本地领先远程 %d 个提交", status.Ahead))
			}
		}
	} else {
		status.Issues = append(status.Issues, "不是 Git 插件目录")
	}
	config, configErr := m.PackageConfig(root, item.Name)
	if configErr != nil {
		status.Issues = append(status.Issues, "无法读取插件配置")
	} else {
		status.ConfigComplete = true
		for _, field := range config.Fields {
			if field.Required && isConfigEmpty(config.Values[field.Name]) && !field.DefaultConfigured() {
				status.ConfigComplete = false
				status.Issues = append(status.Issues, "缺少必填配置："+field.Name)
			}
		}
	}
	return status, nil
}

// RefreshLocalPackageHistory turns a shallow backpack checkout into a complete
// history for its checked-out branch, then fetches the branch's latest remote
// refs. This is explicit because a normal version-list request must stay fast
// and should not unexpectedly download an entire repository.
func (m Manager) RefreshLocalPackageHistory(root, name string) (Result, error) {
	item, err := m.localPackage(root, name)
	if err != nil {
		return Result{}, err
	}
	if !item.Valid {
		return Result{}, errors.New("背包插件缺少有效 package.json")
	}
	inside, gitErr := gitRun(item.Path, "rev-parse", "--is-inside-work-tree")
	if gitErr != nil || strings.TrimSpace(inside) != "true" {
		return Result{}, errors.New("该插件不是 Git 工作区")
	}
	branch, branchErr := gitRun(item.Path, "symbolic-ref", "--quiet", "--short", "HEAD")
	branch = strings.TrimSpace(branch)
	if branchErr != nil || !isGitBranch(branch) {
		return Result{}, errors.New("当前插件未检出有效 Git 分支")
	}
	shallow, shallowErr := gitRun(item.Path, "rev-parse", "--is-shallow-repository")
	if shallowErr != nil {
		return Result{}, errors.New("无法读取插件 Git 浅克隆状态")
	}
	args := []string{"fetch", "origin", branch}
	output := ""
	if strings.TrimSpace(shallow) == "true" {
		args = []string{"fetch", "--unshallow", "origin", branch}
		output = "已解除浅克隆限制并拉取完整提交记录。"
	} else {
		output = "已拉取当前分支的最新提交记录。"
	}
	gitOutput, fetchErr := gitRun(item.Path, args...)
	if fetchErr != nil {
		return Result{Path: item.Path, Output: gitOutput}, fmt.Errorf("刷新当前分支提交记录失败：%w", fetchErr)
	}
	if gitOutput = strings.TrimSpace(gitOutput); gitOutput != "" {
		output += "\n" + gitOutput
	}
	return Result{Path: item.Path, Output: output}, nil
}

func (m Manager) LocalPackageStatuses(root string) ([]LocalPackageStatus, error) {
	items, err := m.LocalPackages(root)
	if err != nil {
		return nil, err
	}
	result := make([]LocalPackageStatus, 0, len(items))
	for _, item := range items {
		status, statusErr := m.LocalPackageStatus(root, item.Name)
		if statusErr != nil {
			status = LocalPackageStatus{Name: item.Name, Issues: []string{statusErr.Error()}}
		}
		result = append(result, status)
	}
	return result, nil
}

func packageWorkspaceEnabled(manifest string) bool {
	data, err := os.ReadFile(manifest)
	if err != nil {
		return false
	}
	var values map[string]any
	if json.Unmarshal(data, &values) != nil {
		return false
	}
	workspaces := values["workspaces"]
	switch value := workspaces.(type) {
	case []any:
		return workspaceContains(value, "packages/*")
	case map[string]any:
		packages, _ := workspacePackages(value)
		return workspaceContains(packages, "packages/*")
	default:
		return false
	}
}

func (m Manager) LocalPackageChanges(root, name string) ([]LocalPackageChange, error) {
	item, err := m.localPackage(root, name)
	if err != nil {
		return nil, err
	}
	if inside, gitErr := gitRun(item.Path, "rev-parse", "--is-inside-work-tree"); gitErr != nil || strings.TrimSpace(inside) != "true" {
		return nil, errors.New("该插件不是 Git 工作区")
	}
	output, err := backpackGitStatus(item.Path)
	if err != nil {
		return nil, errors.New("无法读取插件 Git 状态")
	}
	changes := []LocalPackageChange{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if len(line) < 4 {
			continue
		}
		changes = append(changes, LocalPackageChange{Status: strings.TrimSpace(line[:2]), Path: strings.TrimSpace(line[3:])})
	}
	return changes, nil
}

func (m Manager) LocalPackageDiff(root, name, changePath string) (GitDiffResult, error) {
	item, err := m.localPackage(root, name)
	if err != nil {
		return GitDiffResult{}, err
	}
	return gitDiffAtRepository(item.Path, changePath)
}

func (m Manager) LocalPackageStashes(root, name string) ([]LocalPackageStash, error) {
	item, err := m.localPackage(root, name)
	if err != nil {
		return nil, err
	}
	output, err := gitRun(item.Path, "stash", "list", "--format=%gd%x1f%ct%x1f%s")
	if err != nil {
		return nil, errors.New("无法读取插件 stash")
	}
	result := []LocalPackageStash{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		parts := strings.SplitN(line, "\x1f", 3)
		if len(parts) != 3 {
			continue
		}
		var timestamp int64
		fmt.Sscanf(parts[1], "%d", &timestamp)
		result = append(result, LocalPackageStash{Ref: parts[0], CreatedAt: timestamp * 1000, Message: parts[2]})
	}
	return result, nil
}

func (m Manager) StashAndSyncLocalPackage(root, name, version string) (Result, error) {
	item, err := m.localPackage(root, name)
	if err != nil {
		return Result{}, err
	}
	status, err := releaseGitStatus(item.Path)
	if err != nil {
		return Result{}, err
	}
	logs := []string{}
	if status.Dirty {
		if output, err := gitRun(item.Path, "stash", "push", "--include-untracked", "-m", "AlemonX 同步前保存"); err != nil {
			return Result{}, errors.New("无法保存插件本地修改到 stash")
		} else {
			logs = append(logs, "已保存本地修改到 Git stash。", output)
		}
	}
	if status.Ahead > 0 {
		backup := "alx-backup-" + time.Now().Format("20060102-150405")
		if output, err := gitRun(item.Path, "branch", backup, "HEAD"); err != nil {
			return Result{Path: item.Path, Output: strings.Join(logs, "\n") + "\n" + output}, errors.New("无法为本地领先提交创建备份分支")
		}
		logs = append(logs, "已创建本地提交备份分支 "+backup+"。")
	}
	result, err := switchLocalPackageVersion(root, name, version, true)
	if err != nil {
		return result, err
	}
	result.Output = strings.TrimSpace(strings.Join(append(logs, result.Output), "\n"))
	return result, nil
}

func (m Manager) ApplyLocalPackageStash(root, name, ref string, drop bool) (Result, error) {
	item, err := m.localPackage(root, name)
	if err != nil {
		return Result{}, err
	}
	if !stashRefValid(ref) {
		return Result{}, errors.New("stash 标识无效")
	}
	args := []string{"stash", "apply", "--index", ref}
	if drop {
		args = []string{"stash", "pop", "--index", ref}
	}
	output, err := gitRun(item.Path, args...)
	if err != nil {
		return Result{Path: item.Path, Output: output}, errors.New("恢复 stash 失败；请先处理当前本地修改")
	}
	return Result{Path: item.Path, Output: "已恢复 " + ref + "。\n" + output}, nil
}

func (m Manager) DropLocalPackageStash(root, name, ref string) (Result, error) {
	item, err := m.localPackage(root, name)
	if err != nil {
		return Result{}, err
	}
	if !stashRefValid(ref) {
		return Result{}, errors.New("stash 标识无效")
	}
	output, err := gitRun(item.Path, "stash", "drop", ref)
	if err != nil {
		return Result{Path: item.Path, Output: output}, errors.New("删除 stash 失败")
	}
	return Result{Path: item.Path, Output: "已删除 " + ref + "。"}, nil
}

func stashRefValid(ref string) bool {
	return len(ref) >= 8 && strings.HasPrefix(ref, "stash@{") && strings.HasSuffix(ref, "}")
}

// RemoveLocalPackageAndDisable removes the apps entry as part of the same
// workflow. The directory is staged first so a config failure can be rolled
// back without losing the plugin.
func (m Manager) RemoveLocalPackageAndDisable(root, name string) (Result, error) {
	item, err := m.localPackage(root, name)
	if err != nil {
		return Result{}, err
	}
	staged := item.Path + ".alx-removing-" + fmt.Sprint(time.Now().UnixNano())
	if err := os.Rename(item.Path, staged); err != nil {
		return Result{}, fmt.Errorf("无法准备移除插件：%w", err)
	}
	rollback := func(cause error) (Result, error) {
		if restoreErr := os.Rename(staged, item.Path); restoreErr != nil {
			return Result{}, fmt.Errorf("%v；且无法恢复插件目录：%w", cause, restoreErr)
		}
		return Result{}, cause
	}
	_, err = m.UpdateRuntimeConfig(root, "", func(content string) (string, error) {
		content = normalizeRuntimeConfigYAML(content)
		config := map[string]any{}
		if strings.TrimSpace(content) != "" {
			if err := yaml.Unmarshal([]byte(content), &config); err != nil {
				return "", fmt.Errorf("无法解析运行配置：%w", err)
			}
		}
		apps := appsFromConfig(config["apps"])
		delete(apps, name)
		return setTopLevelBooleanMap(content, "apps", apps), nil
	})
	if err != nil {
		return rollback(err)
	}
	if err := os.RemoveAll(staged); err != nil {
		return Result{Path: staged}, fmt.Errorf("已取消启用，但无法删除插件目录；目录保留在 %s：%w", staged, err)
	}
	return Result{Path: item.Path, Output: "已从背包移除 " + name + "，并取消启用。"}, nil
}
