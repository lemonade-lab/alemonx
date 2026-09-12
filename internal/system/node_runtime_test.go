package system

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"alemonx/internal/resources"
	"alemonx/internal/workspace"
)

func initEmbeddedNVMForTest(t *testing.T) {
	t.Helper()
	resources.Init(fstest.MapFS{
		"nvm/v0.40.7/nvm.sh":     {Data: []byte("# nvm")},
		"nvm/v0.40.7/nvm-exec":   {Data: []byte("#!/bin/sh")},
		"nvm/v0.40.7/LICENSE.md": {Data: []byte("MIT")},
	}, workspace.Layout{})
}

func isolateUserNVM(t *testing.T) {
	t.Helper()
	t.Setenv("NVM_DIR", "")
	previous := userHomeDir
	userHomeDir = func() (string, error) { return t.TempDir(), nil }
	t.Cleanup(func() { userHomeDir = previous })
}

func hideSystemNode(t *testing.T) {
	t.Helper()
	previous := nodeLookPath
	nodeLookPath = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { nodeLookPath = previous })
}

func TestNodeArchitecture(t *testing.T) {
	for architecture, want := range map[string]string{
		"amd64": "x64", "arm64": "arm64", "386": "x86", "arm": "armv7l",
		"ppc64le": "ppc64le", "s390x": "s390x", "riscv64": "riscv64",
	} {
		if got := nodeArchitecture(architecture); got != want {
			t.Fatalf("nodeArchitecture(%q) = %q, want %q", architecture, got, want)
		}
	}
	if got := nodeArchitecture("mips64"); got != "" {
		t.Fatalf("nodeArchitecture(mips64) = %q, want empty", got)
	}
}

func TestNVMDefaultInstallTargetsNode22(t *testing.T) {
	for _, forbidden := range []string{"nvm install --lts", "lts/*", "nvm alias default"} {
		if strings.Contains(nvmInstallNode22Script, forbidden) {
			t.Fatalf("default NVM script must not select the newest LTS: %q", nvmInstallNode22Script)
		}
	}
	for _, required := range []string{"nvm install 22", "nvm use 22"} {
		if !strings.Contains(nvmInstallNode22Script, required) {
			t.Fatalf("default NVM script lacks %q: %q", required, nvmInstallNode22Script)
		}
	}
}

