package robot

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
)

type ProjectDirectoryEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
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
