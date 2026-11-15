package robot

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"alemonx/internal/packagemanager"
	"alemonx/internal/pm2config"
	"alemonx/internal/system"
)

func TestRuntimeSwitchRoundTrip(t *testing.T) {
	if _, err := system.ResolveCommand("bun"); err != nil {
		t.Skip("Bun is unavailable")
	}
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm is unavailable")
	}
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{
		"name":"runtime-switch-fixture","version":"1.0.0","packageManager":"npm@10.9.9",
		"dependencies":{"fixture-dependency":"file:vendor/dependency"},
		"scripts":{"app":"node index.js","custom":"echo unchanged"}
	}`)
	writeAppPageFixture(t, filepath.Join(root, "vendor/dependency/package.json"), `{"name":"fixture-dependency","version":"1.0.0"}`)
	writeAppPageFixture(t, filepath.Join(root, "other/node_modules/retained"), "unrelated")
	settings := pm2config.DefaultOptions(root)
	settings.Env["APP_MODE"] = "fixture"
	settings.MaxRestarts = 4
	writeAppPageFixture(t, filepath.Join(root, "pm2.config.cjs"), pm2config.ConfigWithOptions(root, settings))
	manager := Manager{}
	for _, target := range []string{"bun", "node"} {
		plan, err := manager.RuntimeSwitchPlan(context.Background(), root, target)
		if err != nil || len(plan.Blocked) != 0 {
			t.Fatalf("preflight %s: %+v, %v", target, plan, err)
		}
		if _, err := manager.ApplyRuntimeSwitch(context.Background(), root, target, plan.Revision); err != nil {
			t.Fatalf("switch %s: %v", target, err)
		}
		data, err := os.ReadFile(filepath.Join(root, "package.json"))
		if err != nil || packagemanager.ProjectRuntime(root) != target ||
			!strings.Contains(string(data), `"app": "`+target+` index.js"`) ||
			!strings.Contains(string(data), `"custom": "echo unchanged"`) {
			t.Fatalf("unexpected manifest after switching to %s: %s, %v", target, data, err)
		}
		pm2, err := manager.PM2Registration(root)
		if err != nil || pm2.MaxRestarts != 4 || pm2.Env["APP_MODE"] != "fixture" {
			t.Fatalf("lost PM2 options: %+v, %v", pm2, err)
		}
		if _, err := os.Stat(filepath.Join(root, "other/node_modules/retained")); err != nil {
			t.Fatal("unrelated nested installation was modified", err)
		}
	}
	if version := packagemanager.DeclaredVersion(root, "npm"); version != "10.9.9" {
		t.Fatalf("original npm version was not restored: %q", version)
	}
	if _, err := os.Stat(filepath.Join(root, "bunfig.toml")); !os.IsNotExist(err) {
		t.Fatalf("generated Bun config remained after switching back: %v", err)
	}
}

func TestRuntimeSwitchRestoresOriginalOnInstallFailure(t *testing.T) {
	if _, err := system.ResolveCommand("bun"); err != nil {
		t.Skip("Bun is unavailable")
	}
	root := t.TempDir()
	const manifest = `{"name":"runtime-switch-failure","packageManager":"npm@10","dependencies":{"missing":"file:./not-present"}}`
	writeAppPageFixture(t, filepath.Join(root, "package.json"), manifest)
	writeAppPageFixture(t, filepath.Join(root, "node_modules/original"), "preserve")
	manager := Manager{}
	plan, err := manager.RuntimeSwitchPlan(context.Background(), root, "bun")
	if err != nil || len(plan.Blocked) != 0 {
		t.Fatalf("preflight: %+v, %v", plan, err)
	}
	if _, err := manager.ApplyRuntimeSwitch(context.Background(), root, "bun", "stale-preview"); err == nil {
		t.Fatal("accepted stale preview")
	}
	if _, err := manager.ApplyRuntimeSwitch(context.Background(), root, "bun", plan.Revision); err == nil {
		t.Fatal("installation unexpectedly succeeded")
	}
	for name, expected := range map[string]string{"package.json": manifest, "node_modules/original": "preserve"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(data) != expected {
			t.Errorf("%s was not restored: %s, %v", name, data, err)
		}
	}
}
