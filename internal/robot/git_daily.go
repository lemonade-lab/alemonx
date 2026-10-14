package robot

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type GitStash struct {
	SHA     string `json:"sha"`
	Ref     string `json:"ref"`
	Subject string `json:"subject"`
}

type GitWorkspaceActionOptions struct {
	ExpectedBranch string   `json:"expectedBranch,omitempty"`
	Paths          []string `json:"paths,omitempty"`
	IndexRevision  string   `json:"indexRevision,omitempty"`
	Remote         string   `json:"remote,omitempty"`
}

var gitWorkspaceLocks sync.Map

func lockGitWorkspace(path string) (func(), error) {
	root, err := gitRun(path, "rev-parse", "--git-common-dir")
	if err != nil {
		return nil, errors.New("当前目录不是 Git 仓库")
	}
	if !filepath.IsAbs(root) {
		root = filepath.Join(path, root)
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
	}
	value, _ := gitWorkspaceLocks.LoadOrStore(root, &sync.Mutex{})
	mutex := value.(*sync.Mutex)
	mutex.Lock()
	return mutex.Unlock, nil
}

// Porcelain's leading space and NUL delimiters are data, not whitespace.
func gitRawOutput(root string, args ...string) (string, error) {
	return runCommandOutput(root, map[string]string{"GIT_TERMINAL_PROMPT": "0"}, false, false, "git", args...)
}

func gitWorkspaceChanges(path string) ([]GitChange, error) {
	prefix, err := gitWorkspacePrefix(path)
	if err != nil {
		return nil, err
	}
	output, err := gitRawOutput(path, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--", ".")
	if err != nil {
		return nil, err
	}
	changes := []GitChange{}
	fields := strings.Split(output, "\x00")
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 4 {
			continue
		}
		code, name := entry[:2], entry[3:]
		original := ""
		if strings.ContainsAny(code, "RC") {
			i++
			if i >= len(fields) {
				return nil, errors.New("无法解析 Git 重命名状态")
			}
			original = fields[i]
		}
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		name = strings.TrimPrefix(name, prefix)
		if original != "" {
			if strings.HasPrefix(original, prefix) {
				original = strings.TrimPrefix(original, prefix)
			} else {
				// Keep the boundary visible; staging this cross-project rename is refused.
				original = "../" + original
			}
		}
		conflict := code == "DD" || code == "AA" || strings.Contains(code, "U")
		changes = append(changes, GitChange{Status: code, Path: name, OriginalPath: original,
			Staged:   code[0] != ' ' && code[0] != '?' && !conflict,
			Unstaged: code[1] != ' ', Conflict: conflict})
	}
	return changes, nil
}

func gitWorkspaceChangesAtRepository(path string) ([]GitChange, error) {
	root, err := gitWorkspaceRoot(path)
	if err != nil {
		return nil, err
	}
	return gitWorkspaceChanges(root)
}

func ensureScopedGitIndex(path string) error {
	prefix, err := gitWorkspacePrefix(path)
	if err != nil {
		return err
	}
	files, err := gitRawOutput(path, "diff", "--cached", "--no-renames", "--name-only", "-z")
	if err != nil {
		return err
	}
	for _, file := range strings.Split(files, "\x00") {
		if file != "" && !strings.HasPrefix(file, prefix) {
			return errors.New("父仓库还有当前项目以外的已暂存文件；请在仓库根目录处理后再提交，避免混入其他项目的改动")
		}
	}
	return nil
}

