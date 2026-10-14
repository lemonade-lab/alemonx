package robot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func dailyGit(t *testing.T, path string, args ...string) string {
	t.Helper()
	output, err := gitRun(path, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return output
}

func dailyWrite(t *testing.T, path, name, content string) {
	t.Helper()
	full := filepath.Join(path, name)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func dailyRepository(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git is unavailable")
	}
	root := t.TempDir()
	dailyGit(t, root, "init", "-b", "main")
	dailyGit(t, root, "config", "user.name", "Git Audit")
	dailyGit(t, root, "config", "user.email", "audit@example.invalid")
	dailyGit(t, root, "config", "core.quotepath", "true")
	dailyWrite(t, root, "alpha.txt", "old\n")
	dailyWrite(t, root, "beta.txt", "old\n")
	dailyGit(t, root, "add", ".")
	dailyGit(t, root, "commit", "-m", "initial")
	return root
}

func dailyAction(t *testing.T, root, action string, paths ...string) {
	t.Helper()
	if _, err := GitWorkspaceActionWithOptions(root, action, "", "", GitWorkspaceActionOptions{Paths: paths}); err != nil {
		t.Fatal(err)
	}
}

func TestGitDailyStatusPreservesPathsAndExpandsUntrackedFiles(t *testing.T) {
	root := dailyRepository(t)
	names := []string{"alpha.txt", "中文.txt", "space name.txt", " leading.txt", "trailing .txt", "literal[1].txt"}
	if os.PathSeparator != '\\' {
		names = append(names, "line\nbreak.txt", "back\\slash.txt")
	}
	for _, name := range names {
		dailyWrite(t, root, name, "old\n")
	}
	dailyGit(t, root, "add", ".")
	dailyGit(t, root, "commit", "-m", "paths")
	for _, name := range names {
		dailyWrite(t, root, name, "new\n")
	}
	dailyWrite(t, root, "newdir/nested/中文.txt", "added\n")
	status, err := GitWorkspaceView(root, "commit")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]GitChange{}
	for _, change := range status.Changes {
		found[change.Path] = change
	}
	for _, name := range names {
		change, exists := found[name]
		if !exists || change.Staged || !change.Unstaged {
			t.Fatalf("missing/corrupt change %q: %#v", name, status.Changes)
		}
		diff, err := GitDiffView(root, name, "unstaged")
		if err != nil || !strings.Contains(diff.Diff, "+new") {
			t.Fatalf("diff %q: %#v %v", name, diff, err)
		}
	}
	if _, exists := found["newdir/nested/中文.txt"]; !exists {
		t.Fatalf("untracked directory was collapsed: %#v", status.Changes)
	}
	diff, err := GitDiffView(root, "newdir/nested/中文.txt", "unstaged")
	if err != nil || !strings.Contains(diff.Diff, "+added") {
		t.Fatalf("untracked diff: %#v %v", diff, err)
	}
}

func TestGitDailyCommitPreservesIndexAndUnstagedWork(t *testing.T) {
	root := dailyRepository(t)
	dailyWrite(t, root, "alpha.txt", "staged\n")
	dailyAction(t, root, "stage", "alpha.txt")
	dailyWrite(t, root, "alpha.txt", "later\n")
	dailyWrite(t, root, "beta.txt", "unrelated\n")
	dailyWrite(t, root, "new.txt", "new\n")
	status, err := GitWorkspaceView(root, "commit")
	if err != nil {
		t.Fatal(err)
	}
	staged, err := GitDiffView(root, "alpha.txt", "staged")
	if err != nil || !strings.Contains(staged.Diff, "+staged") || strings.Contains(staged.Diff, "+later") {
		t.Fatalf("staged diff: %#v %v", staged, err)
	}
	unstaged, err := GitDiffView(root, "alpha.txt", "unstaged")
	if err != nil || !strings.Contains(unstaged.Diff, "+later") {
		t.Fatalf("unstaged diff: %#v %v", unstaged, err)
	}
	_, err = GitWorkspaceActionWithOptions(root, "commit", "", "subject\n\nbody", GitWorkspaceActionOptions{IndexRevision: status.IndexRevision})
	if err != nil {
		t.Fatal(err)
	}
	if got := dailyGit(t, root, "show", "HEAD:alpha.txt"); got != "staged" {
		t.Fatalf("committed working tree instead of index: %q", got)
	}
	if got := dailyGit(t, root, "show", "HEAD:beta.txt"); got != "old" {
		t.Fatalf("included unrelated work: %q", got)
	}
	if got := dailyGit(t, root, "diff", "--", "alpha.txt"); !strings.Contains(got, "+later") {
		t.Fatalf("lost unstaged work: %s", got)
	}
	if _, err := GitWorkspaceAction(root, "commit", "", "no staged files"); err == nil {
		t.Fatal("commit should require staged files")
	}
}

