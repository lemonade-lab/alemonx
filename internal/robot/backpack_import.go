package robot

import (
	"archive/zip"
	"encoding/base64"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The base64 form leaves room for the JSON-RPC envelope within MCP's 1 MiB request.
const MaxInlineBackpackArchiveSize = 700 << 10

func (m Manager) ImportInlineBackpackArchive(root, encoded string) (LocalPackage, error) {
	archive, err := os.CreateTemp("", "alx-inline-backpack-*")
	if err != nil {
		return LocalPackage{}, err
	}
	defer os.Remove(archive.Name())
	reader := base64.NewDecoder(base64.StdEncoding, strings.NewReader(encoded))
	size, err := io.Copy(archive, io.LimitReader(reader, MaxInlineBackpackArchiveSize+1))
	closeErr := archive.Close()
	if err != nil || size == 0 || size > MaxInlineBackpackArchiveSize {
		return LocalPackage{}, errors.New("内联插件压缩包必须为有效 Base64，且不超过 700 KiB")
	}
	if closeErr != nil {
		return LocalPackage{}, closeErr
	}
	return m.InstallLocalPackageUpload(root, archive.Name())
}

// ImportLocalBackpackPlugin accepts a server-local source inside the robot
// directory and reuses the workbench archive installer.
func (m Manager) ImportLocalBackpackPlugin(root, source string) (LocalPackage, error) {
	path, err := managedProjectFile(root, filepath.FromSlash(source))
	if err != nil {
		return LocalPackage{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return LocalPackage{}, err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return LocalPackage{}, errors.New("请选择普通插件文件夹或压缩包")
	}
	archive, err := os.CreateTemp("", "alx-local-backpack-*")
	if err != nil {
		return LocalPackage{}, err
	}
	defer os.Remove(archive.Name())
	if info.IsDir() {
		err = packLocalBackpackFolder(path, archive)
	} else {
		// Browser uploads detect the archive format from its contents as well.
		err = copyBackpackImport(path, archive, info, maxExtractedPackageSize)
	}
	closeErr := archive.Close()
	if err != nil {
		return LocalPackage{}, err
	}
	if closeErr != nil {
		return LocalPackage{}, closeErr
	}
	// The temporary filename is extensionless so the installer detects ZIP
	// and gzip archives by content rather than by the client-supplied name.
	return m.InstallLocalPackageUpload(root, archive.Name())
}

func packLocalBackpackFolder(root string, output io.Writer) error {
	writer := zip.NewWriter(output)
	seen := make(map[string]bool)
	remaining := maxExtractedPackageSize
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || path == root {
			return err
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(entry.Name(), ".") || blockedProjectPath(name) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		name = filepath.ToSlash(name)
		key := strings.ToLower(name)
		if seen[key] || len(seen) >= 10000 || strings.ContainsAny(name, "\\:") {
			return errors.New("插件路径无效、重复，或文件数量超过 10000")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("插件文件夹只能包含普通文件")
		}
		target, err := writer.Create(name)
		if err != nil {
			return err
		}
		if err := copyBackpackImport(path, target, info, remaining); err != nil {
			return err
		}
		seen[key] = true
		remaining -= info.Size()
		return nil
	})
	closeErr := writer.Close()
	if err != nil {
		return err
	}
	if !seen["package.json"] {
		return errors.New("请选择根目录包含 package.json 的插件文件夹")
	}
	return closeErr
}

func copyBackpackImport(path string, target io.Writer, expected os.FileInfo, limit int64) error {
	if expected.Size() > limit {
		return errors.New("插件导入不能超过 500 MB")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	actual, err := file.Stat()
	if err != nil {
		return err
	}
	if !actual.Mode().IsRegular() || !os.SameFile(expected, actual) {
		return errors.New("源文件已变更，请重新选择")
	}
	size, err := io.Copy(target, io.LimitReader(file, expected.Size()+1))
	if err != nil {
		return err
	}
	if size != expected.Size() {
		return errors.New("源文件大小已变更，请重新选择")
	}
	return nil
}
