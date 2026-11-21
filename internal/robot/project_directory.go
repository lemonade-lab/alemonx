package robot

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var ErrProjectEntryExists = errors.New("项目中已有同名文件或目录。")

type ProjectDirectoryEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
}

// CreateWorkspaceProjectEntry creates an empty file or directory in the project root.
func (Manager) CreateWorkspaceProjectEntry(root, name string, directory bool) (ProjectDirectoryEntry, error) {
	name = strings.TrimSpace(name)
	if !filepath.IsLocal(name) || name == "." || strings.HasSuffix(name, ".") || strings.ContainsAny(name, "/\\:<>\"|?*\x00\r\n\t") {
		return ProjectDirectoryEntry{}, errors.New("请输入有效的文件或目录名称。")
	}
	path, err := managedProjectFile(root, name)
	if err != nil {
		return ProjectDirectoryEntry{}, err
	}
	if directory {
		err = os.Mkdir(path, 0755)
	} else {
		var file *os.File
		file, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err == nil {
			err = file.Close()
		}
	}
	if errors.Is(err, os.ErrExist) {
		return ProjectDirectoryEntry{}, ErrProjectEntryExists
	}
	if permissionError(err) {
		return ProjectDirectoryEntry{}, permissionAdvice("创建 " + name)
	}
	if err != nil {
		return ProjectDirectoryEntry{}, err
	}
	return ProjectDirectoryEntry{Name: name, Path: name, Directory: directory}, nil
}

// ListProjectDirectory lists one level of a managed project using the same
// exclusions as the project file reader.
func (Manager) ListProjectDirectory(root, directory string) ([]ProjectDirectoryEntry, error) {
	var path string
	var err error
	if directory == "" {
		path, err = projectPath(root)
	} else {
		path, err = managedProjectFile(root, directory)
	}
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return nil, errors.New("无法读取项目目录")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, errors.New("无法读取项目目录")
	}
	result := make([]ProjectDirectoryEntry, 0, len(entries))
	for _, entry := range entries {
		relative := filepath.Join(directory, entry.Name())
		if blockedProjectPath(relative) || (!entry.IsDir() && !entry.Type().IsRegular()) {
			continue
		}
		result = append(result, ProjectDirectoryEntry{
			Name: entry.Name(), Path: filepath.ToSlash(relative), Directory: entry.IsDir(),
		})
	}
	// ReadDir supplies the name order within each group.
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Directory && !result[j].Directory
	})
	return result, nil
}