func TestGitDailyIndexChangeRequiresNewConfirmation(t *testing.T) {
	root := dailyRepository(t)
	dailyWrite(t, root, "alpha.txt", "first\n")
	dailyAction(t, root, "stage", "alpha.txt")
	status, err := GitWorkspaceView(root, "commit")
	if err != nil {
		t.Fatal(err)
	}
	dailyWrite(t, root, "alpha.txt", "second\n")
	dailyAction(t, root, "stage", "alpha.txt")
	if _, err := GitWorkspaceActionWithOptions(root, "commit", "", "stale", GitWorkspaceActionOptions{IndexRevision: status.IndexRevision}); err == nil {
		t.Fatal("stale confirmation committed new content")
	}
	if dailyGit(t, root, "show", "HEAD:alpha.txt") != "old" {
		t.Fatal("HEAD changed")
	}
}

func TestGitDailyBranchChangeRequiresNewConfirmation(t *testing.T) {
	root := dailyRepository(t)
	dailyGit(t, root, "switch", "-c", "other")
	dailyWrite(t, root, "alpha.txt", "pending\n")
	_, err := GitWorkspaceActionWithOptions(root, "stage", "", "", GitWorkspaceActionOptions{ExpectedBranch: "main", Paths: []string{"alpha.txt"}})
	if err == nil || !strings.Contains(err.Error(), "分支") {
		t.Fatalf("stale branch was accepted: %v", err)
	}
	if got := dailyGit(t, root, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("changed index on unexpected branch: %q", got)
	}
}

func TestGitDailyStageUnstageRestoreAndLiteralPaths(t *testing.T) {
	root := dailyRepository(t)
	dailyWrite(t, root, "a[1].txt", "literal\n")
	dailyWrite(t, root, "a1.txt", "other\n")
	dailyAction(t, root, "stage", "a[1].txt")
	if got := dailyGit(t, root, "diff", "--cached", "--name-only"); got != "a[1].txt" {
		t.Fatalf("pathspec matched other files: %q", got)
	}
	dailyAction(t, root, "unstage", "a[1].txt")
	if got := dailyGit(t, root, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("unstage failed: %q", got)
	}
	dailyWrite(t, root, "alpha.txt", "staged\n")
	dailyAction(t, root, "stage", "alpha.txt")
	dailyWrite(t, root, "alpha.txt", "discard\n")
	dailyAction(t, root, "restore", "alpha.txt")
	content, _ := os.ReadFile(filepath.Join(root, "alpha.txt"))
	if string(content) != "staged\n" {
		t.Fatalf("restore overwrote index: %q", content)
	}
	for _, paths := range [][]string{{"../outside"}, {"a[1].txt", "missing"}, {":(glob)*"}} {
		if _, err := GitWorkspaceActionWithOptions(root, "stage", "", "", GitWorkspaceActionOptions{Paths: paths}); err == nil {
			t.Fatalf("invalid selection accepted: %v", paths)
		}
	}
	if _, err := GitWorkspaceActionWithOptions(root, "restore", "", "", GitWorkspaceActionOptions{Paths: []string{"a1.txt"}}); err == nil {
		t.Fatal("untracked deletion should not be exposed as restore")
	}
}

func TestGitDailyUnbornRepositoryCanUnstage(t *testing.T) {
	root := t.TempDir()
	dailyGit(t, root, "init", "-b", "main")
	dailyWrite(t, root, "new.txt", "new\n")
	dailyAction(t, root, "stage", "new.txt")
	dailyAction(t, root, "unstage", "new.txt")
	if _, err := os.Stat(filepath.Join(root, "new.txt")); err != nil {
		t.Fatal("unstage deleted working file")
	}
}

func TestGitDailySubdirectoryCannotCommitOutsideIndex(t *testing.T) {
	root := dailyRepository(t)
	dailyWrite(t, root, "sub/inside.txt", "old\n")
	dailyGit(t, root, "add", ".")
	dailyGit(t, root, "commit", "-m", "subproject")
	dailyWrite(t, root, "sub/inside.txt", "inside\n")
	dailyWrite(t, root, "alpha.txt", "outside\n")
	sub := filepath.Join(root, "sub")
	status, err := GitWorkspaceView(sub, "commit")
	if err != nil || len(status.Changes) != 1 || status.Changes[0].Path != "inside.txt" {
		t.Fatalf("scope: %#v %v", status.Changes, err)
	}
	dailyAction(t, sub, "stage", "inside.txt")
	dailyAction(t, root, "stage", "alpha.txt")
	if _, err := GitWorkspaceAction(sub, "commit", "", "sub only"); err == nil {
		t.Fatal("outside staged content was committed")
	}
	dailyAction(t, root, "unstage", "alpha.txt")
	if _, err := GitWorkspaceAction(sub, "commit", "", "sub only"); err != nil {
		t.Fatal(err)
	}
	if got := dailyGit(t, root, "show", "--format=", "--name-only", "HEAD"); got != "sub/inside.txt" {
		t.Fatalf("commit leaked outside scope: %q", got)
	}
}

