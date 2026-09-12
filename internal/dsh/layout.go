package dsh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Copy into staging and publish only complete trees. The old home remains a
// backup; an interrupted copy is never mistaken for a completed migration.
func copyPrivateTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		// The SDK heals these installation-dependent fallbacks on every launch.
		// They can point to a removed installation; never follow or migrate them.
		parts := strings.Split(filepath.ToSlash(rel), "/")
		generated := filepath.ToSlash(rel) == "profiles/node_modules" ||
			(len(parts) == 3 && parts[0] == "profiles" && parts[2] == ".dsh-module-fallback")
		if generated || filepath.ToSlash(rel) == "node_modules/@alemonx/dsh-approval-bridge" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			// Profile-local projections into the SDK-owned fallback are also
			// regenerated. Keep arbitrary user-installed profile packages intact.
			if len(parts) >= 4 && parts[0] == "profiles" && parts[2] == "node_modules" {
				owned := filepath.Join(source, "profiles", parts[1], ".dsh-module-fallback", "node_modules", filepath.Join(parts[3:]...))
				linked := link
				if !filepath.IsAbs(linked) {
					linked = filepath.Join(filepath.Dir(path), linked)
				}
				if canonicalRoot(linked) == canonicalRoot(owned) {
					return nil
				}
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(rel), link))
			if filepath.IsAbs(link) || resolved == ".." || strings.HasPrefix(resolved, ".."+string(filepath.Separator)) {
				return fmt.Errorf("DSH 旧数据包含越界链接：%s", path)
			}
			return os.Symlink(link, target)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("DSH 旧数据包含特殊文件：%s", path)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm()&0700|0600)
		if err != nil {
			in.Close()
			return err
		}
		_, err = io.Copy(out, in)
		in.Close()
		closeErr := out.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
}

func migrateRuntimeHome(old, target string) error {
	if _, err := os.Stat(target); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(old); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(target), ".migrate-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := copyPrivateTree(old, staging); err != nil {
		return err
	}
	return os.Rename(staging, target)
}

func (r *RuntimeRegistry) MigrateLegacy() error {
	if r.initErr != nil {
		return r.initErr
	}
	if r.legacyDir == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(r.legacyDir, "runtimes"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if err := migrateRuntimeHome(filepath.Join(r.legacyDir, "runtimes", entry.Name()), filepath.Join(r.baseDir, "runtimes", entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func NewWorkspaceRegistry(root string) *RuntimeRegistry {
	r := NewRegistry(filepath.Join(root, "dsh"), nil)
	if root == "" || !filepath.IsAbs(root) {
		r.initErr = fmt.Errorf("DSH 工作区不可用")
		return r
	}
	r.legacyDir, r.initErr = DefaultHome()
	return r
}

func acquireRuntimeLock(home string) (*os.File, error) {
	file, err := os.OpenFile(filepath.Join(home, ".runtime.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockRuntime(file); err != nil {
		file.Close()
		return nil, fmt.Errorf("DSH 运行数据正在被另一个工作台实例使用：%s", home)
	}
	return file, nil
}

// Relocate rebinds an existing runtime without renaming its storage directory.
// It preserves sessions and SDK state, and never deletes the old project.
func (r *RuntimeRegistry) Relocate(ctx context.Context, oldRoot, newRoot string) error {
	if !filepath.IsAbs(oldRoot) || !filepath.IsAbs(newRoot) {
		return fmt.Errorf("迁移必须指定绝对目录")
	}
	oldRoot, newRoot = canonicalRoot(oldRoot), canonicalRoot(newRoot)
	if oldRoot == newRoot {
		return nil
	}
	old, err := r.Runtime(oldRoot)
	if err != nil {
		return err
	}
	persisted, err := old.LoadPersistedConfig()
	if err != nil {
		return err
	}
	next, err := r.Runtime(newRoot)
	if err != nil {
		return err
	}
	if _, err := next.LoadPersistedConfig(); err == nil {
		return fmt.Errorf("目标机器人已有 DSH 数据，不能覆盖")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := old.Stop(ctx); err != nil {
		return err
	}
	backup, err := os.CreateTemp(old.dir, "relocation-*.json")
	if err != nil {
		return err
	}
	err = json.NewEncoder(backup).Encode(persisted)
	closeErr := backup.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	persisted.Root = newRoot
	if err := old.savePersistedConfig(persisted); err != nil {
		return err
	}
	r.mu.Lock()
	delete(r.runtimes, oldRoot)
	r.runtimes[newRoot] = old
	r.mu.Unlock()
	return nil
}
