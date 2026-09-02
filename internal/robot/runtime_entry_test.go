package robot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRepairRuntimeUsesIndexJSEntry verifies that repairing a project produces
// scripts and PM2 config that launch the robot's index.js, never the package
// main field (which points at the lib/ build artifact).
func TestRepairRuntimeUsesIndexJSEntry(t *testing.T) {
	root := t.TempDir()
	// main points at the build output; repair must still target index.js.
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"bot","main":"lib/index.js","scripts":{}}`)
	if _, err := (Manager{}).RepairRuntime(root, "dev"); err != nil {
		t.Fatalf("RepairRuntime(dev): %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"app": "node index.js"`, `"dev": "node index.js"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("script missing %q:\n%s", want, data)
		}
	}
}

// TestRepairRuntimePM2UsesIndexJSEntry ensures the PM2 repair config points at
// index.js and never rewrites the build artifact under lib/.
func TestRepairRuntimePM2UsesIndexJSEntry(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"bot","main":"lib/index.js","scripts":{}}`)
	if _, err := (Manager{}).RepairRuntime(root, "pm2"); err != nil {
		t.Fatalf("RepairRuntime(pm2): %v", err)
	}
	config, err := os.ReadFile(filepath.Join(root, "pm2.config.cjs"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), `script: './index.js'`) {
		t.Fatalf("PM2 config should run index.js:\n%s", config)
	}
	if strings.Contains(string(config), "lib/index.js") {
		t.Fatalf("PM2 config must not reference the build artifact:\n%s", config)
	}
	// The robot's startup script is created at the top level, not lib/.
	if _, err := os.Stat(filepath.Join(root, "index.js")); err != nil {
		t.Fatalf("index.js should exist after repair: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "lib", "index.js")); !os.IsNotExist(err) {
		t.Fatalf("lib/index.js should not be created by repair")
	}
}

func TestRuntimeRepairUpgradesLegacyPM2ConfigWithoutTreatingItAsCustom(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"bot","dependencies":{"alemonjs":"^2"},"scripts":{}}`)
	if err := os.WriteFile(filepath.Join(root, "app.js"), []byte("export default {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pm2.config.cjs"), []byte(legacyTemplatePM2Config), 0644); err != nil {
		t.Fatal(err)
	}
	plan, err := (Manager{}).RuntimeRepairPlan(root, "pm2")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.RequiresConfirmation) != 0 {
		t.Fatalf("legacy generated config should not require confirmation: %#v", plan)
	}
	if !strings.Contains(strings.Join(plan.Automatic, "\n"), "隔离同名项目进程") {
		t.Fatalf("legacy PM2 config should be marked for upgrade: %#v", plan)
	}
}

func TestRuntimeRepairPreservesCustomPM2ConfigUntilConfirmed(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"bot","dependencies":{"alemonjs":"^2"},"scripts":{}}`)
	if err := os.WriteFile(filepath.Join(root, "app.js"), []byte("export default {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pm2.config.cjs"), []byte("module.exports = { apps: [{ name: 'my-custom-bot', script: './server.js' }] };\n"), 0644); err != nil {
		t.Fatal(err)
	}
	plan, err := (Manager{}).RuntimeRepairPlan(root, "pm2")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.RequiresConfirmation) != 1 {
		t.Fatalf("custom PM2 config should require confirmation: %#v", plan)
	}
}

// TestParsePM2ProcessesMapsJListFields verifies the pm2 jlist payload maps to
// the table fields the UI renders.
func TestParsePM2ProcessesMapsJListFields(t *testing.T) {
	output := `[
  {
    "pid": 9896,
    "name": "alemonb",
    "pm_id": 0,
    "restart_time": 2,
    "pm2_env": {"script": "./index.js", "namespace": "default", "status": "online", "pm_uptime": 1751900000000},
    "monit": {"memory": 123456789, "cpu": 0.5}
  }
]`
	items, err := parsePM2Processes(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d processes, want 1", len(items))
	}
	p := items[0]
	if p.Name != "alemonb" || p.ID != 0 || p.Status != "online" || p.PID != 9896 {
		t.Fatalf("process = %#v", p)
	}
	if p.Memory != 123456789 || p.Restarts != 2 || p.Script != "./index.js" || p.Namespace != "default" {
		t.Fatalf("process fields = %#v", p)
	}
	if p.CPU != 0.5 || p.Uptime != 1751900000000 {
		t.Fatalf("process monit fields = %#v", p)
	}
}

