package robot

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const maxGitDiffBytes = 512 * 1024

// GitDiffResult is one changed file's working-tree-vs-HEAD comparison.
// Untracked files carry their raw content rendered as additions because git
// itself produces no diff for them.
type GitDiffResult struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Diff      string `json:"diff"`
	Binary    bool   `json:"binary"`
	Untracked bool   `json:"untracked"`
	Missing   bool   `json:"missing"`
	Truncated bool   `json:"truncated"`
}

// GitDiff returns the diff for one changed file inside the robot workspace.
// The path is passed to git as a literal pathspec (after "--") and, for
// untracked files, additionally resolved inside the repository before reading.
func GitDiff(root, changePath string) (GitDiffResult, error) {
	repo, err := workspacePath(root)
	if err != nil {
		return GitDiffResult{}, err
	}
	return gitDiffAtRepository(repo, changePath)
}

// gitDiffAtRepository is shared by the robot workspace and a discovered
// backpack package. Its caller must resolve repo from a trusted root.
func GitDiffView(root, changePath, view string) (GitDiffResult, error) {
	repo, err := workspacePath(root)
	if err != nil {
		return GitDiffResult{}, err
	}
	if view != "all" && view != "staged" && view != "unstaged" {
		return GitDiffResult{}, errors.New("不支持的变更对比视图")
	}
	return gitDiffAtRepositoryView(repo, changePath, view)
}

func gitDiffAtRepository(repo, changePath string) (GitDiffResult, error) {
	return gitDiffAtRepositoryView(repo, changePath, "all")
}

func gitDiffAtRepositoryView(repo, changePath, view string) (GitDiffResult, error) {
	cleanPath, err := cleanGitChangePath(changePath)
	if err != nil {
		return GitDiffResult{}, err
	}
	result := GitDiffResult{Path: cleanPath}
	inside, err := gitRun(repo, "rev-parse", "--is-inside-work-tree")
	if err != nil || inside != "true" {
		return GitDiffResult{}, errors.New("当前目录不是 Git 仓库")
	}
	if status := gitStatusForPath(repo, cleanPath); status != "" {
		result.Status = status
	}
	if view != "staged" && strings.HasPrefix(result.Status, "??") {
		return gitUntrackedDiff(repo, cleanPath, result)
	}
	args := []string{"diff", "--no-ext-diff", "--unified=3", "--no-color"}
	if view == "staged" {
		args = append(args, "--cached")
	} else if view == "all" {
		args = append(args, "HEAD")
	}
	args = append(args, "--", ":(literal)"+cleanPath)
	changes, err := gitWorkspaceChanges(repo)
	if err != nil {
		return GitDiffResult{}, err
	}
	for _, change := range changes {
		if change.Path == cleanPath && change.OriginalPath != "" && !strings.HasPrefix(change.OriginalPath, "../") {
			args = append(args, ":(literal)"+change.OriginalPath)
		}
	}
	output, diffErr := gitRawOutput(repo, args...)
	if diffErr != nil {
		if view == "all" {
			if _, err := gitRun(repo, "rev-parse", "--verify", "HEAD"); err != nil {
				return gitUntrackedDiff(repo, cleanPath, result)
			}
		}
		return GitDiffResult{}, diffErr
	}
	if strings.Contains(output, "Binary files") || strings.Contains(output, "GIT binary patch") {
		result.Binary = true
		result.Truncated = len(output) > maxGitDiffBytes
		result.Diff = truncateGitDiff(output)
		return result, nil
	}
	result.Truncated = len(output) > maxGitDiffBytes
	result.Diff = truncateGitDiff(output)
	return result, nil
}

func gitStatusForPath(repo, cleanPath string) string {
	output, err := gitRawOutput(repo, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--", ":(literal)"+cleanPath)
	if err != nil {
		return ""
	}
	if len(output) >= 2 {
		return strings.TrimSpace(output[:2])
	}
	return ""
}

func gitUntrackedDiff(repo, cleanPath string, result GitDiffResult) (GitDiffResult, error) {
	repoResolved, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return GitDiffResult{}, fmt.Errorf("无法解析仓库目录：%w", err)
	}
	full := filepath.Join(repo, filepath.FromSlash(cleanPath))
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		if os.IsNotExist(err) {
			result.Missing = true
			result.Diff = "文件不存在或已被删除。"
			return result, nil
		}
		return GitDiffResult{}, fmt.Errorf("无法读取变更文件：%w", err)
	}
	rel, err := filepath.Rel(repoResolved, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return GitDiffResult{}, errors.New("变更文件路径无效")
	}
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		result.Missing = true
		result.Diff = "该路径不是可对比的文件。"
		return result, nil
	}
	file, err := os.Open(resolved)
	if err != nil {
		return GitDiffResult{}, fmt.Errorf("无法读取变更文件：%w", err)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxGitDiffBytes+1))
	if err != nil {
		return GitDiffResult{}, fmt.Errorf("无法读取变更文件：%w", err)
	}
	result.Untracked = true
	if bytes.IndexByte(content, 0) >= 0 {
		result.Binary = true
		result.Diff = "二进制文件，无法直接对比。"
		return result, nil
	}
	text := string(content)
	truncated := false
	if len(text) > maxGitDiffBytes {
		text = text[:maxGitDiffBytes]
		truncated = true
	}
	var builder strings.Builder
	for _, line := range strings.Split(text, "\n") {
		builder.WriteByte('+')
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	if truncated {
		builder.WriteString("…（文件内容过长已截断）\n")
	}
	result.Diff = builder.String()
	result.Truncated = truncated
	return result, nil
}

func cleanGitChangePath(changePath string) (string, error) {
	if runtime.GOOS == "windows" {
		changePath = strings.ReplaceAll(changePath, "\\", "/")
	}
	if strings.ContainsRune(changePath, 0) {
		return "", errors.New("变更文件路径无效")
	}
	clean := filepath.ToSlash(filepath.Clean(changePath))
	if clean == "." || clean == ".." || filepath.IsAbs(clean) ||
		strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", errors.New("变更文件路径无效")
	}
	return clean, nil
}

func truncateGitDiff(output string) string {
	if len(output) <= maxGitDiffBytes {
		return output
	}
	cut := maxGitDiffBytes
	if index := strings.LastIndex(output[:cut], "\n"); index > 0 {
		cut = index
	}
	return output[:cut] + "\n…（diff 过长已截断）\n"
}