func gitWorkspaceFileAction(path, action string, paths []string) (Result, error) {
	if len(paths) == 0 {
		return Result{}, errors.New("请先选择变更文件")
	}
	changes, err := gitWorkspaceChanges(path)
	if err != nil {
		return Result{}, err
	}
	known := map[string]GitChange{}
	for _, change := range changes {
		known[change.Path] = change
	}
	args := []string{}
	switch action {
	case "stage":
		args = []string{"add", "-A", "--"}
	case "unstage":
		if _, err := gitRun(path, "rev-parse", "--verify", "HEAD"); err == nil {
			args = []string{"reset", "--quiet", "HEAD", "--"}
		} else {
			args = []string{"rm", "--cached", "--quiet", "--"}
		}
	case "restore":
		args = []string{"restore", "--worktree", "--"}
	}
	seen := map[string]bool{}
	for _, name := range paths {
		clean, err := cleanGitChangePath(name)
		if err != nil || clean != name {
			return Result{}, errors.New("变更文件路径无效")
		}
		change, exists := known[name]
		if !exists {
			return Result{}, errors.New("所选文件的状态已变化，请刷新后重试")
		}
		if action == "unstage" && !change.Staged {
			return Result{}, errors.New("所选文件尚未暂存")
		}
		if action == "restore" && (change.Status == "??" || change.Conflict) {
			return Result{}, errors.New("只支持撤销已跟踪文件的未暂存改动；新增文件或冲突文件请单独处理")
		}
		names := []string{name}
		if action != "restore" && change.OriginalPath != "" {
			names = append(names, change.OriginalPath)
		}
		for _, item := range names {
			if _, err := cleanGitChangePath(item); err != nil {
				return Result{}, errors.New("重命名跨越当前项目边界，请在仓库根目录处理")
			}
			if !seen[item] {
				args = append(args, ":(literal)"+item)
				seen[item] = true
			}
		}
	}
	output, err := gitRun(path, args...)
	return Result{Path: path, Output: output}, err
}

func gitWorkspaceRemote(path, selected string) (string, error) {
	if selected == "" {
		branch := currentBranch(path)
		selected, _ = gitRun(path, "config", "--get", "branch."+branch+".remote")
		if selected == "." {
			selected = ""
		}
		if selected == "" {
			for _, name := range gitLines(path, "remote") {
				if selected == "" || name == "origin" {
					selected = name
				}
			}
		}
	}
	if !validRemoteName(selected) {
		return "", errors.New("请先配置并选择远程仓库")
	}
	if _, err := gitRun(path, "remote", "get-url", selected); err != nil {
		return "", errors.New("所选远程仓库不存在，请刷新后重试")
	}
	return selected, nil
}

func gitWorkspaceNetworkAction(path, action, value, selected string) (Result, error) {
	remote, err := gitWorkspaceRemote(path, selected)
	if err != nil {
		return Result{}, err
	}
	args := []string{}
	switch action {
	case "fetch":
		if shallow, _ := gitRun(path, "rev-parse", "--is-shallow-repository"); shallow == "true" {
			if output, err := gitRun(path, "fetch", "--unshallow", remote); err != nil {
				return Result{Path: path, Output: output}, err
			}
		}
		args = []string{"fetch", "--prune", "--tags", remote, "+refs/heads/*:refs/remotes/" + remote + "/*"}
	case "tag-push":
		if !validGitRef(value) {
			return Result{}, errors.New("标签名称无效")
		}
		args = []string{"push", remote, "refs/tags/" + value + ":refs/tags/" + value}
	case "push", "pull":
		branch := currentBranch(path)
		if branch == "" {
			return Result{}, errors.New("当前处于分离 HEAD 状态，请先切换到本地分支")
		}
		if action == "pull" && value != "" && value != branch {
			return Result{}, errors.New("更新目标不是当前分支，请先切换到所选分支再更新")
		}
		target := "refs/heads/" + branch
		if upstreamRemote, _ := gitRun(path, "config", "--get", "branch."+branch+".remote"); upstreamRemote == remote {
			if ref, err := gitRun(path, "config", "--get", "branch."+branch+".merge"); err == nil && strings.HasPrefix(ref, "refs/heads/") {
				target = ref
			}
		}
		if action == "pull" {
			changes, err := gitWorkspaceChangesAtRepository(path)
			if err != nil {
				return Result{}, err
			}
			if len(changes) > 0 {
				return Result{}, errors.New("请先提交或临时保存整个仓库的改动，再同步远程更新")
			}
			args = []string{"pull", "--ff-only", remote, target}
		} else {
			args = []string{"push", "-u", remote, "HEAD:" + target}
		}
	}
	output, err := runWithEnv(path, map[string]string{"GIT_TERMINAL_PROMPT": "0"}, "git", args...)
	if err != nil && action == "pull" && (strings.Contains(output, "fast-forward") || strings.Contains(output, "divergent")) {
		return Result{Path: path, Output: output}, fmt.Errorf("本地与远程分支已分叉；请获取远程更新，在“分支”页检出远程分支后合并，再同步：%w", err)
	}
	return Result{Path: path, Output: output}, err
}