// TestAppPortReadsAndSavesServerPort covers the "应用" flow: reading the
// configured port from alemon.config.yaml, the default fallback, and writing a
// new port (replacing an existing serverPort or appending one).
func TestAppPortReadsAndSavesServerPort(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"bot"}`)
	// No config file yet: default port, not configured.
	info, err := (Manager{}).AppPort(root)
	if err != nil || info.Port != defaultAppPort || info.Configured {
		t.Fatalf("default port = %+v, %v", info, err)
	}
	// Save a port then read it back.
	if _, err := (Manager{}).SaveAppPort(root, 19191); err != nil {
		t.Fatalf("SaveAppPort: %v", err)
	}
	info, _ = (Manager{}).AppPort(root)
	if info.Port != 19191 || !info.Configured {
		t.Fatalf("port after save = %+v, want 19191 configured", info)
	}
	// Replace an existing serverPort.
	if _, err := (Manager{}).SaveAppPort(root, 20000); err != nil {
		t.Fatalf("SaveAppPort replace: %v", err)
	}
	info, _ = (Manager{}).AppPort(root)
	if info.Port != 20000 || !info.Configured {
		t.Fatalf("port after replace = %+v, want 20000 configured", info)
	}
	// Invalid port is rejected.
	if _, err := (Manager{}).SaveAppPort(root, 70000); err == nil {
		t.Fatal("invalid port should be rejected")
	}
}

// TestSetAppEnabledTogglesLocalPackageInApps covers the backpack 启动/停用
// flow: adding and removing a local package's npm name in alemon.config.yaml.
func TestSetAppEnabledTogglesLocalPackageInApps(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"bot"}`)
	// Initially no apps.
	apps, err := (Manager{}).EnabledApps(root)
	if err != nil || len(apps) != 0 {
		t.Fatalf("initial apps = %v, %v", apps, err)
	}
	// Enable a package.
	if _, err := (Manager{}).SetAppEnabled(root, "alemonjs-load-yunzai", true); err != nil {
		t.Fatalf("SetAppEnabled(true): %v", err)
	}
	apps, _ = (Manager{}).EnabledApps(root)
	if len(apps) != 1 || apps[0] != "alemonjs-load-yunzai" {
		t.Fatalf("apps after enable = %v, want [alemonjs-load-yunzai]", apps)
	}
	// Disable it.
	if _, err := (Manager{}).SetAppEnabled(root, "alemonjs-load-yunzai", false); err != nil {
		t.Fatalf("SetAppEnabled(false): %v", err)
	}
	apps, _ = (Manager{}).EnabledApps(root)
	if len(apps) != 0 {
		t.Fatalf("apps after disable = %v, want empty", apps)
	}
	// Existing serverPort must survive a yaml rewrite.
	if _, err := (Manager{}).SaveAppPort(root, 19191); err != nil {
		t.Fatal(err)
	}
	if _, err := (Manager{}).SetAppEnabled(root, "another", true); err != nil {
		t.Fatal(err)
	}
	info, _ := (Manager{}).AppPort(root)
	if info.Port != 19191 {
		t.Fatalf("serverPort lost after apps write, got %d", info.Port)
	}
	apps, _ = (Manager{}).EnabledApps(root)
	if len(apps) != 1 || apps[0] != "another" {
		t.Fatalf("apps = %v, want [another]", apps)
	}
}