func TestManagedNodeCommandResolvesBundledRuntime(t *testing.T) {
	isolateUserNVM(t)
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
	// The environment check now describes the actual node command. Make that
	// command explicit instead of expecting the check to rewrite PATH.
	t.Setenv("PATH", bin)
	report := NewChecker().CheckGoal("build", "npm")
	checked := map[string]bool{}
	for _, check := range report.Checks {
		if check.ID == "node" || check.ID == "npm" {
			checked[check.ID] = true
			if check.Status != "ready" {
				t.Fatalf("check %#v should be ready from the actual PATH", check)
			}
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

func TestResolveCommandNodeFollowsCurrentPath(t *testing.T) {
	isolateUserNVM(t)
	cache := t.TempDir()
	previousCache := userCacheDir
	userCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { userCacheDir = previousCache })

	bin := filepath.Join(cache, "alemonx", "environments", "nvm", nvmVersion, "versions", "node", "v24.0.0", "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	current := t.TempDir()
	for _, name := range []string{"node", "npm", "npx"} {
		path := filepath.Join(current, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\necho v22.22.3\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", current)
	for _, name := range []string{"node", "npm", "npx"} {
		want := filepath.Join(current, name)
		if got, err := ResolveCommand(name); err != nil || got != want {
			t.Fatalf("ResolveCommand(%q) = %q, %v; want current PATH command %q", name, got, err, want)
		}
	}
}

func TestResolveCommandDoesNotFallBackToHiddenManagedNode(t *testing.T) {
	isolateUserNVM(t)
	cache := t.TempDir()
	previousCache := userCacheDir
	userCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { userCacheDir = previousCache })
	bin := filepath.Join(cache, "alemonx", "environments", "node", "installed", "node-v22.22.3-linux-arm64", "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte("node"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := ResolveCommand("node"); err == nil {
		t.Fatal("ResolveCommand(node) must not select a hidden managed runtime")
	}
}

func TestNVMStatusDoesNotTreatManagedDefaultAsCurrentNode(t *testing.T) {
	isolateUserNVM(t)
	hideSystemNode(t)
	cache := t.TempDir()
	previousCache := userCacheDir
	userCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { userCacheDir = previousCache })
	directory := filepath.Join(cache, "alemonx", "environments", "nvm", nvmVersion)
	for _, version := range []string{"v22.22.3", "v24.0.0"} {
		bin := filepath.Join(directory, "versions", "node", version, "bin")
		if err := os.MkdirAll(bin, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bin, "node"), []byte("node"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(directory, "alias"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "alias", "default"), []byte("v22.22.3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status := NVMStatus()
	if !status.Available || status.ActiveVersion != "" {
		t.Fatalf("NVMStatus() = %#v, want no active version without node --version", status)
	}
	if got := NVMNodeBin(); got != filepath.Join(directory, "versions", "node", "v22.22.3", "bin") {
		t.Fatalf("NVMNodeBin() = %q, want default version bin", got)
	}
}

func TestActivateNVMDefaultForProcessMakesDefaultNodeCurrent(t *testing.T) {
	isolateUserNVM(t)
	cache := t.TempDir()
	previousCache := userCacheDir
	userCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { userCacheDir = previousCache })
	directory := filepath.Join(cache, "alemonx", "environments", "nvm", nvmVersion)
	bin := filepath.Join(directory, "versions", "node", "v22.22.3", "bin")
	if err := os.MkdirAll(filepath.Join(directory, "alias"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte("#!/bin/sh\necho v22.22.3\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "alias", "default"), []byte("v22.22.3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	got, err := ActivateNVMDefaultForProcess()
	if err != nil {
		t.Fatal(err)
	}
	if got != bin {
		t.Fatalf("ActivateNVMDefaultForProcess() = %q, want %q", got, bin)
	}
	if path, err := exec.LookPath("node"); err != nil || path != filepath.Join(bin, "node") {
		t.Fatalf("node after activation = %q, %v", path, err)
	}
}

func TestContainerListsPersistedNVMVersionsAlongsideCurrentNode(t *testing.T) {
	t.Setenv("ALX_CONTAINER", "1")
	userNVM := t.TempDir()
	t.Setenv("NVM_DIR", userNVM)
	if err := os.WriteFile(filepath.Join(userNVM, "nvm.sh"), []byte("# nvm"), 0o600); err != nil {
		t.Fatal(err)
	}
	nvmBin := filepath.Join(userNVM, "versions", "node", "v18.20.4", "bin")
	if err := os.MkdirAll(nvmBin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nvmBin, "node"), []byte("node"), 0o700); err != nil {
		t.Fatal(err)
	}
	imageNode := filepath.Join(t.TempDir(), "node")
	if err := os.WriteFile(imageNode, []byte("#!/bin/sh\necho v22.22.3\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	previousNodeLookup := nodeLookPath
	nodeLookPath = func(string) (string, error) { return imageNode, nil }
	t.Cleanup(func() { nodeLookPath = previousNodeLookup })

	status := NVMStatus()
	if status.Fixed || !status.Available || status.ActiveVersion != "v22.22.3" || len(status.Versions) != 1 || status.Versions[0] != "v18.20.4" {
		t.Fatalf("container NVMStatus() = %#v, want image Node plus persisted NVM versions", status)
	}
}

func TestContainerDoesNotRejectNodeRuntimeMutation(t *testing.T) {
	t.Setenv("ALX_CONTAINER", "1")
	if _, err := InstallNVMNodeVersion(context.Background(), "invalid"); err == nil || strings.Contains(err.Error(), "镜像固定") {
		t.Fatalf("InstallNVMNodeVersion container error = %v, want normal validation", err)
	}
	if _, err := UseNVMNodeVersion(context.Background(), "invalid"); err == nil || strings.Contains(err.Error(), "镜像固定") {
		t.Fatalf("UseNVMNodeVersion container error = %v, want normal validation", err)
	}
}

func TestNVMStatusUsesSystemNodeOverManagedDefault(t *testing.T) {
	isolateUserNVM(t)
	cache := t.TempDir()
	previousCache := userCacheDir
	userCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { userCacheDir = previousCache })
	directory := filepath.Join(cache, "alemonx", "environments", "nvm", nvmVersion)
	bin := filepath.Join(directory, "versions", "node", "v22.22.3", "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte("node"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, "alias"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "alias", "default"), []byte("v22.22.3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	systemNode := filepath.Join(t.TempDir(), "node")
	if err := os.WriteFile(systemNode, []byte("#!/bin/sh\necho v18.20.4\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	previousNodeLookup := nodeLookPath
	nodeLookPath = func(string) (string, error) { return systemNode, nil }
	t.Cleanup(func() { nodeLookPath = previousNodeLookup })
	if status := NVMStatus(); status.ActiveVersion != "v18.20.4" {
		t.Fatalf("NVMStatus() = %#v, want actual system node v18.20.4", status)
	}
}

func TestNVMStatusReturnsEmptyVersionsInsteadOfNil(t *testing.T) {
	isolateUserNVM(t)
	cache := t.TempDir()
	previousCache := userCacheDir
	userCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { userCacheDir = previousCache })
	if versions := NVMStatus().Versions; versions == nil || len(versions) != 0 {
		t.Fatalf("NVMStatus().Versions = %#v, want non-nil empty slice", versions)
	}
}

func TestApplyNodeRuntimeMakesNodeVersionAndChildEnvironmentAgree(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "v18", "bin")
	second := filepath.Join(root, "v22", "bin")
	for bin, version := range map[string]string{first: "v18.20.4", second: "v22.22.3"} {
		if err := os.MkdirAll(bin, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bin, "node"), []byte("#!/bin/sh\necho "+version+"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", first+string(os.PathListSeparator)+os.Getenv("PATH"))
	runtime, err := CurrentNodeRuntime()
	if err != nil || runtime.Version != "v18.20.4" || runtime.Bin != first {
		t.Fatalf("initial runtime = %#v, %v", runtime, err)
	}
	runtime, err = ApplyNodeRuntime(second)
	if err != nil || runtime.Version != "v22.22.3" || runtime.Bin != second {
		t.Fatalf("applied runtime = %#v, %v", runtime, err)
	}
	if got := environmentPath(runtime.Environment); filepath.SplitList(got)[0] != second {
		t.Fatalf("child PATH = %q, want %q first", got, second)
	}
}

func TestNVMStatusShowsSystemNodeWithoutManagedVersions(t *testing.T) {
	isolateUserNVM(t)
	cache := t.TempDir()
	previousCache := userCacheDir
	userCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { userCacheDir = previousCache })
	bin := t.TempDir()
	node := filepath.Join(bin, "node")
	if err := os.WriteFile(node, []byte("#!/bin/sh\necho v20.18.1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	status := NVMStatus()
	if status.Available || status.ActiveVersion != "v20.18.1" || len(status.Versions) != 0 {
		t.Fatalf("NVMStatus() = %#v, want system node v20.18.1 without managed versions", status)
	}
}

func TestNVMNodeBinDoesNotChooseUserRuntimeWithoutDefault(t *testing.T) {
	cache := t.TempDir()
	previousCache := userCacheDir
	userCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { userCacheDir = previousCache })
	userNVM := t.TempDir()
	t.Setenv("NVM_DIR", userNVM)
	if err := os.WriteFile(filepath.Join(userNVM, "nvm.sh"), []byte("# nvm"), 0o600); err != nil {
		t.Fatal(err)
	}
	userBin := filepath.Join(userNVM, "versions", "node", "v24.0.0", "bin")
	if err := os.MkdirAll(userBin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userBin, "node"), []byte("node"), 0o700); err != nil {
		t.Fatal(err)
	}
	managedBin := filepath.Join(cache, "alemonx", "environments", "nvm", nvmVersion, "versions", "node", "v22.22.3", "bin")
	if err := os.MkdirAll(managedBin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(managedBin, "node"), []byte("node"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := NVMNodeBin(); got != "" {
		t.Fatalf("NVMNodeBin() = %q, want no implicit user runtime", got)
	}
}

func TestNVMStatusDoesNotUseMajorDefaultAliasAsCurrentNode(t *testing.T) {
	isolateUserNVM(t)
	hideSystemNode(t)
	cache := t.TempDir()
	previousCache := userCacheDir
	userCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { userCacheDir = previousCache })
	directory := filepath.Join(cache, "alemonx", "environments", "nvm", nvmVersion)
	for _, version := range []string{"v22.22.3", "v26.0.0"} {
		bin := filepath.Join(directory, "versions", "node", version, "bin")
		if err := os.MkdirAll(bin, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bin, "node"), []byte("node"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(directory, "alias"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "alias", "default"), []byte("22\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if status := NVMStatus(); status.ActiveVersion != "" {
		t.Fatalf("NVMStatus() = %#v, want no active version without node --version", status)
	}
}

func TestPrependCommandPathRemovesPreviousNVMRuntime(t *testing.T) {
	oldBin := "/tmp/nvm/versions/node/v26.0.0/bin"
	newBin := "/tmp/nvm/versions/node/v22.22.3/bin"
	t.Setenv("PATH", oldBin+string(os.PathListSeparator)+"/usr/bin")
	prependCommandPath(newBin)
	if got := os.Getenv("PATH"); strings.Contains(got, oldBin) || !strings.HasPrefix(got, newBin+string(os.PathListSeparator)) {
		t.Fatalf("PATH = %q, want only selected NVM runtime", got)
	}
}

func TestEnsureNVMMaterializesEmbeddedBundleWithoutGit(t *testing.T) {
	isolateUserNVM(t)
	cache := t.TempDir()
	previousCache := userCacheDir
	userCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { userCacheDir = previousCache })
	initEmbeddedNVMForTest(t)
	directory, created, err := ensureNVM()
	if err != nil || !created {
		t.Fatalf("ensureNVM = %q, %t, %v", directory, created, err)
	}
	if _, err := os.Stat(filepath.Join(directory, "nvm.sh")); err != nil {
		t.Fatalf("embedded nvm.sh: %v", err)
	}
}
