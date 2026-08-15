package robot

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxExtractedPackageSize int64 = 500 << 20
const maxPackageJSONSize int64 = 1 << 20

// InstallLocalPackageUpload validates a browser-uploaded plugin archive and
// unpacks it into the robot's packages directory. The destination name comes
// from the package manifest (or the archive's single root directory), never
// from the upload filename, and an existing directory with the same name is
// refused instead of being overwritten.
func (Manager) InstallLocalPackageUpload(root, archivePath string) (LocalPackage, error) {
	project, err := projectPath(root)
	if err != nil {
		return LocalPackage{}, err
	}
	destination := filepath.Join(project, "packages")
	if info, err := os.Lstat(destination); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return LocalPackage{}, errors.New("packages 必须是普通目录")
	} else if err != nil && !os.IsNotExist(err) {
		return LocalPackage{}, fmt.Errorf("无法检查 packages 目录：%w", err)
	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(destination, 0755); err != nil {
			if permissionError(err) {
				return LocalPackage{}, permissionAdvice("创建 packages 目录")
			}
			return LocalPackage{}, fmt.Errorf("无法创建 packages 目录：%w", err)
		}
	}
	staging, err := os.MkdirTemp(destination, ".package-upload-")
	if err != nil {
		return LocalPackage{}, errors.New("无法创建解压目录：" + err.Error())
	}
	defer os.RemoveAll(staging)
	if err := extractPackageArchive(archivePath, staging); err != nil {
		return LocalPackage{}, err
	}
	source, err := locatePackageRoot(staging)
	if err != nil {
		return LocalPackage{}, err
	}
	manifest, err := readPackageManifest(source)
	if err != nil {
		return LocalPackage{}, err
	}
	name := strings.TrimSpace(manifest.Name)
	if name == "" {
		name = filepath.Base(source)
	}
	name = filepath.Base(name)
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\`) || len([]rune(name)) > 180 {
		return LocalPackage{}, errors.New("上传包 package.json 名称无效")
	}
	target := filepath.Join(destination, name)
	if _, err := os.Lstat(target); err == nil {
		return LocalPackage{}, fmt.Errorf("背包中已存在同名插件包 %s", name)
	} else if !os.IsNotExist(err) {
		return LocalPackage{}, fmt.Errorf("无法检查目标目录：%w", err)
	}
	if err := os.Rename(source, target); err != nil {
		return LocalPackage{}, fmt.Errorf("保存插件包失败：%w", err)
	}
	return LocalPackage{Name: manifest.Name, Version: manifest.Version, Description: manifest.Description, Path: target, Valid: true}, nil
}

func readPackageManifest(directory string) (struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}, error) {
	var manifest struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
	}
	info, err := os.Stat(filepath.Join(directory, "package.json"))
	if err != nil || info.IsDir() || info.Size() > maxPackageJSONSize {
		return manifest, errors.New("上传包缺少有效的 package.json")
	}
	data, err := os.ReadFile(filepath.Join(directory, "package.json"))
	if err != nil {
		return manifest, errors.New("无法读取上传包 package.json")
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, errors.New("上传包 package.json 无法解析")
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return manifest, errors.New("上传包 package.json 缺少 name")
	}
	return manifest, nil
}

// locatePackageRoot accepts either an archive that contains package.json at
// its root or a single wrapper directory containing package.json.
func locatePackageRoot(staging string) (string, error) {
	if _, err := os.Stat(filepath.Join(staging, "package.json")); err == nil {
		return staging, nil
	}
	entries, err := os.ReadDir(staging)
	if err != nil {
		return "", errors.New("无法读取上传包内容")
	}
	if len(entries) == 1 && entries[0].IsDir() {
		candidate := filepath.Join(staging, entries[0].Name())
		if _, err := os.Stat(filepath.Join(candidate, "package.json")); err == nil {
			return candidate, nil
		}
	}
	return "", errors.New("上传包中未找到 package.json")
}

func extractPackageArchive(source, destination string) error {
	switch packageArchiveFormat(source) {
	case "zip":
		return extractPackageZip(source, destination)
	case "tar":
		return extractPackageTarGz(source, destination)
	}
	return errors.New("插件包格式不受支持")
}

// packageArchiveFormat identifies the archive kind from the filename first,
// then falls back to magic bytes so browser uploads that reach a temporary
// path without an extension are still validated.
func packageArchiveFormat(path string) string {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".zip") {
		return "zip"
	}
	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		return "tar"
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	var magic [2]byte
	if _, err := io.ReadFull(file, magic[:]); err != nil {
		return ""
	}
	switch {
	case magic[0] == 'P' && magic[1] == 'K':
		return "zip"
	case magic[0] == 0x1f && magic[1] == 0x8b:
		return "tar"
	}
	return ""
}

func safePackagePath(root, name string) (string, error) {
	name = filepath.Clean(filepath.FromSlash(name))
	if name == "." || filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
		return "", errors.New("插件包包含非法路径")
	}
	target := filepath.Join(root, name)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("插件包路径越界")
	}
	return target, nil
}

func extractPackageZip(source, destination string) error {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return errors.New("插件包压缩包无法读取")
	}
	defer archive.Close()
	var total int64
	for _, entry := range archive.File {
		target, err := safePackagePath(destination, entry.Name)
		if err != nil {
			return err
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return errors.New("插件包不允许包含符号链接")
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if !entry.FileInfo().Mode().IsRegular() {
			return errors.New("插件包包含不支持的文件类型")
		}
		total += int64(entry.UncompressedSize64)
		if total > maxExtractedPackageSize {
			return errors.New("插件包解压内容过大")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		input, err := entry.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, entry.Mode().Perm()|0600)
		if err == nil {
			_, err = io.Copy(output, io.LimitReader(input, maxExtractedPackageSize+1))
			_ = output.Close()
		}
		_ = input.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractPackageTarGz(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return errors.New("插件包压缩包无法读取")
	}
	defer input.Close()
	gzipReader, err := gzip.NewReader(input)
	if err != nil {
		return errors.New("插件包压缩包无法读取")
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	var total int64
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.New("插件包压缩包无法读取")
		}
		target, err := safePackagePath(destination, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 {
				return errors.New("插件包包含非法文件")
			}
			total += header.Size
			if total > maxExtractedPackageSize {
				return errors.New("插件包解压内容过大")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode)&0777|0600)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(output, io.LimitReader(reader, maxExtractedPackageSize+1))
			_ = output.Close()
			if copyErr != nil {
				return copyErr
			}
		default:
			return errors.New("插件包不允许包含链接或特殊文件")
		}
	}
	return nil
}
