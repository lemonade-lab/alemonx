// Package workspace defines the single runtime working directory used by
// ALemonX. All user-editable runtime state that is not stored in the user
// configuration directory lives under one root:
//
//	<root>/
//	  templates/   materialized AlemonJS project templates (editable)
//	  bots/        default destination for newly created robots
//	  plugins/     user-installed system plugins
//
// The bundled templates stay embedded in the binary and are copied here on
// first run; files that already exist on disk are never overwritten so user
// customizations survive upgrades.
package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// TemplateVersion is bumped whenever the bundled templates change in a way
// that should be surfaced to users as "templates can be refreshed".
const TemplateVersion = "1"

const templateMarker = ".alemonx-version"

// Layout is the resolved absolute workspace root and its well-known
// subdirectories.
type Layout struct {
	Root string
}

// Templates returns the directory that stores the runtime template sources.
func (l Layout) Templates() string {
	return filepath.Join(l.Root, "templates")
}

// Bots returns the directory where newly created robots land by default.
func (l Layout) Bots() string {
	return filepath.Join(l.Root, "bots")
}

// Packages returns the directory where bundled runtime tools are materialized.
func (l Layout) Packages() string {
	return filepath.Join(l.Root, "packages")
}

// Plugins returns the directory that stores user-installed system plugins.
// It is deliberately part of the workspace so a Docker bind mount preserves
// plugin installations across container replacement.
func (l Layout) Plugins() string {
	return filepath.Join(l.Root, "plugins")
}

// TemplatesOutdated reports whether the materialized templates were produced
// by an older embedded template version. It never modifies the copy.
func TemplatesOutdated(root string) bool {
	data, err := os.ReadFile(filepath.Join(root, "templates", templateMarker))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) != TemplateVersion
}

// ResolveRoot returns the absolute workspace root. Precedence:
//
//  1. an explicit value (the --workspace flag),
//  2. the ALX_WORKSPACE environment variable,
//  3. the first writable ALEMONJS_SETUP_ROOTS entry (Docker deployments),
//  4. <cwd>/workspace.
//
// It only resolves the path and never creates directories, so read-only CLI
// commands do not touch the filesystem.
func ResolveRoot(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return absolute(explicit)
	}
	if value := strings.TrimSpace(os.Getenv("ALX_WORKSPACE")); value != "" {
		return absolute(value)
	}
	if root, err := firstWritableSetupRoot(); err == nil {
		return root, nil
	}
	current, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("无法读取当前工作目录：%w", err)
	}
	return filepath.Join(current, "workspace"), nil
}

// Ensure resolves the workspace root, creates templates/, bots/ and plugins/,
// and materializes the bundled templates into templates/ when they are missing.
func Ensure(explicit string, templates fs.FS) (Layout, error) {
	root, err := ResolveRoot(explicit)
	if err != nil {
		return Layout{}, err
	}
	layout := Layout{Root: root}
	if err := os.MkdirAll(layout.Root, 0o755); err != nil {
		return Layout{}, fmt.Errorf("无法创建工作区目录 %s：%w", layout.Root, err)
	}
	if err := os.MkdirAll(layout.Templates(), 0o755); err != nil {
		return Layout{}, fmt.Errorf("无法创建工作区模板目录 %s：%w", layout.Templates(), err)
	}
	if err := os.MkdirAll(layout.Bots(), 0o755); err != nil {
		return Layout{}, fmt.Errorf("无法创建工作区机器人目录 %s：%w", layout.Bots(), err)
	}
	if err := os.MkdirAll(layout.Packages(), 0o755); err != nil {
		return Layout{}, fmt.Errorf("无法创建工作区工具目录 %s：%w", layout.Packages(), err)
	}
	if err := os.MkdirAll(layout.Plugins(), 0o755); err != nil {
		return Layout{}, fmt.Errorf("无法创建工作区插件目录 %s：%w", layout.Plugins(), err)
	}
	if templates != nil {
		if err := materializeTemplates(templates, layout.Templates()); err != nil {
			return Layout{}, err
		}
	}
	return layout, nil
}

// materializeTemplates copies every top-level template directory from the
// embedded source into target. Files already present on disk are kept as-is;
// only missing files are copied. The version marker is written once after a
// fresh materialization so later upgrades can tell users a refresh is
// available without silently clobbering edits.
func materializeTemplates(source fs.FS, target string) error {
	entries, err := fs.ReadDir(source, ".")
	if err != nil {
		return fmt.Errorf("无法读取内嵌模板：%w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := copyMissingTree(source, entry.Name(), target); err != nil {
			return err
		}
	}
	marker := filepath.Join(target, templateMarker)
	if _, err := os.Stat(marker); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(marker, []byte(TemplateVersion), 0o644); err != nil {
			return fmt.Errorf("无法写入模板版本标记：%w", err)
		}
	}
	return nil
}

func copyMissingTree(source fs.FS, name, target string) error {
	return fs.WalkDir(source, name, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// path already contains the top-level template directory (for example
		// "bot/package.json"), so it is preserved under target as-is.
		output := filepath.Join(target, path)
		if entry.IsDir() {
			return os.MkdirAll(output, 0o755)
		}
		if _, err := os.Stat(output); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		data, err := fs.ReadFile(source, path)
		if err != nil {
			return err
		}
		return os.WriteFile(output, data, 0o644)
	})
}

func absolute(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("无法解析工作区路径 %s：%w", path, err)
	}
	return filepath.Clean(absolute), nil
}

func firstWritableSetupRoot() (string, error) {
	value := strings.TrimSpace(os.Getenv("ALEMONJS_SETUP_ROOTS"))
	if value == "" {
		return "", errors.New("未配置可写的保存根目录")
	}
	for _, root := range filepath.SplitList(value) {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		if err := writable(root); err == nil {
			absolute, absErr := filepath.Abs(root)
			if absErr != nil {
				return "", absErr
			}
			return filepath.Clean(absolute), nil
		}
	}
	return "", errors.New("没有可写的保存根目录")
}

func writable(directory string) error {
	file, err := os.CreateTemp(directory, ".alemonx-write-check-")
	if err != nil {
		return err
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}
