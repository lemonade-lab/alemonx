package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDesktopReleaseSelection(t *testing.T) {
	for _, platform := range []struct{ os, arch, asset string }{
		{"darwin", "arm64", "alemonx-desktop-macos_arm64.zip"},
		{"windows", "amd64", "alemonx-desktop-windows_amd64.zip"},
		{"linux", "amd64", "alemonx-desktop-linux_amd64.tar.gz"},
		{"linux", "arm64", "alemonx-desktop-linux_arm64.tar.gz"},
		{"darwin", "amd64", ""},
	} {
		if got := desktopAssetName(platform.os, platform.arch); got != platform.asset {
			t.Errorf("%s/%s matched %q", platform.os, platform.arch, got)
		}
	}
	for _, versions := range []struct {
		latest, current string
		newer           bool
	}{
		{"v0.10.0", "v0.9.8", true},
		{"v1.0.0", "v0.99.99", true},
		{"v0.3.0", "v0.3.0", false},
		{"v0.3.0", "v0.4.0", false},
		{"v0.4.0", "dev", false},
	} {
		if got := newerDesktopRelease(versions.latest, versions.current); got != versions.newer {
			t.Errorf("newerDesktopRelease(%s, %s) = %t", versions.latest, versions.current, got)
		}
	}
}

func TestDesktopArchiveChecksumRequiresExactAsset(t *testing.T) {
	want := strings.Repeat("c", 64)
	sums := strings.Repeat("a", 64) + "  alemonx-desktop-windows_amd64-installer.exe\n" + want + "  alemonx-desktop-windows_amd64.zip\n"
	if got, err := desktopArchiveChecksum(strings.NewReader(sums), "alemonx-desktop-windows_amd64.zip"); err != nil || got != want {
		t.Fatalf("checksum = %q, %v", got, err)
	}
	if _, err := desktopArchiveChecksum(strings.NewReader(sums), "alx-windows-amd64.zip"); err == nil {
		t.Fatal("matched a Web/CLI package")
	}
}

func TestDesktopInstallRestoresFilesOnPartialFailure(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	launcher, backend := filepath.Join(root, "launcher"), filepath.Join(root, "backend")
	for path, data := range map[string]string{launcher: "old launcher", backend: "old backend", filepath.Join(stage, "launcher"): "new launcher"} {
		if err := os.WriteFile(path, []byte(data), 0700); err != nil {
			t.Fatal(err)
		}
	}
	err := applyDesktopInstall(desktopInstallPlan{Stage: stage, Files: []string{"launcher", "backend"}, Destinations: []string{launcher, backend}})
	if err == nil {
		t.Fatal("expected the missing backend to fail installation")
	}
	for path, want := range map[string]string{launcher: "old launcher", backend: "old backend"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("%s contains %q, %v", path, got, err)
		}
	}
}

func TestMacDesktopArchiveRejectsTraversal(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS bundle layout")
	}
	dir := t.TempDir()
	archive := filepath.Join(dir, "update.zip")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("ALemonX Desktop.app/../unexpected")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("unexpected")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extractDesktopUpdate(archive, dir); err == nil {
		t.Fatal("accepted path outside the app bundle")
	}
}
