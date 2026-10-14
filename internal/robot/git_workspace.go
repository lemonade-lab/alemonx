package robot

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// GitWorkspaceStatus describes source-control state only. It intentionally has
// no package-build or release-branch checks: Git management and publishing are
// separate user workflows.
type GitWorkspaceStatus struct {
	Root             string            `json:"root"`
	Repository       bool              `json:"repository"`
	AtRepositoryRoot bool              `json:"atRepositoryRoot"`
	GitRoot          string            `json:"gitRoot,omitempty"`
	RemoteName       string            `json:"remoteName,omitempty"`
	RemoteAdvice     string            `json:"remoteAdvice,omitempty"`
	Stashes          []GitStash        `json:"stashes"`
	Remote           string            `json:"remote,omitempty"`
	Branch           string            `json:"branch,omitempty"`
	Upstream         string            `json:"upstream,omitempty"`
	RemoteReachable  bool              `json:"remoteReachable"`
	RemoteSynced     bool              `json:"remoteSynced"`
	RemoteChecked    bool              `json:"remoteChecked"`
	Ahead            int               `json:"ahead"`
	Behind           int               `json:"behind"`
	AuthorName       string            `json:"authorName,omitempty"`
	AuthorEmail      string            `json:"authorEmail,omitempty"`
	IndexRevision    string            `json:"indexRevision,omitempty"`
	MergeInProgress  bool              `json:"mergeInProgress"`
	HistoryHasMore   bool              `json:"historyHasMore"`
	Changes          []GitChange       `json:"changes"`
	Branches         []GitBranch       `json:"branches"`
	RemoteBranches   []GitRemoteBranch `json:"remoteBranches"`
	Commits          []GitCommit       `json:"commits"`
	Tags             []GitTag          `json:"tags"`
	Remotes          []GitRemote       `json:"remotes"`
}

type GitBranch struct {
	Name     string `json:"name"`
	Current  bool   `json:"current"`
	Upstream string `json:"upstream,omitempty"`
}

// GitRemoteBranch is a branch advertised by a remote. It is read from the
// local remote-tracking refs after `fetch`; selecting it creates a normal
// local tracking branch, never a detached HEAD.
type GitRemoteBranch struct {
	Name   string `json:"name"`
	Remote string `json:"remote"`
	Branch string `json:"branch"`
}

