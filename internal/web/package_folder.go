package web

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
)

// Browser-selected directories carry relative paths, never server filesystem paths.
func (s *server) robotPackageFolderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	const limit int64 = 500 << 20
	r.Body = http.MaxBytesReader(w, r.Body, limit+(10<<20))
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, 400, "请选择一个插件文件夹。")
		return
	}
	tmp, err := os.CreateTemp("", "alx-folder-*.zip")
	if err != nil {
		writeError(w, 500, "无法准备安装目录。")
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	archive := zip.NewWriter(tmp)
	defer archive.Close()
	seen := map[string]bool{}
	var size int64
	for {
		part, readErr := reader.NextPart()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			writeError(w, 400, "读取文件夹失败，或文件夹超过 500 MB。")
			return
		}
		name := strings.TrimPrefix(part.FormName(), "files:")
		invalid := !strings.HasPrefix(part.FormName(), "files:") || name == "" || path.IsAbs(name) || path.Clean(name) != name || strings.ContainsAny(name, "\\:\x00")
		for _, segment := range strings.Split(name, "/") {
			if strings.HasPrefix(segment, ".") || segment == "node_modules" {
				invalid = true
			}
		}
		key := strings.ToLower(name)
		if invalid || seen[key] || len(seen) >= 10000 {
			writeError(w, 400, "文件路径无效、重复，或文件数量超过 10000。")
			return
		}
		seen[key] = true
		file, createErr := archive.Create(name)
		if createErr != nil {
			writeError(w, 500, "无法准备插件文件。")
			return
		}
		n, copyErr := io.Copy(file, io.LimitReader(part, limit-size+1))
		size += n
		if copyErr != nil || size > limit {
			writeError(w, 400, "文件夹读取失败，或超过 500 MB。")
			return
		}
	}
	if !seen["package.json"] {
		writeError(w, 400, "请选择根目录包含 package.json 的插件文件夹。")
		return
	}
	if err := archive.Close(); err != nil {
		writeError(w, 500, "无法保存插件文件。")
		return
	}
	if err := tmp.Close(); err != nil {
		writeError(w, 500, "无法保存插件文件。")
		return
	}
	item, err := s.robots.InstallLocalPackageUpload(r.URL.Query().Get("root"), tmp.Name())
	if err != nil {
		writeError(w, 400, fmt.Sprint(err))
		return
	}
	writeJSON(w, http.StatusOK, item)
}
