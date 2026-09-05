package system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDevelopmentCommandFindsUserYarnOutsideServicePATH(t *testing.T) {
	isolateUserNVM(t)
	cache := t.TempDir()
	previousCache := userCacheDir
	userCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { userCacheDir = previousCache })
	home := t.TempDir()
	previousHome := developmentUserHomeDir
	developmentUserHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { developmentUserHomeDir = previousHome })
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	yarn := filepath.Join(bin, "yarn")
	if err := os.WriteFile(yarn, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "")
	path, err := ResolveDevelopmentCommand("yarn")
	if err != nil || path != yarn {
		t.Fatalf("ResolveDevelopmentCommand(yarn) = %q, %v; want %q", path, err, yarn)
	}
	program, args, environment, notice := PrepareDevelopmentCommand("yarn", []string{"--cwd", "frontend", "build"})
	if program != yarn || len(args) != 3 || notice != "" {
		t.Fatalf("PrepareDevelopmentCommand = (%q, %#v, %q), want direct Yarn", program, args, notice)
	}
	if !strings.Contains(environmentValue(environment, "PATH"), bin) {
		t.Fatalf("development PATH does not include user yarn bin: %#v", environment)
	}
}

func TestPrepareDevelopmentCommandFallsBackToCorepack(t *testing.T) {
	isolateUserNVM(t)
	cache := t.TempDir()
	previousCache := userCacheDir
	userCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { userCacheDir = previousCache })
	home := t.TempDir()
	previousHome := developmentUserHomeDir
	developmentUserHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { developmentUserHomeDir = previousHome })
	previousSystemDirectories := developmentSystemCommandDirectories
	developmentSystemCommandDirectories = func(string) []string { return nil }
	t.Cleanup(func() { developmentSystemCommandDirectories = previousSystemDirectories })
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	corepack := filepath.Join(bin, "corepack")
	if err := os.WriteFile(corepack, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "")
	program, args, _, notice := PrepareDevelopmentCommand("yarn", []string{"build"})
	if program != corepack || strings.Join(args, " ") != "yarn build" || notice == "" {
		t.Fatalf("PrepareDevelopmentCommand fallback = (%q, %#v, %q)", program, args, notice)
	}
}

func TestDevelopmentCommandEnvironmentKeepsCurrentNodeFirst(t *testing.T) {
	current := t.TempDir()
	previousSystemDirectories := developmentSystemCommandDirectories
	developmentSystemCommandDirectories = func(string) []string { return []string{t.TempDir()} }
	t.Cleanup(func() { developmentSystemCommandDirectories = previousSystemDirectories })
	t.Setenv("PATH", current)
	if got := filepath.SplitList(environmentValue(DevelopmentCommandEnvironment(), "PATH")); len(got) == 0 || got[0] != current {
		t.Fatalf("development PATH = %#v, want current PATH first", got)
	}
}

func environmentValue(environment []string, key string) string {
	prefix := key + "="
	for _, value := range environment {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	return ""
}
