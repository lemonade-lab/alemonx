package robot

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

var (
	ErrProjectFileRevision = errors.New("文件已被修改，请重新加载磁盘版本后再保存。")
	ErrProjectFileNotText  = errors.New("此文件无法作为 UTF-8 文本打开。")
)

type ProjectTextFile struct {
	Content  string `json:"content"`
	Revision string `json:"revision"`
}

func readRegularProjectFile(path string) ([]byte, os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("读取失败：%w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, nil, errors.New("只能读取普通项目文件")
	}
	if info.Size() > maxMCPFileSize {
		return nil, nil, errors.New("文件超过 1 MiB，无法读取")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("读取失败：%w", err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, nil, ErrProjectFileRevision
	}
	data, err := io.ReadAll(io.LimitReader(file, maxMCPFileSize+1))
	if err != nil {
		return nil, nil, fmt.Errorf("读取失败：%w", err)
	}
	if len(data) > maxMCPFileSize {
		return nil, nil, errors.New("文件超过 1 MiB，无法读取")
	}
	return data, opened, nil
}

func validProjectText(content string) bool {
	return utf8.ValidString(content) && !strings.ContainsRune(content, 0)
}

func projectText(content string) ProjectTextFile {
	return ProjectTextFile{Content: content, Revision: fileContentRevision(content)}
}

func (Manager) ReadWorkspaceProjectText(root, name string) (ProjectTextFile, error) {
	path, err := managedProjectFile(root, name)
	if err != nil {
		return ProjectTextFile{}, err
	}
	data, _, err := readRegularProjectFile(path)
	if err != nil {
		return ProjectTextFile{}, err
	}
	if !validProjectText(string(data)) {
		return ProjectTextFile{}, ErrProjectFileNotText
	}
	return projectText(string(data)), nil
}

// SaveWorkspaceProjectText updates an existing file only when its contents
// still match the revision presented by the editor. Each workbench write is
// serialized with other editor saves and managed runtime config changes.
func (Manager) SaveWorkspaceProjectText(root, name, content, revision string) (ProjectTextFile, error) {
	if len(content) > maxMCPFileSize {
		return ProjectTextFile{}, errors.New("文件内容超过 1 MiB，无法保存。")
	}
	if !validProjectText(content) {
		return ProjectTextFile{}, ErrProjectFileNotText
	}
	if revision == "" {
		return ProjectTextFile{}, errors.New("缺少文件版本，请重新打开文件。")
	}
	project, err := projectPath(root)
	if err != nil {
		return ProjectTextFile{}, err
	}
	defer lockProjectFiles(project)()

	path, err := managedProjectFile(project, name)
	if err != nil {
		return ProjectTextFile{}, err
	}
	data, info, err := readRegularProjectFile(path)
	if err != nil {
		return ProjectTextFile{}, err
	}
	current := string(data)
	if !validProjectText(current) {
		return ProjectTextFile{}, ErrProjectFileNotText
	}
	mode := info.Mode().Perm()
	if sameWorkspacePath(path, filepath.Join(project, "alemon.config.yaml")) {
		content = normalizeRuntimeConfigYAML(content)
		mode = 0o600
	}
	if revision != fileContentRevision(current) && content != current {
		return ProjectTextFile{}, ErrProjectFileRevision
	}
	if content != current {
		err := writeFileAtomicallyAfterCheck(path, []byte(content), mode, func() error {
			if _, err := managedProjectFile(project, name); err != nil {
				return ErrProjectFileRevision
			}
			latest, latestInfo, err := readRegularProjectFile(path)
			if err != nil || !os.SameFile(info, latestInfo) || string(latest) != current {
				return ErrProjectFileRevision
			}
			return nil
		})
		if err != nil {
			return ProjectTextFile{}, fmt.Errorf("保存失败：%w", err)
		}
	}
	return projectText(content), nil
}
