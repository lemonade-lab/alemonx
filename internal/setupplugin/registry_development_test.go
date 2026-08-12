package setupplugin

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestDevelopmentRunnerIsUsedWhenReleaseRunnerIsMissing(t *testing.T) {
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "runner"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "runner", "main.go"), []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}
	plugin := Plugin{
		Source:  source,
		Runtime: "binary",
		Entry:   map[string]string{"missing-platform": "dist/runner"},
		Development: &RuntimeSpec{
			Runtime: "go",
			Entry:   map[string]string{"go": "runner/main.go"},
		},
	}
	entry, err := plugin.entryPath()
	if err != nil {
		t.Fatal(err)
	}
	if entry.name != "go" || !slices.Equal(entry.args, []string{"run", filepath.Join(plugin.Source, "runner")}) {
		t.Fatalf("unexpected development entry: %#v", entry)
	}
}

func TestDevelopmentGoRunnerUsesDeclaredPackageDirectory(t *testing.T) {
	source := t.TempDir()
	runner := filepath.Join(source, "runner")
	if err := os.MkdirAll(runner, 0755); err != nil {
		t.Fatal(err)
	}
	plugin := Plugin{Source: source, DevelopmentSource: true, Development: &RuntimeSpec{Runtime: "go", Entry: map[string]string{"go": "runner"}}}
	entry, err := plugin.entryPath()
	if err != nil || entry.name != "go" || !slices.Equal(entry.args, []string{"run", runner}) {
		t.Fatalf("development package entry = %#v, err=%v", entry, err)
	}
}

func TestActiveDevelopmentSourceOverridesReleaseRunner(t *testing.T) {
	root := t.TempDir()
	release := filepath.Join(root, "release")
	if err := os.MkdirAll(filepath.Join(release, "web"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(release, "alx.json"), []byte(`{"id":"sample-plugin","name":"Sample","version":"1.0.0","runtime":"binary","entry":{"`+runtimePlatformForTest()+`":"dist/runner"},"development":{"runtime":"go","entry":{"go":"runner/main.go"}},"web":{"root":"web"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(release, "dist"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(release, "dist", "runner"), []byte("release"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(release, "runner"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(release, "runner", "main.go"), []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(root)
	releasePlugin, err := LoadDevelopmentSource(release)
	if err != nil {
		t.Fatal(err)
	}
	registry.ActivateDevelopment(releasePlugin)
	plugin, err := registry.Find("sample-plugin")
	if err != nil {
		t.Fatal(err)
	}
	entry, err := plugin.entryPath()
	if err != nil || entry.name != "go" {
		t.Fatalf("development entry = %#v, err=%v", entry, err)
	}
	registry.DeactivateDevelopment("sample-plugin")
	if _, err := registry.Find("sample-plugin"); err != nil {
		t.Fatalf("release did not return after development stop: %v", err)
	}
}

func TestDevelopmentCommandRunnerUsesStructuredCommand(t *testing.T) {
	plugin := Plugin{Source: t.TempDir(), DevelopmentSource: true, Development: &RuntimeSpec{Runtime: "command", Command: &CommandSpec{Program: "python3", Args: []string{"runner/main.py"}}}}
	entry, err := plugin.entryPath()
	if err != nil || entry.name != "python3" || len(entry.args) != 1 || entry.args[0] != "runner/main.py" {
		t.Fatalf("command entry = %#v, err=%v", entry, err)
	}
}

func TestDevelopmentStaticWebRootOverridesReleaseRoot(t *testing.T) {
	source := t.TempDir()
	for _, directory := range []string{"release-web", "dev-web"} {
		if err := os.MkdirAll(filepath.Join(source, directory), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "alx.json"), []byte(`{"id":"static-source","name":"Static","version":"1.0.0","web":{"root":"release-web"},"development":{"web":{"mode":"static","root":"dev-web"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	plugin, err := LoadDevelopmentSource(source)
	if err != nil {
		t.Fatal(err)
	}
	root, err := plugin.WebRoot()
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.EvalSymlinks(filepath.Join(source, "dev-web"))
	if err != nil {
		t.Fatal(err)
	}
	if root != expected {
		t.Fatalf("development web root = %q", root)
	}
}

func runtimePlatformForTest() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}
