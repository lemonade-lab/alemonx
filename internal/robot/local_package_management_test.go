package robot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackpackPackageCanBeConfiguredAndRemovedByManifestName(t *testing.T) {
	root := t.TempDir()
	writeWebViewFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	packagePath := filepath.Join(root, "packages", "checkout-folder", "package.json")
	writeWebViewFixture(t, packagePath, `{
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

func TestSwitchLocalPackageVersionRequiresConfirmationBeforeDiscardingGitChanges(t *testing.T) {
	root := t.TempDir()
	writeWebViewFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	plugin := filepath.Join(root, "packages", "local-plugin")
	writeWebViewFixture(t, filepath.Join(plugin, "package.json"), `{"name":"local-plugin","version":"1.0.0"}`)
	for _, command := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
		{"add", "."},
		{"commit", "-m", "initial"},
		{"tag", "v1.0.0"},
	} {
		if _, err := gitRun(plugin, command...); err != nil {
			t.Skipf("git is unavailable for local package switch test: %v", err)
		}
	}
	writeWebViewFixture(t, filepath.Join(plugin, "package.json"), `{"name":"local-plugin","version":"1.0.0","local":true}`)
	writeWebViewFixture(t, filepath.Join(plugin, "scratch.txt"), "local change")

	if _, err := switchLocalPackageVersion(root, "local-plugin", "v1.0.0", false); err == nil || !strings.Contains(err.Error(), "强制切换") {
		t.Fatalf("ordinary switch should preserve local changes, err=%v", err)
	}
	if _, err := (Manager{}).Run(root, "force-switch-local-package-version", "", "local-plugin", "v1.0.0", "", "", false); err == nil || !strings.Contains(err.Error(), "确认") {
		t.Fatalf("force switch without confirmation should be rejected, err=%v", err)
	}
	result, err := (Manager{}).Run(root, "force-switch-local-package-version", "", "local-plugin", "v1.0.0", "", "", true)
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
}
