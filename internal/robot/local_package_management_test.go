package robot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackpackWorkspaceSwitchAddsAndRemovesOnlyBackpackPattern(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{
  "name": "robot",
  "workspaces": {"packages": ["modules/*"], "nohoist": ["legacy"]}
}`)
	manager := Manager{}
	if _, err := manager.Run(root, "enable-backpack-workspace", "", "", "", "", "", false); err != nil {
		t.Fatalf("enable backpack workspace: %v", err)
	}
	manifest := readWorkspaceManifest(t, root)
	if manifest["private"] != true {
		t.Fatalf("private = %#v, want true", manifest["private"])
	}
	workspaces := manifest["workspaces"].(map[string]any)
	if !workspaceContains(workspaces["packages"].([]any), "packages/*") || !workspaceContains(workspaces["packages"].([]any), "modules/*") {
		t.Fatalf("workspaces = %#v", workspaces)
	}
	if _, err := manager.Run(root, "disable-backpack-workspace", "", "", "", "", "", false); err != nil {
		t.Fatalf("disable backpack workspace: %v", err)
	}
	manifest = readWorkspaceManifest(t, root)
	if manifest["private"] != false {
		t.Fatalf("private = %#v, want false", manifest["private"])
	}
	workspaces = manifest["workspaces"].(map[string]any)
	if workspaceContains(workspaces["packages"].([]any), "packages/*") || !workspaceContains(workspaces["packages"].([]any), "modules/*") {
		t.Fatalf("workspaces = %#v", workspaces)
	}
}

func TestBackpackWorkspaceSwitchRemovesEmptyWorkspace(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot","private":true,"workspaces":["packages/*"]}`)
	if _, err := (Manager{}).Run(root, "disable-backpack-workspace", "", "", "", "", "", false); err != nil {
		t.Fatalf("disable backpack workspace: %v", err)
	}
	manifest := readWorkspaceManifest(t, root)
	if _, exists := manifest["workspaces"]; exists {
		t.Fatalf("workspaces should be removed: %#v", manifest)
	}
}

