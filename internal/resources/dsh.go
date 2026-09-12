package resources

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const dshEntry = "node_modules/@deepseek-ai/dsh/lib/bin.js"

var dshRequiredFiles = []string{dshEntry, "plugins/approval-bridge/package.json", "plugins/approval-bridge/index.js", "plugins/approval-bridge/session-resume.js"}

func completeDSHPackage(dir string) bool {
	for _, name := range append([]string{".complete"}, dshRequiredFiles...) {
		info, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}

// Development can publish a complete archive after the server has started.
// A failed refresh retains the previously usable package.
func SetDSHPreparation(err error) {
	mu.Lock()
	defer mu.Unlock()
	if err != nil {
		provisionErrors["dsh"] = err.Error()
	} else {
		delete(provisionErrors, "dsh")
		delete(materialized, "dsh")
	}
}

// DSHProgram materializes the exact sidecar shipped in this executable.
// Versions are immutable and retained so rolling back alx also rolls back DSH.
func DSHProgram() (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if embedded == nil || workspaceRoot == "" {
		return "", fmt.Errorf("DSH 工作区尚未初始化")
	}
	if entry := materialized["dsh"]; entry != "" {
		root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(entry)))))
		if completeDSHPackage(root) {
			return entry, nil
		}
	}
	data, err := fs.ReadFile(embedded, "dsh/runtime.zip")
	if err != nil {
		if message := provisionErrors["dsh"]; message != "" {
			return "", fmt.Errorf("%s", message)
		}
		return "", fmt.Errorf("安装包缺少 DSH 程序，请使用完整构建：%w", err)
	}
	entry, err := materializeDSH(data, filepath.Join(workspaceRoot, "dsh", "packages", runtime.GOOS+"-"+runtime.GOARCH))
	if err == nil {
		materialized["dsh"] = entry
	}
	return entry, err
}

func materializeDSH(data []byte, base string) (string, error) {
	sum := sha256.Sum256(data)
	target := filepath.Join(base, hex.EncodeToString(sum[:]))
	entry := filepath.Join(target, filepath.FromSlash(dshEntry))
	if completeDSHPackage(target) {
		return entry, nil
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		return "", err
	}
	// Serialize publishers across workbench processes, not only goroutines.
	lock, err := os.OpenFile(filepath.Join(base, ".install.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	if err := lockDSHPackage(lock); err != nil {
		return "", err
	}
	defer unlockDSHPackage(lock)
	if completeDSHPackage(target) {
		return entry, nil
	}
	staging, err := os.MkdirTemp(base, ".prepare-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	var remaining int64 = 512 << 20
	for _, item := range archive.File {
		name := item.Name
		if name == "alx-runtime-platform.json" {
			in, err := item.Open()
			if err != nil {
				return "", err
			}
			var platform struct {
				OS   string `json:"os"`
				Arch string `json:"arch"`
			}
			err = json.NewDecoder(io.LimitReader(in, 4096)).Decode(&platform)
			in.Close()
			if err != nil || platform.OS != runtime.GOOS || platform.Arch != runtime.GOARCH {
				return "", fmt.Errorf("DSH 程序包平台不匹配，需要 %s/%s", runtime.GOOS, runtime.GOARCH)
			}
		}
		if !fs.ValidPath(name) || strings.ContainsAny(name, "\\:") || item.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("DSH 程序包路径无效：%s", name)
		}
		output := filepath.Join(staging, filepath.FromSlash(name))
		if item.FileInfo().IsDir() {
			continue
		}
		if item.UncompressedSize64 > uint64(remaining) {
			return "", fmt.Errorf("DSH 程序包过大")
		}
		if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
			return "", err
		}
		in, err := item.Open()
		if err != nil {
			return "", err
		}
		out, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, item.Mode().Perm()&0755|0600)
		if err != nil {
			in.Close()
			return "", err
		}
		n, copyErr := io.Copy(out, io.LimitReader(in, remaining+1))
		in.Close()
		closeErr := out.Close()
		remaining -= n
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		if remaining < 0 {
			return "", fmt.Errorf("DSH 程序包过大")
		}
	}
	for _, required := range dshRequiredFiles {
		if info, err := os.Lstat(filepath.Join(staging, filepath.FromSlash(required))); err != nil || !info.Mode().IsRegular() {
			return "", fmt.Errorf("DSH 程序包缺少 %s", required)
		}
	}
	if err := os.WriteFile(filepath.Join(staging, ".complete"), []byte("1\n"), 0600); err != nil {
		return "", err
	}
	// Validate the replacement first. Quarantine the damaged program only;
	// never delete old contents or touch runtimes/events while repairing.
	backup := ""
	if _, err := os.Lstat(target); err == nil {
		backup, err = os.MkdirTemp(base, ".damaged-")
		if err != nil {
			return "", err
		}
		backup = filepath.Join(backup, filepath.Base(target))
		if err := os.Rename(target, backup); err != nil {
			return "", err
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Rename(staging, target); err != nil {
		if backup != "" {
			if restoreErr := os.Rename(backup, target); restoreErr != nil {
				return "", fmt.Errorf("DSH 修复发布失败：%v；原目录保留于 %s（恢复失败：%v）", err, backup, restoreErr)
			}
		}
		return "", err
	}
	if backup != "" {
		log.Printf("DSH 程序目录已自动修复；原目录保留于 %s", backup)
	}
	return entry, nil
}