func TestGitDailyRenameCanBeUnstagedAndReviewed(t *testing.T) {
	root := dailyRepository(t)
	dailyGit(t, root, "mv", "alpha.txt", "renamed 中文.txt")
	status, err := GitWorkspaceView(root, "commit")
	if err != nil || len(status.Changes) != 1 || status.Changes[0].OriginalPath != "alpha.txt" {
		t.Fatalf("rename: %#v %v", status.Changes, err)
	}
	diff, err := GitDiffView(root, "renamed 中文.txt", "staged")
	if err != nil || !strings.Contains(diff.Diff, "rename from alpha.txt") {
		t.Fatalf("rename diff: %#v %v", diff, err)
	}
	dailyAction(t, root, "unstage", "renamed 中文.txt")
	if got := dailyGit(t, root, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("rename source still staged: %q", got)
	}
}

func TestGitDailyAlternateRemoteFetchPushPullAndTag(t *testing.T) {
	root := dailyRepository(t)
	remote := t.TempDir()
	dailyGit(t, remote, "init", "--bare")
	dailyGit(t, root, "remote", "add", "backup", remote)
	options := GitWorkspaceActionOptions{Remote: "backup"}
	if _, err := GitWorkspaceActionWithOptions(root, "push", "", "", options); err != nil {
		t.Fatal(err)
	}
	if _, err := GitWorkspaceActionWithOptions(root, "fetch", "", "", options); err != nil {
		t.Fatal(err)
	}
	status, err := GitWorkspaceViewForRemote(root, "remote", "backup")
	if err != nil || status.RemoteName != "backup" || !status.RemoteSynced || !status.RemoteReachable {
		t.Fatalf("remote: %#v %v", status, err)
	}
	other := t.TempDir()
	dailyGit(t, other, "clone", "--branch", "main", remote, ".")
	dailyGit(t, other, "config", "user.name", "Other")
	dailyGit(t, other, "config", "user.email", "other@example.invalid")
	dailyWrite(t, other, "alpha.txt", "remote update\n")
	dailyGit(t, other, "add", ".")
	dailyGit(t, other, "commit", "-m", "remote update")
	dailyGit(t, other, "push")
	if _, err := GitWorkspaceActionWithOptions(root, "pull", "", "", options); err != nil {
		t.Fatal(err)
	}
	if got := dailyGit(t, root, "show", "HEAD:alpha.txt"); got != "remote update" {
		t.Fatalf("pull did not update branch: %q", got)
	}
	dailyGit(t, root, "tag", "main")
	if _, err := GitWorkspaceActionWithOptions(root, "tag-push", "main", "", options); err != nil {
		t.Fatal(err)
	}
	if got := dailyGit(t, remote, "show-ref", "--verify", "refs/tags/main"); got == "" {
		t.Fatal("ambiguous tag was not pushed")
	}
}

func TestGitDailyPullTargetsCurrentBranchAndItsNamedUpstream(t *testing.T) {
	root := dailyRepository(t)
	remote := t.TempDir()
	dailyGit(t, remote, "init", "--bare")
	dailyGit(t, root, "remote", "add", "backup", remote)
	dailyGit(t, root, "push", "-u", "backup", "HEAD:refs/heads/trunk")
	before := dailyGit(t, root, "rev-parse", "HEAD")
	options := GitWorkspaceActionOptions{Remote: "backup", ExpectedBranch: "main"}
	if _, err := GitWorkspaceActionWithOptions(root, "pull", "other", "", options); err == nil || !strings.Contains(err.Error(), "当前分支") {
		t.Fatalf("incorrect branch target must be rejected: %v", err)
	}
	if dailyGit(t, root, "rev-parse", "HEAD") != before {
		t.Fatal("wrong target changed HEAD")
	}
	other := t.TempDir()
	dailyGit(t, other, "clone", "--branch", "trunk", remote, ".")
	dailyGit(t, other, "config", "user.name", "Other")
	dailyGit(t, other, "config", "user.email", "other@example.invalid")
	dailyWrite(t, other, "alpha.txt", "upstream trunk update\n")
	dailyGit(t, other, "commit", "-am", "update trunk")
	dailyGit(t, other, "push")
	if _, err := GitWorkspaceActionWithOptions(root, "pull", "main", "", options); err != nil {
		t.Fatal(err)
	}
	if currentBranch(root) != "main" || dailyGit(t, root, "show", "HEAD:alpha.txt") != "upstream trunk update" {
		t.Fatal("update must use main's configured upstream trunk")
	}
}

