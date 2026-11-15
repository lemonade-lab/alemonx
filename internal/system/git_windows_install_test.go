package system

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGitForWindowsAsset(t *testing.T) {
	for arch, expected := range map[string]string{
		"amd64":   "MinGit-2.56.1-64-bit.zip",
		"arm64":   "MinGit-2.56.1-arm64.zip",
		"386":     "MinGit-2.56.1-32-bit.zip",
		"riscv64": "",
	} {
		if got := gitForWindowsAsset("v2.56.1.windows.1", arch); got != expected {
			t.Errorf("%s: got %q, want %q", arch, got, expected)
		}
	}
	if got := gitForWindowsAsset("v2.56.1-rc0", "amd64"); got != "" {
		t.Errorf("accepted unstable tag: %q", got)
	}
}

func TestExtractMinGit(t *testing.T) {
	for _, name := range []string{"cmd/git.exe", "../git.exe", "cmd/.. /git.exe", "cmd/git.exe:stream"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			archive := filepath.Join(root, "git.zip")
			file, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			writer := zip.NewWriter(file)
			entry, err := writer.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := entry.Write([]byte("fixture")); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(root, "unpacked")
			err = extractMinGit(context.Background(), archive, output)
			if name != "cmd/git.exe" {
				if err == nil {
					t.Fatal("unsafe archive entry was accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(output, "cmd", "git.exe"))
			if err != nil || string(data) != "fixture" {
				t.Fatalf("extracted file = %q, %v", data, err)
			}
		})
	}
}