type GitTag struct {
	Name      string `json:"name"`
	Subject   string `json:"subject,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
}

type GitRemote struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type GitChange struct {
	Status       string `json:"status"`
	Path         string `json:"path"`
	OriginalPath string `json:"originalPath,omitempty"`
	Staged       bool   `json:"staged"`
	Unstaged     bool   `json:"unstaged"`
	Conflict     bool   `json:"conflict"`
}

func GitWorkspace(root string) (GitWorkspaceStatus, error) { return GitWorkspaceView(root, "all") }

// GitWorkspaceView loads only the data needed by one tab. Large histories,
// tag lists and remote branch lists are therefore not read when opening the
// default "提交" tab.
func GitWorkspaceView(root, view string) (GitWorkspaceStatus, error) {
	return GitWorkspaceViewForRemote(root, view, "")
}

func GitWorkspaceViewForRemote(root, view, remote string) (GitWorkspaceStatus, error) {
	return GitWorkspaceViewWithOptions(root, view, remote, 0)
}

func GitWorkspaceViewWithOptions(root, view, remote string, offset int) (GitWorkspaceStatus, error) {
	if offset < 0 || offset > 100000 {
		return GitWorkspaceStatus{}, errors.New("提交记录页码无效")
	}
	path, err := workspacePath(root)
	if err != nil {
		return GitWorkspaceStatus{}, err
	}
	status := GitWorkspaceStatus{Root: path, Changes: []GitChange{}, Branches: []GitBranch{}, RemoteBranches: []GitRemoteBranch{}, Commits: []GitCommit{}, Tags: []GitTag{}, Remotes: []GitRemote{}}
	inside, err := gitRun(path, "rev-parse", "--is-inside-work-tree")
	if err != nil || inside != "true" {
		return status, nil
	}
	status.Repository = true
	status.GitRoot, _ = gitWorkspaceRoot(path)
	status.AtRepositoryRoot = sameWorkspacePath(path, status.GitRoot)
	status.Branch, _ = gitRun(path, "branch", "--show-current")
	status.AuthorName, _ = gitRun(path, "config", "--get", "user.name")
	status.AuthorEmail, _ = gitRun(path, "config", "--get", "user.email")
	if _, err := gitRun(path, "rev-parse", "--verify", "MERGE_HEAD"); err == nil {
		status.MergeInProgress = true
	}
	var loadError error
	loadChanges := func() {
		status.Changes, loadError = gitWorkspaceChanges(path)
		status.IndexRevision, _ = gitWorkspaceIndexRevision(path)
		if status.Changes == nil {
			status.Changes = []GitChange{}
		}
		status.Stashes = []GitStash{}
		for _, line := range gitLines(path, "stash", "list", "--format=%H%x1f%gd%x1f%s") {
			parts := strings.SplitN(line, "\x1f", 3)
			if len(parts) == 3 {
				status.Stashes = append(status.Stashes, GitStash{SHA: parts[0], Ref: parts[1], Subject: parts[2]})
			}
		}
	}
	loadBranches := func() {
		for _, line := range gitLines(path, "for-each-ref", "--format=%(refname:short)|%(upstream:short)", "refs/heads") {
			parts := strings.SplitN(line, "|", 2)
			branch := GitBranch{Name: parts[0], Current: parts[0] == status.Branch}
			if len(parts) == 2 {
				branch.Upstream = parts[1]
			}
			status.Branches = append(status.Branches, branch)
		}
		for _, name := range gitLines(path, "for-each-ref", "--format=%(refname:short)", "refs/remotes") {
			parts := strings.SplitN(name, "/", 2)
			if len(parts) == 2 && parts[1] != "HEAD" {
				status.RemoteBranches = append(status.RemoteBranches, GitRemoteBranch{Name: name, Remote: parts[0], Branch: parts[1]})
			}
		}
	}
	loadTags := func() {
		for _, name := range gitLines(path, "tag", "--list", "--sort=-v:refname") {
			tag := GitTag{Name: name}
			if detail, err := gitRun(path, "for-each-ref", "--format=%(contents:subject)|%(creatordate:iso-strict)", "refs/tags/"+name); err == nil {
				parts := strings.SplitN(detail, "|", 2)
				tag.Subject = parts[0]
				if len(parts) == 2 {
					tag.CreatedAt = parts[1]
				}
			}
			status.Tags = append(status.Tags, tag)
		}
	}
	loadRemotes := func(check bool) {
		for _, name := range gitLines(path, "remote") {
			url, _ := gitRun(path, "remote", "get-url", name)
			status.Remotes = append(status.Remotes, GitRemote{Name: name, URL: url})
		}
		if upstream, err := gitRun(path, "rev-parse", "--abbrev-ref", "@{upstream}"); err == nil {
			status.Upstream = upstream
			if counts, err := gitRun(path, "rev-list", "--left-right", "--count", "HEAD...@{upstream}"); err == nil {
				_, _ = fmt.Sscanf(counts, "%d\t%d", &status.Ahead, &status.Behind)
			}
		}
		name, err := gitWorkspaceRemote(path, remote)
		if err != nil {
			return
		}
		status.RemoteName = name
		status.Remote, _ = gitRun(path, "remote", "get-url", name)
		if !check {
			return
		}
		status.RemoteChecked = true
		output, err := runWithEnv(path, map[string]string{"GIT_TERMINAL_PROMPT": "0"}, "git", "ls-remote", "--heads", name)
		if err != nil {
			status.RemoteAdvice = strings.ReplaceAll(remoteAdvice(output), "origin", name)
			return
		}
		status.RemoteReachable = true
		mergeRef := "refs/heads/" + status.Branch
		if upstreamRemote, _ := gitRun(path, "config", "--get", "branch."+status.Branch+".remote"); upstreamRemote == name {
			if ref, err := gitRun(path, "config", "--get", "branch."+status.Branch+".merge"); err == nil {
				mergeRef = ref
			}
		}
		head, _ := gitRun(path, "rev-parse", "HEAD")
		for _, line := range strings.Split(output, "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && fields[1] == mergeRef {
				status.RemoteSynced = head == fields[0]
			}
		}
	}

	switch view {
	case "commit":
		loadChanges()
		loadRemotes(false)
	case "history":
		status.Commits, status.HistoryHasMore, loadError = gitWorkspaceHistoryPage(path, offset)
	case "tag":
		loadTags()
		loadRemotes(false)
	case "branch":
		loadBranches()
		loadRemotes(view == "remote" || view == "all")
	case "remote":
		loadRemotes(view == "remote" || view == "all")
	case "all":
		loadChanges()
		loadBranches()
		status.Commits = sourceCommits(path)
		loadTags()
		loadRemotes(view == "remote" || view == "all")
	default:
		return GitWorkspaceStatus{}, errors.New("不支持的 Git 视图")
	}
	return status, loadError
}

// GitWorkspaceAction only exposes named Git operations. It never executes a
// browser-provided shell command.
func GitWorkspaceAction(root, action, value, message string) (Result, error) {
	return GitWorkspaceActionWithOptions(root, action, value, message, GitWorkspaceActionOptions{})
}

func GitWorkspaceActionWithOptions(root, action, value, message string, options GitWorkspaceActionOptions) (Result, error) {
	path, err := workspacePath(root)
	if err != nil {
		return Result{}, err
	}
	unlock, err := lockGitWorkspace(path)
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	value = strings.TrimSpace(value)
	if options.ExpectedBranch != "" && currentBranch(path) != options.ExpectedBranch {
		return Result{}, errors.New("当前分支在确认期间发生变化，请刷新后重新确认操作")
	}
	inside, err := gitRun(path, "rev-parse", "--is-inside-work-tree")
	if err != nil || inside != "true" {
		return Result{}, errors.New("当前机器人目录尚未初始化 Git")
	}
	switch action {
	case "fetch", "pull", "push", "tag-push":
		return gitWorkspaceNetworkAction(path, action, value, options.Remote)
	case "stage", "unstage", "restore":
		return gitWorkspaceFileAction(path, action, options.Paths)
	case "commit":
		if strings.TrimSpace(message) == "" {
			return Result{}, errors.New("请填写提交说明")
		}
		if options.IndexRevision != "" {
			revision, err := gitWorkspaceIndexRevision(path)
			if err != nil {
				return Result{}, err
			}
			if revision != options.IndexRevision {
				return Result{}, errors.New("已暂存内容在确认期间发生变化，请刷新并重新审查后提交")
			}
		}
		if err := ensureScopedGitIndex(path); err != nil {
			return Result{}, err
		}
		staged, err := gitRawOutput(path, "diff", "--cached", "--name-only", "-z", "--", ".")
		if err != nil {
			return Result{}, err
		}
		if staged == "" {
			return Result{}, errors.New("没有已暂存的变更，请先选择文件并暂存")
		}
		output, err := gitRun(path, "commit", "-m", message)
		return Result{Path: path, Output: output}, err
	case "stash-save", "stash-apply", "stash-drop":
		return gitWorkspaceStashAction(path, action, value, message)
	case "merge", "merge-abort":
		if action == "merge-abort" {
			output, err := gitRun(path, "merge", "--abort")
			return Result{Path: path, Output: output}, err
		}
		if !validGitRef(value) {
			return Result{}, errors.New("分支名称无效")
		}
		changes, err := gitWorkspaceChangesAtRepository(path)
		if err != nil {
			return Result{}, err
		}
		if len(changes) > 0 {
			return Result{}, errors.New("请先提交或临时保存整个仓库的改动，再合并分支")
		}
		ref := "refs/heads/" + value
		if strings.HasPrefix(value, "refs/remotes/") {
			ref = value
		}
		if _, err := gitRun(path, "show-ref", "--verify", ref); err != nil {
			return Result{}, errors.New("所选分支不存在，请获取远程分支后重试")
		}
		output, err := gitRun(path, "merge", "--no-edit", ref)
		return Result{Path: path, Output: output}, err
	case "branch-create":
		if !validGitRef(value) {
			return Result{}, errors.New("分支名称无效")
		}
		output, err := gitRun(path, "switch", "-c", value)
		return Result{Path: path, Output: output}, err
	case "branch-switch":
		if !validGitRef(value) {
			return Result{}, errors.New("分支名称无效")
		}
		output, err := gitRun(path, "switch", value)
		return Result{Path: path, Output: output}, err
	case "branch-track":
		if !validRemoteBranch(value) {
			return Result{}, errors.New("远程分支名称无效")
		}
		parts := strings.SplitN(value, "/", 2)
		output, err := gitRun(path, "switch", "--track", "-c", parts[1], value)
		return Result{Path: path, Output: output}, err
	case "branch-delete":
		if !validGitRef(value) || value == currentBranch(path) {
			return Result{}, errors.New("不能删除当前分支，或分支名称无效")
		}
		output, err := gitRun(path, "branch", "-d", value)
		return Result{Path: path, Output: output}, err
	case "tag-create":
		if !validGitRef(value) {
			return Result{}, errors.New("标签名称无效")
		}
		message = strings.TrimSpace(message)
		if message == "" || strings.ContainsAny(message, "\r\n") {
			return Result{}, errors.New("请填写单行标签说明")
		}
		output, err := gitRun(path, "tag", "-a", value, "-m", message)
		return Result{Path: path, Output: output}, err
	case "tag-delete":
		if !validGitRef(value) {
			return Result{}, errors.New("标签名称无效")
		}
		output, err := gitRun(path, "tag", "-d", value)
		return Result{Path: path, Output: output}, err
	case "identity-set":
		name, email := strings.TrimSpace(value), strings.TrimSpace(message)
		if name == "" || email == "" || strings.ContainsAny(name+email, "\r\n") {
			return Result{}, errors.New("请填写有效的 Git 提交姓名和邮箱")
		}
		if _, err := gitRun(path, "config", "--local", "user.name", name); err != nil {
			return Result{}, err
		}
		output, err := gitRun(path, "config", "--local", "user.email", email)
		return Result{Path: path, Output: output}, err
	case "remote-add", "remote-set-url":
		message = strings.TrimSpace(message)
		if !validRemoteName(value) || !validRemoteURL(message) {
			return Result{}, errors.New("远程名称或仓库地址无效")
		}
		args := []string{"remote", "add", value, message}
		if action == "remote-set-url" {
			args = []string{"remote", "set-url", value, message}
		}
		output, err := gitRun(path, args...)
		return Result{Path: path, Output: output}, err
	case "remote-remove":
		if !validRemoteName(value) {
			return Result{}, errors.New("远程名称无效")
		}
		output, err := gitRun(path, "remote", "remove", value)
		return Result{Path: path, Output: output}, err
	default:
		return Result{}, errors.New("不支持的 Git 操作")
	}
}

var gitRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
var gitRemotePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func validGitRef(value string) bool {
	value = strings.TrimSpace(value)
	return gitRefPattern.MatchString(value) && !strings.Contains(value, "..") && !strings.HasSuffix(value, ".") && !strings.HasSuffix(value, "/")
}

func validRemoteName(value string) bool {
	return gitRemotePattern.MatchString(strings.TrimSpace(value))
}

func validRemoteURL(value string) bool {
	value = strings.TrimSpace(value)
	return (strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "ssh://") || strings.Contains(value, "@")) && !strings.ContainsAny(value, "\r\n \t")
}

func currentBranch(root string) string {
	branch, _ := gitRun(root, "branch", "--show-current")
	return branch
}

func validRemoteBranch(value string) bool {
	parts := strings.SplitN(strings.TrimSpace(value), "/", 2)
	return len(parts) == 2 && validRemoteName(parts[0]) && validGitRef(parts[1])
}
