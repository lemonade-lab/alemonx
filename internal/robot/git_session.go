package robot

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type GitBuildSession struct {
	ID        string    `json:"sessionId"`
	Branch    string    `json:"branch"`
	Commit    string    `json:"commit"`
	Target    string    `json:"target"`
	Files     []string  `json:"files"`
	Logs      string    `json:"logs"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}
type gitBuildState struct {
	GitBuildSession
	root           string
	worktree       string
	publishing     bool
	branchPushed   bool
	releaseCommit  string
	releaseVersion string
}

var gitBuildSessions = struct {
	sync.Mutex
	items map[string]gitBuildState
}{items: map[string]gitBuildState{}}

func PrepareGitBuild(root, branch, commit string) (GitBuildSession, error) {
	path, err := workspacePath(root)
	if err != nil {
		return GitBuildSession{}, err
	}
	syncOutput, err := syncPublishRemote(path)
	if err != nil {
		return GitBuildSession{}, err
	}
	status, err := GitReleaseStatus(path)
	if err != nil || len(status.Issues) > 0 {
		if err != nil {
			return GitBuildSession{}, err
		}
		return GitBuildSession{}, errors.New("发布前检查未通过：" + strings.Join(status.Issues, "；"))
	}
	if branch == "" {
		branch = status.Branch
	}
	if !validGitRef(branch) {
		return GitBuildSession{}, errors.New("请选择有效的源码分支")
	}
	branchRef, err := sourceBranchRef(path, branch)
	if err != nil {
		return GitBuildSession{}, err
	}
	commit, err = gitRun(path, "rev-parse", "--verify", commit+"^{commit}")
	if err != nil {
		return GitBuildSession{}, errors.New("所选提交不存在")
	}
	if _, err = gitRun(path, "merge-base", "--is-ancestor", commit, branchRef); err != nil {
		return GitBuildSession{}, errors.New("所选提交不属于源码分支")
	}
	worktree, err := os.MkdirTemp("", "alx-git-build-")
	if err != nil {
		return GitBuildSession{}, err
	}
	cleanup := func() { _, _ = gitRun(path, "worktree", "remove", "--force", worktree); _ = os.RemoveAll(worktree) }
	output := syncOutput
	worktreeOutput, err := gitRun(path, "worktree", "add", "--detach", worktree, commit)
	output = strings.TrimSpace(output + "\n" + worktreeOutput)
	if err != nil {
		cleanup()
		return GitBuildSession{}, err
	}
	install, err := runPackageManager(worktree, "install")
	output += "\n" + install
	if err != nil {
		cleanup()
		return GitBuildSession{}, buildDependencyError("安装构建依赖失败", output, err)
	}
	buildKind, buildScript := resolveBuildScript(worktree)
	nestedInstall, err := installBuildSubprojects(worktree, buildScript)
	output += "\n" + nestedInstall
	if err != nil {
		cleanup()
		return GitBuildSession{}, buildDependencyError("安装前端构建依赖失败", output, err)
	}
	build, err := runResolvedBuild(worktree, buildKind, buildScript)
	output += "\n" + build
	if err != nil {
		cleanup()
		return GitBuildSession{}, buildDependencyError("构建失败", output, err)
	}
	files := scanPublishFiles(worktree, "")
	if len(files) == 0 {
		cleanup()
		return GitBuildSession{}, errors.New("构建后没有可发布的产物")
	}
	bytes := make([]byte, 12)
	_, _ = rand.Read(bytes)
	id := hex.EncodeToString(bytes)
	createdAt := time.Now()
	session := GitBuildSession{ID: id, Branch: branch, Commit: commit, Target: releaseBranchName(branch, status.RemoteBranch), Files: files, Logs: strings.TrimSpace(output), CreatedAt: createdAt, ExpiresAt: createdAt.Add(30 * time.Minute)}
	gitBuildSessions.Lock()
	cleanupGitBuildSessions()
	gitBuildSessions.items[id] = gitBuildState{GitBuildSession: session, root: path, worktree: worktree}
	gitBuildSessions.Unlock()
	return session, nil
}

// installBuildSubprojects installs dependencies for package-manager commands
// that build a nested project, such as `yarn --cwd frontend build`. A root
// install does not install this directory when the repository is not a Yarn
// workspace, which otherwise produces misleading "Cannot find module react"
// TypeScript errors during the isolated release build.
func resolveBuildScript(root string) (kind, script string) {
	pkg, err := readPackage(root)
	if err != nil {
		return "lvy", ""
	}
	scripts, _ := pkg["scripts"].(map[string]any)
	if declaration, ok := pkg["alemonjs"].(map[string]any); ok {
		if value, ok := declaration["build"].(string); ok && strings.TrimSpace(value) != "" {
			name := strings.TrimSpace(value)
			if _, exists := scripts[name]; exists {
				return "script", name
			}
		}
	}
	if _, exists := scripts["bundle"]; exists {
		return "script", "bundle"
	}
	if _, exists := scripts["build"]; exists {
		return "script", "build"
	}
	return "lvy", ""
}

func runResolvedBuild(root, kind, script string) (string, error) {
	if kind == "script" {
		return runPackageManager(root, "run", script)
	}
	// The fallback must go through npx: a Git release is built in an isolated
	// worktree, and a bare `lvy` invocation cannot see node_modules/.bin. npx
	// resolves the project's installed lvy package (and can fetch it when the
	// project does not declare it directly).
	return run(root, "npx", "lvy", "build")
}

func installBuildSubprojects(root, scriptName string) (string, error) {
	pkg, err := readPackage(root)
	if err != nil {
		return "", err
	}
	scripts, _ := pkg["scripts"].(map[string]any)
	buildScript, _ := scripts[scriptName].(string)
	if strings.TrimSpace(buildScript) == "" {
		return "", nil
	}
	fields := strings.Fields(buildScript)
	logs := []string{}
	seen := map[string]bool{}
	for index, field := range fields {
		if field != "--cwd" && field != "--prefix" || index+1 >= len(fields) {
			continue
		}
		relative := strings.Trim(fields[index+1], "'\"")
		if relative == "" || filepath.IsAbs(relative) || relative == "." {
			continue
		}
		path := filepath.Clean(filepath.Join(root, relative))
		within, relErr := filepath.Rel(root, path)
		if relErr != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) || seen[path] {
			continue
		}
		if _, statErr := os.Stat(filepath.Join(path, "package.json")); statErr != nil {
			continue
		}
		seen[path] = true
		manager := projectPackageManager(path)
		logs = append(logs, "安装 "+manager+" 子项目依赖："+within)
		install, installErr := runPackageManager(path, "install")
		if install != "" {
			logs = append(logs, install)
		}
		if installErr != nil {
			return strings.Join(logs, "\n"), installErr
		}
	}
	return strings.Join(logs, "\n"), nil
}

func buildDependencyError(action, output string, cause error) error {
	lower := strings.ToLower(output)
	if strings.Contains(lower, `the engine "node" is incompatible`) || strings.Contains(lower, "engine \"node\" is incompatible") {
		got := "当前 Node.js 版本"
		if marker := strings.Index(output, "Got \""); marker >= 0 {
			if rest := output[marker+len("Got \""):]; strings.Index(rest, "\"") >= 0 {
				got = "当前 Node.js " + rest[:strings.Index(rest, "\"")]
			}
		}
		return errors.New(action + "：" + got + " 不被项目依赖支持。请安装并切换到 Node.js 24 LTS（推荐），或切换到错误信息中要求的版本后重新构建；Node.js 25 等非 LTS 版本常会被依赖明确拒绝。")
	}
	detail := strings.TrimSpace(output)
	if cause != nil && (detail == "" || !strings.Contains(detail, cause.Error())) {
		if detail != "" {
			detail += "\n"
		}
		detail += cause.Error()
	}
	if detail == "" {
		detail = "未返回错误详情"
	}
	// A build can print tens of thousands of bytes (for example webpack or
	// TypeScript diagnostics). Returning all of it as the HTTP error makes the
	// 400 response oversized and causes the desktop client to truncate it. The
	// tail contains the actionable command failure in Yarn/npm output.
	const maxErrorDetail = 12000
	if len(detail) > maxErrorDetail {
		detail = "（前面的构建日志已省略）\n" + detail[len(detail)-maxErrorDetail:]
	}
	return errors.New(action + "：" + detail)
}
func scanPublishFiles(root, prefix string) []string {
	entries, _ := os.ReadDir(root)
	items := []string{}
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, ".") || n == "node_modules" || (prefix == "" && n == "package.json") {
			continue
		}
		rel := filepath.Join(prefix, n)
		if e.IsDir() {
			if children := scanPublishFiles(filepath.Join(root, n), rel); len(children) > 0 {
				items = append(items, rel)
				items = append(items, children...)
			}
		} else {
			items = append(items, rel)
		}
	}
	return items
}
func cleanupGitBuildSessions() {
	now := time.Now()
	for id, s := range gitBuildSessions.items {
		if now.Sub(s.CreatedAt) > 30*time.Minute {
			_, _ = gitRun(s.root, "worktree", "remove", "--force", s.worktree)
			_ = os.RemoveAll(s.worktree)
			delete(gitBuildSessions.items, id)
		}
	}
}

// CleanupGitBuildSessions releases detached worktrees when the setup process
// exits. Expired sessions are also cleaned before every session operation.
func CleanupGitBuildSessions() {
	gitBuildSessions.Lock()
	defer gitBuildSessions.Unlock()
	for id, state := range gitBuildSessions.items {
		_, _ = gitRun(state.root, "worktree", "remove", "--force", state.worktree)
		_ = os.RemoveAll(state.worktree)
		delete(gitBuildSessions.items, id)
	}
}

func PublishPreparedGitBuild(id, version string, artifacts []string, confirmed bool) (Result, error) {
	gitBuildSessions.Lock()
	cleanupGitBuildSessions()
	state, ok := gitBuildSessions.items[id]
	if ok && state.publishing {
		gitBuildSessions.Unlock()
		return Result{}, errors.New("该构建会话正在发布，请等待当前操作完成")
	}
	if ok {
		state.publishing = true
		gitBuildSessions.items[id] = state
	}
	gitBuildSessions.Unlock()
	if !ok {
		return Result{}, errors.New("构建会话已过期，请重新构建")
	}
	if len(artifacts) == 0 {
		return Result{}, errors.New("请至少选择一个最终产物")
	}
	allowed, seen := map[string]bool{}, map[string]bool{}
	for _, item := range state.Files {
		allowed[item] = true
	}
	selected := []string{}
	for _, item := range artifacts {
		item = filepath.Clean(item)
		if !validReleaseArtifact(item) || !allowed[item] || seen[item] {
			return Result{}, errors.New("所选产物无效或不属于本次构建：" + item)
		}
		seen[item] = true
		selected = append(selected, item)
	}
	result, err := publishPreparedWorktree(&state, version, selected, confirmed)
	gitBuildSessions.Lock()
	if err == nil {
		delete(gitBuildSessions.items, id)
	} else if _, exists := gitBuildSessions.items[id]; exists {
		state.publishing = false
		gitBuildSessions.items[id] = state
	}
	gitBuildSessions.Unlock()
	if err == nil {
		_, _ = gitRun(state.root, "worktree", "remove", "--force", state.worktree)
		_ = os.RemoveAll(state.worktree)
	}
	return result, err
}

// RetryPreparedGitTag is available only after the release branch commit was
// accepted but the tag push failed. It never rebuilds or pushes the branch.
func RetryPreparedGitTag(id string) (Result, error) {
	gitBuildSessions.Lock()
	cleanupGitBuildSessions()
	state, ok := gitBuildSessions.items[id]
	if !ok {
		gitBuildSessions.Unlock()
		return Result{}, errors.New("构建会话已过期，请重新构建")
	}
	if state.publishing {
		gitBuildSessions.Unlock()
		return Result{}, errors.New("该构建会话正在发布，请等待当前操作完成")
	}
	if !state.branchPushed || state.releaseCommit == "" || state.releaseVersion == "" {
		gitBuildSessions.Unlock()
		return Result{}, errors.New("当前会话没有可重试的标签推送")
	}
	state.publishing = true
	gitBuildSessions.items[id] = state
	gitBuildSessions.Unlock()

	result, err := retryPreparedGitTag(state)
	gitBuildSessions.Lock()
	if err == nil {
		delete(gitBuildSessions.items, id)
	} else if _, exists := gitBuildSessions.items[id]; exists {
		state.publishing = false
		gitBuildSessions.items[id] = state
	}
	gitBuildSessions.Unlock()
	if err == nil {
		_, _ = gitRun(state.root, "worktree", "remove", "--force", state.worktree)
		_ = os.RemoveAll(state.worktree)
	}
	return result, err
}

// publishRelease is the single implementation of the Git release step: it
// normalizes the version, verifies the tag is unused, and commits the already
// built files from sourceWorktree to the target release branch, then pushes
// the branch and the version tag. It never touches the caller's working tree.
// releaseCommit is non-empty once the branch push succeeded, so a failed tag
// push can be retried without rebuilding.
func publishRelease(root, sourceWorktree, sourceBranch, sourceCommit, version, suggestedVersion string, artifacts []string, targetBranch string, confirmed bool) (releaseCommit string, result Result, err error) {
	if version == "" {
		version = suggestedVersion
	} else {
		version = "v" + strings.TrimPrefix(version, "v")
	}
	if !gitVersionPattern.MatchString(version) {
		return "", Result{}, errors.New("版本号应为 v1.2.3 或 1.2.3")
	}
	if _, err := gitRun(root, "rev-parse", "-q", "--verify", "refs/tags/"+version); err == nil {
		return "", Result{}, errors.New("版本标签 " + version + " 已存在，已发布版本不可覆盖")
	}
	if !confirmed {
		return "", Result{Path: root, Output: "检查通过：将发布 " + version + " 到 " + targetBranch + " 并创建标签"}, errors.New("请确认后再开始 GIT 发布")
	}
	logs := []string{"使用已完成的构建 " + shortGitSHA(sourceCommit)}
	worktree, err := os.MkdirTemp("", "alx-release-")
	if err != nil {
		return "", Result{}, err
	}
	defer os.RemoveAll(worktree)
	start := "HEAD"
	if _, err := gitRun(root, "ls-remote", "--exit-code", "--heads", "origin", "refs/heads/"+targetBranch); err == nil {
		output, err := gitRun(root, "fetch", "origin", targetBranch)
		if err != nil {
			return "", Result{Path: root, Output: strings.Join(append(logs, output), "\n")}, errors.New("无法同步远程 " + targetBranch + " 分支：" + output)
		}
		start = "origin/" + targetBranch
	}
	output, err := gitRun(root, "worktree", "add", "--detach", worktree, start)
	if err != nil {
		return "", Result{Path: root, Output: strings.Join(append(logs, output), "\n")}, errors.New("无法创建安全的临时发布目录：" + output)
	}
	defer gitRun(root, "worktree", "remove", "--force", worktree)
	if output, err = gitRun(worktree, "rm", "-rf", "."); err != nil {
		return "", Result{}, errors.New("无法准备发布目录：" + output)
	}
	if output, err = gitRun(worktree, "clean", "-fdx"); err != nil {
		return "", Result{}, errors.New("无法清理发布目录：" + output)
	}
	if err := copyReleaseFiles(sourceWorktree, worktree, strings.TrimPrefix(version, "v"), artifacts); err != nil {
		return "", Result{}, err
	}
	mappingData, err := json.MarshalIndent(ReleaseMapping{Version: version, SourceBranch: sourceBranch, SourceCommit: sourceCommit}, "", "  ")
	if err != nil {
		return "", Result{}, err
	}
	if err := os.WriteFile(filepath.Join(worktree, ".alx-release.json"), append(mappingData, '\n'), 0644); err != nil {
		return "", Result{}, err
	}
	if _, err := gitRun(worktree, "add", "-A"); err != nil {
		return "", Result{}, err
	}
	commitMessage := "release: " + version + " (" + sourceBranch + "@" + shortGitSHA(sourceCommit) + ")"
	if output, err = gitRun(worktree, "commit", "-m", commitMessage); err != nil {
		return "", Result{}, errors.New("无法创建 release 提交（请先配置 Git 用户名和邮箱）：" + output)
	}
	if output, err = gitRun(worktree, "push", "origin", "HEAD:refs/heads/"+targetBranch); err != nil {
		return "", Result{Path: root, Output: strings.Join(append(logs, output), "\n")}, errors.New(targetBranch + " 分支推送失败：" + output)
	}
	releaseCommit, _ = gitRun(worktree, "rev-parse", "HEAD")
	if output, err = gitRun(worktree, "tag", "-a", version, "-m", "Release "+version+"\n\nSource: "+sourceBranch+"@"+sourceCommit); err != nil {
		return releaseCommit, Result{}, errors.New("release 分支已推送，但无法创建标签：" + output)
	}
	if output, err = gitRun(worktree, "push", "origin", version); err != nil {
		return releaseCommit, Result{Path: root, Output: strings.Join(append(logs, output), "\n")}, errors.New("release 分支已推送，但标签未推送；请修复权限或网络后重试标签推送：" + output)
	}
	logs = append(logs, "已发布 "+version+"："+targetBranch+" 分支、标签及源码映射已推送（"+sourceBranch+"@"+shortGitSHA(sourceCommit)+"）。")
	return releaseCommit, Result{Path: root, Output: strings.Join(logs, "\n")}, nil
}

// publishPreparedWorktree deliberately consumes the exact worktree inspected by
// the user. Rebuilding here would make the selected artifact list misleading.
func publishPreparedWorktree(state *gitBuildState, version string, artifacts []string, confirmed bool) (Result, error) {
	if _, err := syncPublishRemote(state.root); err != nil {
		return Result{}, err
	}
	status, err := GitReleaseStatus(state.root)
	if err != nil {
		return Result{}, err
	}
	if len(status.Issues) > 0 {
		return Result{}, errors.New("发布前检查未通过：" + strings.Join(status.Issues, "；"))
	}
	releaseCommit, result, err := publishRelease(state.root, state.worktree, state.Branch, state.Commit, version, status.SuggestedVersion, artifacts, state.Target, confirmed)
	// A failed tag push still leaves the branch pushed, so record the commit and
	// version for RetryPreparedGitTag whenever publishRelease got that far.
	state.releaseCommit = releaseCommit
	if releaseCommit != "" {
		state.releaseVersion = "v" + strings.TrimPrefix(version, "v")
		if version == "" {
			state.releaseVersion = status.SuggestedVersion
		}
		state.branchPushed = true
	}
	return result, err
}

func retryPreparedGitTag(state gitBuildState) (Result, error) {
	worktree, err := os.MkdirTemp("", "alx-tag-retry-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(worktree)
	if output, err := gitRun(state.root, "worktree", "add", "--detach", worktree, state.releaseCommit); err != nil {
		return Result{}, errors.New("无法准备标签重试目录：" + output)
	}
	defer gitRun(state.root, "worktree", "remove", "--force", worktree)
	if _, err := gitRun(worktree, "tag", "-a", state.releaseVersion, "-m", "Release "+state.releaseVersion+"\n\nSource: "+state.Branch+"@"+state.Commit); err != nil {
		return Result{}, errors.New("无法创建重试标签：" + err.Error())
	}
	if output, err := gitRun(worktree, "push", "origin", state.releaseVersion); err != nil {
		_, _ = gitRun(worktree, "tag", "-d", state.releaseVersion)
		return Result{Path: state.root, Output: output}, errors.New("标签仍未推送；请检查网络或仓库写入权限后重试：" + output)
	}
	return Result{Path: state.root, Output: "已重试并推送标签 " + state.releaseVersion + "。release 分支没有重复提交。"}, nil
}
