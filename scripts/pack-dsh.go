//go:build ignore

// Run after npm ci in resources/packages/dsh/. An archive keeps the sidecar in the same
// release/rollback unit as alx, without embedding npm symlinks into embed.FS.
package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func main() {
	if err := pack(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func pack() error {
	const source = "resources/packages/dsh"
	for _, path := range []string{source + "/node_modules/@deepseek-ai/dsh/lib/bin.js", source + "/plugins/approval-bridge/package.json"} {
		if _, err := os.Stat(path); err != nil {
			return err
		}
	}
	if err := os.MkdirAll("resources/dsh", 0755); err != nil {
		return err
	}
	file, err := os.CreateTemp("resources/dsh", ".runtime-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	archive := zip.NewWriter(file)
	targetOS, targetArch := os.Getenv("ALX_DSH_TARGET_OS"), os.Getenv("ALX_DSH_TARGET_ARCH")
	if targetOS == "" {
		targetOS = runtime.GOOS
	}
	if targetArch == "" {
		targetArch = runtime.GOARCH
	}
	meta, err := archive.Create("alx-runtime-platform.json")
	if err != nil {
		archive.Close()
		file.Close()
		return err
	}
	if err := json.NewEncoder(meta).Encode(map[string]string{"os": targetOS, "arch": targetArch}); err != nil {
		archive.Close()
		file.Close()
		return err
	}
	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		// Invoke the JS entry through selected Node, never platform-specific .bin shims.
		if strings.Contains(filepath.ToSlash(rel), "node_modules/.bin/") {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported DSH file: %s", path)
		}
		header := &zip.FileHeader{Name: filepath.ToSlash(rel), Method: zip.Deflate}
		header.SetMode(info.Mode().Perm())
		header.Modified = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
		out, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, in)
		closeErr := in.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
	zipErr := archive.Close()
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if zipErr != nil {
		return zipErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), "resources/dsh/runtime.zip")
}
