package robot

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildDependencyErrorKeepsProcessFailureDetails(t *testing.T) {
	err := buildDependencyError("安装构建依赖失败", "Preparing worktree (detached HEAD)\nHEAD is now at abc", errors.New("yarn: executable file not found in $PATH"))
	if !strings.Contains(err.Error(), "yarn: executable file not found") {
		t.Fatalf("error lost process failure detail: %v", err)
	}
}

func TestBuildDependencyErrorBoundsLargeBuildOutput(t *testing.T) {
	output := strings.Repeat("verbose build output\n", 2000) + "ERROR: build failed"
	err := buildDependencyError("构建失败", output, errors.New("exit status 1"))
	if len(err.Error()) > 12500 {
		t.Fatalf("build error was not bounded: %d bytes", len(err.Error()))
	}
	if !strings.Contains(err.Error(), "ERROR: build failed") {
		t.Fatalf("build error lost the actionable tail: %v", err)
	}
}

func TestResolveBuildScriptUsesDeclaredPrecedence(t *testing.T) {
	root := t.TempDir()
	writePackage := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writePackage(`{"scripts":{"bundle":"echo bundle","custom":"echo custom"},"alemonjs":{"build":"custom"}}`)
	if kind, script := resolveBuildScript(root); kind != "script" || script != "custom" {
		t.Fatalf("alemonjs.build resolution = %q, %q", kind, script)
	}
	writePackage(`{"scripts":{"bundle":"echo bundle"}}`)
	if kind, script := resolveBuildScript(root); kind != "script" || script != "bundle" {
		t.Fatalf("bundle resolution = %q, %q", kind, script)
	}
	writePackage(`{"scripts":{"build":"echo build"}}`)
	if kind, script := resolveBuildScript(root); kind != "script" || script != "build" {
		t.Fatalf("build resolution = %q, %q", kind, script)
	}
	writePackage(`{"scripts":{}}`)
	if kind, script := resolveBuildScript(root); kind != "lvy" || script != "" {
		t.Fatalf("lvy resolution = %q, %q", kind, script)
	}
}

func TestCloneProgressFromGitOutput(t *testing.T) {
	tests := []struct {
		output  string
		percent int
		detail  string
	}{
		{"Receiving objects:  50% (20/40), 1.20 MiB | 1.20 MiB/s", 47, "正在接收对象（50%）…"},
		{"Resolving deltas: 100% (12/12), done.", 99, "正在解析增量（100%）…"},
	}
	for _, test := range tests {
		progress, ok := cloneProgressFromGitOutput(test.output)
		if !ok || progress.Percent != test.percent || progress.Detail != test.detail {
			t.Fatalf("cloneProgressFromGitOutput(%q) = %#v, %v", test.output, progress, ok)
		}
	}
	if _, ok := cloneProgressFromGitOutput("remote: Enumerating objects: 12, done."); ok {
		t.Fatal("non-percentage Git output should not create a progress update")
	}
}