func TestGitDailyStashPreservesStagedAndUntrackedWork(t *testing.T) {
	root := dailyRepository(t)
	dailyWrite(t, root, "alpha.txt", "staged\n")
	dailyAction(t, root, "stage", "alpha.txt")
	dailyWrite(t, root, "notes.txt", "untracked\n")
	if _, err := GitWorkspaceAction(root, "stash-save", "", "pending work"); err != nil {
		t.Fatal(err)
	}
	status, err := GitWorkspaceView(root, "commit")
	if err != nil || len(status.Changes) != 0 || len(status.Stashes) != 1 {
		t.Fatalf("saved: %#v %v", status, err)
	}
	sha := status.Stashes[0].SHA
	if _, err := GitWorkspaceAction(root, "stash-apply", sha, ""); err != nil {
		t.Fatal(err)
	}
	if got := dailyGit(t, root, "diff", "--cached", "--name-only"); got != "alpha.txt" {
		t.Fatalf("index was not restored: %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "notes.txt")); err != nil {
		t.Fatal("untracked file lost")
	}
	if _, err := GitWorkspaceAction(root, "stash-apply", sha, ""); err == nil {
		t.Fatal("restore into dirty worktree accepted")
	}
	if _, err := GitWorkspaceAction(root, "stash-save", "", "second"); err != nil {
		t.Fatal(err)
	}
	if _, err := GitWorkspaceAction(root, "stash-drop", sha, ""); err != nil {
		t.Fatal(err)
	}
	if got := dailyGit(t, root, "stash", "list", "--format=%H"); strings.Contains(got, sha) {
		t.Fatal("wrong stash removed")
	}
}

func TestGitDailyMergeConflictCanBeAborted(t *testing.T) {
	root := dailyRepository(t)
	dailyGit(t, root, "switch", "-c", "feature")
	dailyWrite(t, root, "alpha.txt", "feature\n")
	dailyGit(t, root, "commit", "-am", "feature")
	dailyGit(t, root, "switch", "main")
	dailyWrite(t, root, "alpha.txt", "main\n")
	dailyGit(t, root, "commit", "-am", "main")
	before := dailyGit(t, root, "rev-parse", "HEAD")
	if _, err := GitWorkspaceAction(root, "merge", "feature", ""); err == nil {
		t.Fatal("expected conflict")
	}
	status, err := GitWorkspaceView(root, "commit")
	if err != nil || !status.MergeInProgress || len(status.Changes) != 1 || !status.Changes[0].Conflict {
		t.Fatalf("conflict state: %#v %v", status, err)
	}
	if _, err := GitWorkspaceAction(root, "merge-abort", "", ""); err != nil {
		t.Fatal(err)
	}
	if dailyGit(t, root, "rev-parse", "HEAD") != before {
		t.Fatal("abort changed local history")
	}
}

func TestGitDailyHistoryPaginationDetailsAndIdentity(t *testing.T) {
	root := dailyRepository(t)
	for i := 0; i < 24; i++ {
		dailyGit(t, root, "commit", "--allow-empty", "-m", fmt.Sprintf("entry %d", i))
	}
	first, err := GitWorkspaceViewWithOptions(root, "history", "", 0)
	if err != nil || len(first.Commits) != 20 || !first.HistoryHasMore {
		t.Fatalf("first history page: %#v %v", first, err)
	}
	second, err := GitWorkspaceViewWithOptions(root, "history", "", 20)
	if err != nil || len(second.Commits) != 5 || second.HistoryHasMore {
		t.Fatalf("second history page: %#v %v", second, err)
	}
	detail, err := GitWorkspaceCommit(root, second.Commits[4].SHA)
	if err != nil || !strings.Contains(detail.Output, "+old") {
		t.Fatalf("commit detail: %#v %v", detail, err)
	}
	if _, err := GitWorkspaceCommit(root, "--all"); err == nil {
		t.Fatal("invalid commit accepted")
	}
	if _, err := GitWorkspaceAction(root, "identity-set", "New Author", "new@example.invalid"); err != nil {
		t.Fatal(err)
	}
	if got := dailyGit(t, root, "config", "--local", "user.name"); got != "New Author" {
		t.Fatalf("identity not saved locally: %q", got)
	}
}
