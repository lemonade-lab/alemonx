package setupplugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMoveIntoCacheRenamesOnSameFilesystem(t *testing.T) {
	source := filepath.Join(t.TempDir(), "alemonx-download.zip")
	content := []byte("plugin archive bytes")
	if err := os.WriteFile(source, content, 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "cache", "alemonx", "package.zip")
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := moveIntoCache(source, destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(string(data), string(content)) {
		t.Fatalf("cache content mismatch: %q", data)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source should be consumed by rename, stat err = %v", err)
	}
}

func TestCopyIntoDirectoryStagesInsideDestination(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.zip")
	content := []byte("cross-device fallback bytes")
	if err := os.WriteFile(source, content, 0o600); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := copyIntoDirectory(source, directory, "package.zip"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "package.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(content) {
		t.Fatalf("copied content mismatch: %q", data)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".package-") {
			t.Fatalf("temporary staging file left behind: %s", entry.Name())
		}
	}
}

func TestMoveIntoCacheFallbackRemovesTempFiles(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.zip")
	if err := os.WriteFile(source, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	// Destination already exists as a directory, so the direct rename fails and
	// the fallback copy must surface an error without leaking temp files.
	if err := os.MkdirAll(filepath.Join(directory, "package.zip"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := moveIntoCache(source, filepath.Join(directory, "package.zip")); err == nil {
		t.Fatal("expected an error when destination is an existing directory")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".package-") {
			t.Fatalf("temporary staging file leaked: %s", entry.Name())
		}
	}
}
