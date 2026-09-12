package system

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func runtimeFixture(t *testing.T, bin, name, version string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\necho '"+version+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
}

func TestSelectedNodeSurvivesGitCheckAndToolRefresh(t *testing.T) {
	root := t.TempDir()
	oldBin, newBin := filepath.Join(root, "system"), filepath.Join(root, "versions", "node", "v24.0.0", "bin")
	runtimeFixture(t, oldBin, "node", "v10.24.1")
	runtimeFixture(t, oldBin, "git", "git version 2.40.0")
	runtimeFixture(t, newBin, "node", "v24.0.0")
	t.Setenv("PATH", oldBin)
	if _, err := ApplyNodeRuntime(newBin); err != nil {
		t.Fatal(err)
	}
	NewChecker().command("git", "Git", "--version", "")
	RefreshCommandEnvironment("git")
	for i := 0; i < 3; i++ {
		got, err := CurrentNodeRuntime()
		if err != nil || got.Version != "v24.0.0" {
			t.Fatalf("runtime reverted: %#v, %v", got, err)
		}
		NewChecker().command("git", "Git", "--version", "")
	}
}

func TestRuntimeSelectionsSurviveRestartAndPythonKeepsNode(t *testing.T) {
	home := t.TempDir()
	previousHome := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHomeDir = previousHome })
	oldBin := filepath.Join(home, "system")
	nodeBin := filepath.Join(home, "versions", "node", "v24.0.0", "bin")
	pythonBin := filepath.Join(home, "python", "bin")
	runtimeFixture(t, oldBin, "node", "v10.24.1")
	runtimeFixture(t, oldBin, "python3", "Python 3.9.0")
	runtimeFixture(t, nodeBin, "node", "v24.0.0")
	runtimeFixture(t, pythonBin, "python3", "Python 3.12.10")
	t.Setenv("PATH", oldBin)
	if err := selectRuntime("node", nodeBin); err != nil {
		t.Fatal(err)
	}
	if err := selectRuntime("python", pythonBin); err != nil {
		t.Fatal(err)
	}
	assertRuntime := func() {
		t.Helper()
		node, err := CurrentNodeRuntime()
		if err != nil || node.Version != "v24.0.0" {
			t.Fatalf("Node = %#v, %v", node, err)
		}
		if got := pythonCommandVersion(); got != "3.12.10" {
			t.Fatalf("Python = %s", got)
		}
	}
	assertRuntime()
	// Model a new service process with only its original system PATH.
	_ = os.Setenv("PATH", oldBin)
	if _, err := ActivateNVMDefaultForProcess(); err != nil {
		t.Fatal(err)
	}
	if err := RestorePythonRuntime(); err != nil {
		t.Fatal(err)
	}
	assertRuntime()
}

func TestBrokenPythonSelectionLeavesEnvironmentUnchanged(t *testing.T) {
	home := t.TempDir()
	previousHome := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHomeDir = previousHome })
	t.Setenv("PATH", "/usr/bin:/bin")
	before := os.Getenv("PATH")
	if err := selectRuntime("python", filepath.Join(home, "missing")); err == nil {
		t.Fatal("expected failure")
	}
	if os.Getenv("PATH") != before {
		t.Fatal("failed selection changed PATH")
	}
	selection, err := readRuntimeSelection()
	if err != nil || selection.Python != "" {
		t.Fatalf("failed selection persisted: %#v, %v", selection, err)
	}
}

func TestPythonSwitchDoesNotChangeUserDefault(t *testing.T) {
	home := t.TempDir()
	previousHome := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHomeDir = previousHome })
	root := filepath.Join(home, ".pyenv")
	t.Setenv("PYENV_ROOT", root)
	t.Setenv("ALX_CONTAINER", "")
	t.Setenv("ALEMONJS_SETUP_ROOTS", "")
	runtimeFixture(t, filepath.Join(root, "bin"), "pyenv", "unexpected invocation")
	runtimeFixture(t, filepath.Join(root, "versions", "3.12.10", "bin"), "python3", "Python 3.12.10")
	defaultFile := filepath.Join(root, "version")
	if err := os.WriteFile(defaultFile, []byte("3.9.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "/usr/bin:/bin")
	if _, err := UsePythonVersion(context.Background(), "3.12.10"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(defaultFile)
	if err != nil || string(data) != "3.9.0\n" {
		t.Fatalf("user default changed: %q, %v", data, err)
	}
	if got := pythonCommandVersion(); got != "3.12.10" {
		t.Fatalf("Python = %s", got)
	}
}
