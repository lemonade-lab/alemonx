//go:build windows

package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestMain(m *testing.M) {
	// Act as a browser when the version check accidentally starts this binary.
	if marker := os.Getenv("ALX_TEST_BROWSER_VERSION_MARKER"); marker != "" {
		if err := os.WriteFile(marker, []byte("launched"), 0600); err != nil {
			os.Exit(1)
		}
		fmt.Println("Chrome 123.0.0.0")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestBrowserVersionReadsWindowsFileWithoutLaunching(t *testing.T) {
	directory, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatal(err)
	}
	version, err := browserVersion(context.Background(), filepath.Join(directory, "kernel32.dll"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+$`).MatchString(version) {
		t.Fatalf("version = %q", version)
	}
}

func TestBrowserVersionDoesNotExecuteFallbackWhenMetadataMissing(t *testing.T) {
	directory := t.TempDir()
	marker := filepath.Join(directory, "launched.txt")
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALX_TEST_BROWSER_VERSION_MARKER", marker)
	checker := &Checker{
		timeout:        time.Second,
		resolveBrowser: func() (string, string) { return path, "Chrome" },
	}
	check := checker.browser()
	if check.Status != "warning" || !check.Optional {
		t.Fatalf("browser check = %#v", check)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("browser executable must not be launched: %v", err)
	}
}

func TestBrowserVersionHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := browserVersion(ctx, "unused.exe"); err != context.Canceled {
		t.Fatalf("version error = %v", err)
	}
}
