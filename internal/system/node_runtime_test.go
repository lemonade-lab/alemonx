package system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedNodeCommandResolvesBundledRuntime(t *testing.T) {
	cache := t.TempDir()
	previous := userCacheDir
	userCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { userCacheDir = previous })

	bin := filepath.Join(cache, "alemonx", "environments", "node", "installed", "node-v24.0.0-linux-x64", "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"node", "npm", "npx"} {
		path := filepath.Join(bin, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\necho v24.0.0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		if got := ManagedNodeCommand(name); got != path {
			t.Fatalf("ManagedNodeCommand(%q) = %q, want %q", name, got, path)
		}
	}
	if got := ManagedNodeCommand("yarn"); got != "" {
		t.Fatalf("ManagedNodeCommand(yarn) = %q, want empty", got)
	}
	t.Setenv("PATH", "")
	report := NewChecker().CheckGoal("build", "npm")
	checked := map[string]bool{}
	for _, check := range report.Checks {
		if check.ID == "node" || check.ID == "npm" {
			checked[check.ID] = true
			if check.Status != "ready" {
				t.Fatalf("check %#v should be ready after repairing PATH", check)
			}
		}
		if check.ID == "node" && !strings.Contains(check.Detail, "已自动修复当前服务 PATH") {
			t.Fatalf("node check should report PATH repair: %#v", check)
		}
	}
	if !checked["node"] || !checked["npm"] {
		t.Fatalf("expected node and npm checks, got %#v", report.Checks)
	}
	for _, directory := range filepath.SplitList(os.Getenv("PATH")) {
		if directory == bin {
			return
		}
	}
	t.Fatalf("PATH = %q, want managed Node bin %q", os.Getenv("PATH"), bin)
}
