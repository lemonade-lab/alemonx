package agent

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"alemonx/internal/system"
)

type Worktree struct {
	Root string
	Base string
}

func CreateWorktree(root, taskID string) (Worktree, error) {
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return Worktree{}, errors.New("项目不是 Git 仓库")
	}
	dir, err := os.MkdirTemp("", "alx-agent-worktree-")
	if err != nil {
		return Worktree{}, err
	}
	_ = os.RemoveAll(dir)
	git, err := system.ResolveCommand("git")
	if err != nil {
		return Worktree{}, errors.New("未检测到 Git；请先在环境中安装 Git")
	}
	if out, err := exec.Command(git, "-C", root, "worktree", "add", "--detach", dir, "HEAD").CombinedOutput(); err != nil {
		return Worktree{}, errors.New(strings.TrimSpace(string(out)))
	}
	return Worktree{Root: dir, Base: root}, nil
}

func (w Worktree) Diff() string {
	git, err := system.ResolveCommand("git")
	if err != nil {
		return ""
	}
	out, _ := exec.Command(git, "-C", w.Root, "diff", "--no-ext-diff").CombinedOutput()
	return string(out)
}
func (w Worktree) Remove() {
	if git, err := system.ResolveCommand("git"); err == nil {
		_, _ = exec.Command(git, "-C", w.Base, "worktree", "remove", "--force", w.Root).CombinedOutput()
	}
	_ = os.RemoveAll(w.Root)
}
