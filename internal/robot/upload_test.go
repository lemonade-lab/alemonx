package robot

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeUploadArchive(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, content := range entries {
		file, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestInstallLocalPackageUploadUnpacksIntoBackpack(t *testing.T) {
	root := t.TempDir()
	writeWebViewFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	archivePath := filepath.Join(t.TempDir(), "plugin.zip")
	archive := writeUploadArchive(t, map[string]string{
		"plugin/package.json": `{"name":"hello-plugin","version":"1.2.3","description":"hello"}`,
		"plugin/index.js":     "module.exports = {}",
	})
	if err := os.WriteFile(archivePath, archive, 0600); err != nil {
		t.Fatal(err)
	}
	item, err := (Manager{}).InstallLocalPackageUpload(root, archivePath)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if item.Name != "hello-plugin" || item.Version != "1.2.3" || !item.Valid {
		t.Fatalf("uploaded item = %#v", item)
	}
	if item.Path != filepath.Join(root, "packages", "hello-plugin") {
		t.Fatalf("path = %q", item.Path)
	}
	if _, err := os.Stat(filepath.Join(root, "packages", "hello-plugin", "package.json")); err != nil {
		t.Fatalf("package directory missing: %v", err)
	}
	items, err := (Manager{}).LocalPackages(root)
	if err != nil || len(items) != 1 || items[0].Name != "hello-plugin" {
		t.Fatalf("backpack after upload = %#v, %v", items, err)
	}
	// Uploading the same package again must be refused.
	if _, err := (Manager{}).InstallLocalPackageUpload(root, archivePath); err == nil || !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("duplicate upload error = %v", err)
	}
}

func TestInstallLocalPackageUploadRejectsInvalidPackage(t *testing.T) {
	root := t.TempDir()
	writeWebViewFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	archivePath := filepath.Join(t.TempDir(), "bad.zip")
	if err := os.WriteFile(archivePath, writeUploadArchive(t, map[string]string{"readme.txt": "hi"}), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Manager{}).InstallLocalPackageUpload(root, archivePath); err == nil {
		t.Fatal("archive without package.json must be rejected")
	}
}

func TestInstallLocalPackageUploadRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	writeWebViewFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	archivePath := filepath.Join(t.TempDir(), "evil.zip")
	if err := os.WriteFile(archivePath, writeUploadArchive(t, map[string]string{
		"package.json":  `{"name":"safe"}`,
		"../escape.txt": "nope",
	}), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Manager{}).InstallLocalPackageUpload(root, archivePath); err == nil {
		t.Fatal("path traversal must be rejected")
	}
	if _, err := os.Stat(filepath.Join(root, "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("archive escaped the backpack, stat err = %v", err)
	}
}