func TestSetAppEnabledPreservesMappedAppsWithoutRewritingOtherConfig(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"bot"}`)
	writeAppPageFixture(t, filepath.Join(root, "alemon.config.yaml"), "# keep this comment\napps:\n  first: true\n  disabled: false\nserverPort: 18110 # app port\n")

	apps, err := (Manager{}).EnabledApps(root)
	if err != nil || len(apps) != 1 || apps[0] != "first" {
		t.Fatalf("mapped apps = %v, %v", apps, err)
	}
	if _, err := (Manager{}).SetAppEnabled(root, "second", true); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(root, "alemon.config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "# keep this comment") || !strings.Contains(string(content), "serverPort: 18110 # app port") {
		t.Fatalf("unrelated config was rewritten:\n%s", content)
	}
	if !strings.Contains(string(content), "disabled: false") || !strings.Contains(string(content), "first: true") || !strings.Contains(string(content), "second: true") {
		t.Fatalf("apps switches were not preserved:\n%s", content)
	}
	apps, err = (Manager{}).EnabledApps(root)
	if err != nil || strings.Join(apps, ",") != "first,second" {
		t.Fatalf("mapped apps = %v, %v", apps, err)
	}
	if _, err := (Manager{}).SetAppEnabled(root, "first", false); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(filepath.Join(root, "alemon.config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "first: false") || !strings.Contains(string(content), "disabled: false") {
		t.Fatalf("disabled apps were removed:\n%s", content)
	}
}

// TestAppPortReachableReportsUnreachableWhenPortClosed verifies the probe
// returns unreachable for a port with no listener (safe in sandboxed CI where
// binding sockets is not permitted).
func TestAppPortReachableReportsUnreachableWhenPortClosed(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"bot"}`)
	// Pick a port that is almost certainly not listening (ephemeral range).
	if _, err := (Manager{}).SaveAppPort(root, 65530); err != nil {
		t.Fatal(err)
	}
	reachable, port, err := (Manager{}).AppPortReachable(root)
	if err != nil {
		t.Fatalf("probe error = %v", err)
	}
	if reachable {
		t.Fatalf("port %d should be unreachable when nothing listens", port)
	}
	if port != 65530 {
		t.Fatalf("probe returned port %d, want 65530", port)
	}
}

// TestIsConsoleNoisePathFiltersDependencyDirectories ensures the terminal
// snapshot hides node_modules and build output so the "目录信息" is about the
// project, not its dependencies.
func TestIsConsoleNoisePathFiltersDependencyDirectories(t *testing.T) {
	noisy := []string{
		"node_modules/alemonjs/index.js",
		"node_modules",
		"dist/index.js",
		"lib/index.js",
		".env",
		"logs/error.log",
		"build/out.js",
	}
	for _, path := range noisy {
		if !isConsoleNoisePath(strings.TrimSpace(path)) {
			t.Errorf("expected %q to be filtered as noise", path)
		}
	}
	// A git status line strips its leading "XY " marker before the check.
	if !isConsoleNoisePath(strings.TrimSpace("?? node_modules/"[3:])) {
		t.Errorf("expected git-status node_modules line to be filtered")
	}
	keep := []string{"src/index.ts", "package.json", "README.md", "app.ts"}
	for _, path := range keep {
		if isConsoleNoisePath(path) {
			t.Errorf("expected %q to be kept", path)
		}
	}
}

// TestStripPM2BannerAndParse covers a PM2 daemon version mismatch, where PM2
// writes a ">>>> In-memory PM2 is out-of-date" banner to stdout ahead of the
// JSON array. The banner must be stripped before parsing.
func TestStripPM2BannerAndParse(t *testing.T) {
	payload := ">>>> In-memory PM2 is out-of-date, do:\n>>>> $ pm2 update\nIn memory PM2 version: 7.0.3\nLocal PM2 version: 5.4.3\n\n[{\"pm_id\":0,\"name\":\"alemonb\",\"status\":\"online\",\"pid\":9896,\"pm2_env\":{\"script\":\"./index.js\",\"namespace\":\"default\",\"pm_uptime\":1751900000000},\"monit\":{\"memory\":123,\"cpu\":0.1}}]"
	stripped := stripPM2Banner(payload)
	items, err := parsePM2Processes(stripped)
	if err != nil {
		t.Fatalf("parse with banner: %v\nstripped=%q", err, stripped)
	}
	if len(items) != 1 || items[0].Name != "alemonb" {
		t.Fatalf("parsed = %#v, want one alemonb process", items)
	}
}

// TestStripPM2BannerIgnoresDaemonSpawnNotices covers a fresh bundled PM2
// daemon, which prints "[PM2] Spawning PM2 daemon ..." notices to stdout ahead
// of the JSON array. Those notices must not be mistaken for the payload.
func TestStripPM2BannerIgnoresDaemonSpawnNotices(t *testing.T) {
	payload := "\n                        -------------\n\n__/\\\\\\\\\n[PM2] Spawning PM2 daemon with pm2_home=/tmp/x\n[PM2] PM2 Successfully daemonized\n[]"
	stripped := stripPM2Banner(payload)
	if stripped != "[]" {
		t.Fatalf("stripped = %q, want []", stripped)
	}
	items, err := parsePM2Processes(stripped)
	if err != nil {
		t.Fatalf("parse empty list: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("parsed = %#v, want empty", items)
	}
}