func readWorkspaceManifest(t *testing.T, root string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestBackpackPackageCanBeConfiguredAndRemovedByManifestName(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	packagePath := filepath.Join(root, "packages", "checkout-folder", "package.json")
	writeAppPageFixture(t, packagePath, `{
  "name":"local-plugin",
  "alemonjs":{"config":[{"name":"token","type":"text","required":true,"description":"令牌"}]}
}`)

	config, err := (Manager{}).PackageConfig(root, "local-plugin")
	if err != nil {
		t.Fatalf("PackageConfig: %v", err)
	}
	if config.Namespace != "local-plugin" || len(config.Fields) != 1 {
		t.Fatalf("unexpected config: %#v", config)
	}
	result, err := removeLocalPackageByName(root, "local-plugin")
	if err != nil {
		t.Fatalf("removeLocalPackageByName: %v", err)
	}
	if result.Path != filepath.Join(root, "packages", "checkout-folder") {
		t.Fatalf("removed path = %q", result.Path)
	}
	if _, err := os.Stat(filepath.Dir(packagePath)); !os.IsNotExist(err) {
		t.Fatalf("package directory should be removed, stat error = %v", err)
	}
}

func TestBackpackRemovesLeftoverDirectoryWithoutValidManifest(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	// A failed install can leave an empty folder (or one with a broken
	// package.json). The backpack lists it by directory name, so removal must
	// work for that identifier even though it cannot run as a plugin.
	leftover := filepath.Join(root, "packages", "broken-plugin")
	if err := os.MkdirAll(leftover, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := removeLocalPackageByName(root, "broken-plugin"); err != nil {
		t.Fatalf("removeLocalPackageByName(leftover): %v", err)
	}
	if _, err := os.Stat(leftover); !os.IsNotExist(err) {
		t.Fatalf("leftover directory should be removed, stat error = %v", err)
	}

	// A directory whose package.json is corrupt is also removable.
	corrupt := filepath.Join(root, "packages", "corrupt-plugin")
	if err := os.MkdirAll(corrupt, 0755); err != nil {
		t.Fatal(err)
	}
	writeAppPageFixture(t, filepath.Join(corrupt, "package.json"), `{not-json`)
	if _, err := removeLocalPackageByName(root, "corrupt-plugin"); err != nil {
		t.Fatalf("removeLocalPackageByName(corrupt): %v", err)
	}
	if _, err := os.Stat(corrupt); !os.IsNotExist(err) {
		t.Fatalf("corrupt directory should be removed, stat error = %v", err)
	}

	// An invalid directory must not be removable through a mismatched name.
	kept := filepath.Join(root, "packages", "keep-me")
	if err := os.MkdirAll(kept, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := removeLocalPackageByName(root, "other-name"); err == nil {
		t.Fatal("removing a mismatched directory must fail")
	}
	if _, err := os.Stat(kept); err != nil {
		t.Fatalf("mismatched directory must be kept, stat error = %v", err)
	}
}

func TestSwitchLocalPackageVersionRequiresConfirmationBeforeDiscardingGitChanges(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	plugin := filepath.Join(root, "packages", "local-plugin")
	writeAppPageFixture(t, filepath.Join(plugin, "package.json"), `{"name":"local-plugin","version":"1.0.0"}`)
	for _, command := range [][]string{
		{"init", "-b", "release"},
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
		{"add", "."},
		{"commit", "-m", "initial"},
	} {
		if _, err := gitRun(plugin, command...); err != nil {
			t.Skipf("git is unavailable for local package switch test: %v", err)
		}
	}
	remote := filepath.Join(t.TempDir(), "plugin.git")
	for _, command := range [][]string{
		{"init", "--bare", remote},
		{"remote", "add", "origin", remote},
		{"push", "-u", "origin", "release"},
	} {
		if _, err := gitRun(plugin, command...); err != nil {
			t.Skipf("git remote setup is unavailable for local package switch test: %v", err)
		}
	}
	writeAppPageFixture(t, filepath.Join(plugin, "package.json"), `{"name":"local-plugin","version":"1.0.0","local":true}`)
	writeAppPageFixture(t, filepath.Join(plugin, "scratch.txt"), "local change")

	if _, err := switchLocalPackageVersion(root, "local-plugin", "release", false); err == nil || !strings.Contains(err.Error(), "存在未提交修改") {
		t.Fatalf("ordinary switch should preserve local changes, err=%v", err)
	}
	if _, err := (Manager{}).Run(root, "force-sync-local-package-release", "", "local-plugin", "release", "", "", false); err == nil || !strings.Contains(err.Error(), "确认") {
		t.Fatalf("force switch without confirmation should be rejected, err=%v", err)
	}
	result, err := (Manager{}).Run(root, "force-sync-local-package-release", "", "local-plugin", "release", "", "", true)
	if err != nil {
		t.Fatalf("force switch: %v\n%s", err, result.Output)
	}
	data, err := os.ReadFile(filepath.Join(plugin, "package.json"))
	if err != nil || strings.Contains(string(data), `"local":true`) {
		t.Fatalf("tracked modification was not discarded: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(plugin, "scratch.txt")); !os.IsNotExist(err) {
		t.Fatalf("untracked file should be removed, stat err=%v", err)
	}
	if status, err := gitRun(plugin, "status", "--porcelain"); err != nil || strings.TrimSpace(status) != "" {
		t.Fatalf("plugin should be clean after force switch: %q, %v", status, err)
	}
	writeAppPageFixture(t, filepath.Join(plugin, "package.json"), `{"name":"local-plugin","version":"1.0.1"}`)
	if _, err := gitRun(plugin, "add", "."); err != nil {
		t.Fatal(err)
	}
	if _, err := gitRun(plugin, "commit", "-m", "local release commit"); err != nil {
		t.Fatal(err)
	}
	if _, err := switchLocalPackageVersion(root, "local-plugin", "release", false); err == nil || !strings.Contains(err.Error(), "领先远程") {
		t.Fatalf("ordinary sync must preserve a clean local commit, err=%v", err)
	}
	if _, err := (Manager{}).SetAppEnabled(root, "local-plugin", true); err == nil || !strings.Contains(err.Error(), "领先远程") {
		t.Fatalf("a release branch ahead of origin must not be enabled, err=%v", err)
	}
	writeAppPageFixture(t, filepath.Join(root, "alemon.config.yaml"), "apps:\n  local-plugin: true\n")
	if err := (Manager{}).ValidateEnabledBackpackRelease(root); err == nil || !strings.Contains(err.Error(), "领先远程") {
		t.Fatalf("runtime release preflight must reject a branch ahead of origin, err=%v", err)
	}
}