func TestLocalPackageCloneDestinationStaysInBackpack(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"robot"}`), 0644); err != nil {
		t.Fatal(err)
	}
	target, err := LocalPackageCloneDestination(root, "https://github.com/example/plugin.git", "plugin")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "packages", "plugin")
	if target.Path != want || target.Exists {
		t.Fatalf("target = %#v, want path %q and no existing directory", target, want)
	}
}

func TestGitReleaseStatusDoesNotBlockLocalChangesOrMissingBuildOutput(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"example","version":"1.0.0","scripts":{"build":"echo build"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	for _, command := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
		{"add", "package.json"},
		{"commit", "-m", "initial"},
	} {
		if _, err := gitRun(root, command...); err != nil {
			t.Skipf("git is unavailable for release-status test: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "local-note.txt"), []byte("not committed\n"), 0644); err != nil {
		t.Fatal(err)
	}

	status, err := GitReleaseStatus(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range status.Issues {
		if issue == "工作区有未提交修改；请先提交或暂存后再打包。" || issue == "尚未发现 lib 构建产物；发布时会先执行 build。" {
			t.Fatalf("local working files must not block a selected-commit release: %#v", status.Issues)
		}
	}
}

func TestGitReleaseStatusDoesNotBlockSourceRemoteDivergence(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "remote.git")
	root := t.TempDir()
	if _, err := gitRun(t.TempDir(), "init", "--bare", remote); err != nil {
		t.Skipf("git is unavailable for release-status test: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"example","version":"1.0.0","scripts":{"build":"echo build"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	for _, command := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
		{"add", "package.json"},
		{"commit", "-m", "initial"},
		{"remote", "add", "origin", remote},
		{"push", "-u", "origin", "main"},
	} {
		if _, err := gitRun(root, command...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"example","version":"1.0.1","scripts":{"build":"echo build"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	for _, command := range [][]string{{"add", "package.json"}, {"commit", "-m", "local update"}} {
		if _, err := gitRun(root, command...); err != nil {
			t.Fatal(err)
		}
	}
	status, err := GitReleaseStatus(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range status.Issues {
		if strings.Contains(issue, "与远程不同步") {
			t.Fatalf("source remote divergence must not block release: %#v", status.Issues)
		}
	}
	if !strings.Contains(strings.Join(status.Checks, "\n"), "与远程不同步") {
		t.Fatalf("source remote divergence should be explained: %#v", status.Checks)
	}
}

func TestGitWorkspaceReportsSourceControlWithoutReleaseChecks(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("initial\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, command := range [][]string{{"init", "-b", "main"}, {"config", "user.name", "Test User"}, {"config", "user.email", "test@example.com"}, {"add", "note.txt"}, {"commit", "-m", "initial"}} {
		if _, err := gitRun(root, command...); err != nil {
			t.Skipf("git is unavailable for workspace test: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "changed.txt"), []byte("pending\n"), 0644); err != nil {
		t.Fatal(err)
	}
	status, err := GitWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Repository || status.Branch != "main" || len(status.Commits) != 1 {
		t.Fatalf("workspace = %#v", status)
	}
	if len(status.Changes) != 1 || status.Changes[0].Path != "changed.txt" {
		t.Fatalf("changes = %#v", status.Changes)
	}
	commitView, err := GitWorkspaceView(root, "commit")
	if err != nil || len(commitView.Changes) != 1 || len(commitView.Commits) != 0 || len(commitView.Tags) != 0 || len(commitView.Branches) != 0 {
		t.Fatalf("commit view must only load working changes: %#v, %v", commitView, err)
	}
	historyView, err := GitWorkspaceView(root, "history")
	if err != nil || len(historyView.Commits) != 1 || len(historyView.Changes) != 0 || len(historyView.Tags) != 0 {
		t.Fatalf("history view must only load commits: %#v, %v", historyView, err)
	}
	if _, err := GitWorkspaceAction(root, "tag-create", "v1.0.0", "release: v1.0.0"); err != nil {
		t.Fatal(err)
	}
	status, err = GitWorkspace(root)
	if err != nil || len(status.Tags) != 1 || status.Tags[0].Name != "v1.0.0" || !strings.Contains(status.Tags[0].Subject, "release") {
		t.Fatalf("tags = %#v, %v", status.Tags, err)
	}
	if len(status.Branches) != 1 || !status.Branches[0].Current || status.Branches[0].Name != "main" {
		t.Fatalf("branches = %#v", status.Branches)
	}
	if _, err := GitWorkspaceAction(root, "branch-create", "bad..branch", ""); err == nil {
		t.Fatal("invalid branch name should be rejected")
	}
}

func TestGitWorkspaceDetectsLiveRemoteDivergenceWhenTrackingRefIsStale(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "remote.git")
	source := filepath.Join(t.TempDir(), "source")
	cloneParent := t.TempDir()
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := gitRun(t.TempDir(), "init", "--bare", remote); err != nil {
		t.Skipf("git is unavailable: %v", err)
	}
	for _, command := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
	} {
		if _, err := gitRun(source, command...); err != nil {
			t.Skipf("git is unavailable: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "note.txt"), []byte("one\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, command := range [][]string{
		{"add", "note.txt"}, {"commit", "-m", "initial"},
		{"remote", "add", "origin", remote}, {"push", "-u", "origin", "main"},
	} {
		if _, err := gitRun(source, command...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := gitRun(cloneParent, "clone", remote, "clone"); err != nil {
		t.Fatal(err)
	}
	clone := filepath.Join(cloneParent, "clone")
	if err := os.WriteFile(filepath.Join(source, "note.txt"), []byte("two\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, command := range [][]string{{"add", "note.txt"}, {"commit", "-m", "remote update"}, {"push"}} {
		if _, err := gitRun(source, command...); err != nil {
			t.Fatal(err)
		}
	}
	status, err := GitWorkspaceView(clone, "remote")
	if err != nil {
		t.Fatal(err)
	}
	if !status.RemoteChecked || !status.RemoteReachable || status.RemoteSynced {
		t.Fatalf("live remote status = %#v", status)
	}
	if status.Ahead != 0 || status.Behind != 0 {
		t.Fatalf("cached counts should remain unchanged before fetch: %#v", status)
	}
}

func TestParseRemoteRefsUsesDefaultBranchAndOrdersTags(t *testing.T) {
	heads, tags, branch := parseRemoteRefs("ref: refs/heads/trunk\tHEAD\nabc\tHEAD\nabc\trefs/heads/trunk\ndef\trefs/heads/release\n1\trefs/tags/v0.0.9\n2\trefs/tags/v0.0.10\n")
	if branch != "trunk" || heads["trunk"] != "abc" || heads["release"] != "def" {
		t.Fatalf("remote heads = %#v, branch = %q", heads, branch)
	}
	if strings.Join(tags, ",") != "v0.0.10,v0.0.9" {
		t.Fatalf("tags = %#v", tags)
	}
}

func TestRemoteAdviceExplainsCommonFailures(t *testing.T) {
	if got := remoteAdvice("git@github.com: Permission denied (publickey)."); !strings.Contains(got, "SSH") {
		t.Fatalf("ssh advice = %q", got)
	}
	if got := remoteAdvice("fatal: repository not found"); !strings.Contains(got, "不存在") {
		t.Fatalf("not-found advice = %q", got)
	}
}

func TestGitWorkspaceFetchConvertsShallowCloneToFullHistory(t *testing.T) {
	remote := t.TempDir()
	if _, err := gitRun(remote, "init", "--bare"); err != nil {
		t.Skipf("git is unavailable: %v", err)
	}
	seed := t.TempDir()
	if err := os.WriteFile(filepath.Join(seed, "note.txt"), []byte("initial\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, command := range [][]string{{"init", "-b", "main"}, {"config", "user.name", "Test User"}, {"config", "user.email", "test@example.com"}, {"add", "."}, {"commit", "-m", "initial"}, {"branch", "feature/remote"}, {"remote", "add", "origin", remote}, {"push", "-u", "origin", "main"}, {"push", "-u", "origin", "feature/remote"}} {
		if _, err := gitRun(seed, command...); err != nil {
			t.Fatal(err)
		}
	}
	// Build a shallow, single-branch clone deterministically. Some git builds
	// ignore --depth on clone for local/file transports, so make the repo
	// shallow afterwards with git fetch --depth; --single-branch keeps only the
	// checked-out branch, which is what a shallow clone carries.
	parent := t.TempDir()
	root := filepath.Join(parent, "repo")
	if _, err := gitRun(parent, "clone", "--single-branch", "--branch", "main", "file://"+remote, "repo"); err != nil {
		t.Fatal(err)
	}
	if _, err := gitRun(root, "fetch", "--depth", "1", "origin"); err != nil {
		t.Fatal(err)
	}
	if shallow, err := gitRun(root, "rev-parse", "--is-shallow-repository"); err != nil || strings.TrimSpace(shallow) != "true" {
		t.Fatalf("expected shallow repository at %s, got %q err=%v", root, shallow, err)
	}
	// Before fetch, only the cloned branch is visible; the other remote branch
	// (feature/remote) is absent because a shallow clone does not carry it.
	before, err := GitWorkspaceView(root, "branch")
	if err != nil {
		t.Fatal(err)
	}
	for _, branch := range before.RemoteBranches {
		if branch.Name == "origin/feature/remote" {
			t.Fatalf("shallow clone should not know feature/remote, got %#v", before.RemoteBranches)
		}
	}
	// Fetching must unshallow and surface every remote branch.
	if _, err := GitWorkspaceAction(root, "fetch", "", ""); err != nil {
		t.Fatal(err)
	}
	after, err := GitWorkspaceView(root, "branch")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, branch := range after.RemoteBranches {
		if branch.Name == "origin/feature/remote" {
			found = true
		}
	}
	if !found {
		t.Fatalf("after fetch, remote branches = %#v", after.RemoteBranches)
	}
}

func TestGitWorkspaceListsFetchedRemoteBranches(t *testing.T) {
	root := t.TempDir()
	remote := t.TempDir()
	if _, err := gitRun(remote, "init", "--bare"); err != nil {
		t.Skipf("git is unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("initial\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, command := range [][]string{{"init", "-b", "main"}, {"config", "user.name", "Test User"}, {"config", "user.email", "test@example.com"}, {"add", "."}, {"commit", "-m", "initial"}, {"remote", "add", "origin", remote}, {"push", "-u", "origin", "main"}, {"switch", "-c", "feature/remote"}, {"push", "-u", "origin", "feature/remote"}, {"switch", "main"}} {
		if _, err := gitRun(root, command...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := GitWorkspaceAction(root, "fetch", "", ""); err != nil {
		t.Fatal(err)
	}
	status, err := GitWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, branch := range status.RemoteBranches {
		if branch.Name == "origin/feature/remote" && branch.Remote == "origin" && branch.Branch == "feature/remote" {
			found = true
		}
	}
	if !found {
		t.Fatalf("remote branches = %#v", status.RemoteBranches)
	}
	if _, err := gitRun(root, "branch", "-D", "feature/remote"); err != nil {
		t.Fatal(err)
	}
	// Git publishing uses the same fetched refs, but presents their short
	// branch name in the source selector rather than an origin/ prefix.
	sources := sourceBranches(root)
	found = false
	for _, branch := range sources {
		if branch.Name == "feature/remote" && len(branch.Commits) > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("publish source branches = %#v", sources)
	}
	ref, err := sourceBranchRef(root, "feature/remote")
	if err != nil || ref != "refs/remotes/origin/feature/remote" {
		t.Fatalf("remote source ref = %q, %v", ref, err)
	}
}

func TestCloneBranchRestoresFullFetchRefspec(t *testing.T) {
	remote := t.TempDir()
	if _, err := gitRun(remote, "init", "--bare"); err != nil {
		t.Skipf("git is unavailable: %v", err)
	}
	seed := t.TempDir()
	if err := os.WriteFile(filepath.Join(seed, "note.txt"), []byte("initial\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, command := range [][]string{{"init", "-b", "main"}, {"config", "user.name", "Test User"}, {"config", "user.email", "test@example.com"}, {"add", "."}, {"commit", "-m", "initial"}, {"branch", "feature/remote"}, {"remote", "add", "origin", remote}, {"push", "-u", "origin", "main"}, {"push", "-u", "origin", "feature/remote"}} {
		if _, err := gitRun(seed, command...); err != nil {
			t.Fatal(err)
		}
	}
	// Simulate git clone --branch: it narrows remote.origin.fetch to that
	// branch, which is exactly the config state CloneRepository is meant to fix.
	parent := t.TempDir()
	dest := filepath.Join(parent, "repo")
	if _, err := gitRun(parent, "clone", "--depth", "1", "--branch", "main", "file://"+remote, "repo"); err != nil {
		t.Fatal(err)
	}
	narrowed, err := gitRun(dest, "config", "--get", "remote.origin.fetch")
	if err != nil {
		t.Fatal(err)
	}
	if narrowed != "+refs/heads/main:refs/remotes/origin/main" {
		t.Fatalf("precondition failed: git clone --branch refspec = %q", narrowed)
	}
	// The wrapper restores the full refspec.
	if _, err := gitRun(dest, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
		t.Fatal(err)
	}
	if _, err := GitWorkspaceAction(dest, "fetch", "", ""); err != nil {
		t.Fatal(err)
	}
	status, err := GitWorkspaceView(dest, "branch")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, branch := range status.RemoteBranches {
		if branch.Name == "origin/feature/remote" {
			found = true
		}
	}
	if !found {
		t.Fatalf("remote branches after fetch = %#v", status.RemoteBranches)
	}
}