func gitWorkspaceStashAction(path, action, sha, message string) (Result, error) {
	root, err := gitWorkspaceRoot(path)
	if err != nil {
		return Result{}, err
	}
	if !sameWorkspacePath(root, path) {
		return Result{}, errors.New("临时保存和恢复作用于整个仓库，请从仓库根目录打开 Git 管理")
	}
	args := []string{}
	if action == "stash-save" {
		if strings.TrimSpace(message) == "" {
			message = "ALemonX 临时保存"
		}
		args = []string{"stash", "push", "--include-untracked", "-m", message}
	} else {
		ref := ""
		for _, line := range gitLines(path, "stash", "list", "--format=%H%x1f%gd") {
			parts := strings.SplitN(line, "\x1f", 2)
			if len(parts) == 2 && parts[0] == sha {
				ref = parts[1]
				break
			}
		}
		if ref == "" {
			return Result{}, errors.New("临时保存记录已变化，请刷新后重试")
		}
		if action == "stash-apply" {
			changes, err := gitWorkspaceChanges(path)
			if err != nil {
				return Result{}, err
			}
			if len(changes) > 0 {
				return Result{}, errors.New("请先提交或保存当前改动，再恢复临时保存记录")
			}
			// Apply preserves the recovery point even if conflicts occur.
			args = []string{"stash", "apply", "--index", sha}
		} else {
			args = []string{"stash", "drop", ref}
		}
	}
	output, err := gitRun(path, args...)
	return Result{Path: path, Output: output}, err
}

func gitWorkspaceIndexRevision(path string) (string, error) {
	raw, err := gitRawOutput(path, "diff", "--cached", "--no-ext-diff", "--no-renames", "--raw", "-z", "--abbrev=64")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(raw))), nil
}

func gitWorkspaceHistoryPage(path string, offset int) ([]GitCommit, bool, error) {
	commits := []GitCommit{}
	if _, err := gitRun(path, "rev-parse", "--verify", "HEAD"); err != nil {
		return commits, false, nil
	}
	output, err := gitRun(path, "log", "HEAD", "--format=%H%x1f%h%x1f%s%x1f%cs", "-21", fmt.Sprintf("--skip=%d", offset))
	if err != nil {
		return nil, false, err
	}
	for _, line := range strings.Split(output, "\n") {
		parts := strings.Split(line, "\x1f")
		if len(parts) == 4 {
			commits = append(commits, GitCommit{SHA: parts[0], ShortSHA: parts[1], Subject: parts[2], CreatedAt: parts[3]})
		}
	}
	more := len(commits) > 20
	if more {
		commits = commits[:20]
	}
	return commits, more, nil
}

func GitWorkspaceCommit(root, sha string) (Result, error) {
	path, err := workspacePath(root)
	if err != nil {
		return Result{}, err
	}
	if !sourceCommitPattern.MatchString(sha) {
		return Result{}, errors.New("提交编号无效")
	}
	commit, err := gitRun(path, "rev-parse", "--verify", sha+"^{commit}")
	if err != nil {
		return Result{}, errors.New("提交不存在，请刷新后重试")
	}
	output, err := gitRawOutput(path, "show", "--no-ext-diff", "--no-color", "--format=fuller", "--stat", "--patch", commit, "--", ".")
	return Result{Path: path, Output: truncateGitDiff(output)}, err
}

func gitWorkspacePrefix(path string) (string, error) {
	output, err := gitRawOutput(path, "rev-parse", "--show-prefix")
	return strings.TrimSuffix(output, "\n"), err
}
func gitWorkspaceRoot(path string) (string, error) {
	output, err := gitRawOutput(path, "rev-parse", "--show-toplevel")
	return strings.TrimSuffix(output, "\n"), err
}
