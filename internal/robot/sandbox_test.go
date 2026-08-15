package robot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSandboxConfigNeutralizesLoginAndPlatform(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"bot"}`)
	writeAppPageFixture(t, filepath.Join(root, "alemon.config.yaml"), "port: 17117\nserverPort: 18110\nlogin: discord\nplatform: '@alemonjs/discord'\nmaster_id:\n  '123': true\n")

	path, cleanup, err := (Manager{}).SandboxConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if path == "" {
		t.Fatal("sandbox override should be created for login config")
	}
	defer cleanup()

	if !strings.HasPrefix(path, ".alx-testone"+string(filepath.Separator)) {
		t.Fatalf("sandbox config must be project-relative for CFG_PATH, got %q", path)
	}
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "\nlogin:") || strings.Contains(content, "\nplatform:") || strings.Contains(content, "\nserverPort:") {
		t.Fatalf("login/platform/serverPort not neutralized:\n%s", content)
	}
	if !strings.Contains(content, "port: 17117") || !strings.Contains(content, "master_id") {
		t.Fatalf("sandbox copy dropped other settings:\n%s", content)
	}
	// The override is a separate file; the user config is untouched.
	original, err := os.ReadFile(filepath.Join(root, "alemon.config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(original), "login: discord") {
		t.Fatal("user config must not be modified")
	}
}

func TestSandboxConfigNoopWithoutLogin(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"bot"}`)
	writeAppPageFixture(t, filepath.Join(root, "alemon.config.yaml"), "port: 17117\n")
	path, cleanup, err := (Manager{}).SandboxConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	cleanup()
	if path != "" {
		t.Fatalf("no override expected, got %q", path)
	}

	// A missing config is also sandbox by default.
	empty := t.TempDir()
	writeAppPageFixture(t, filepath.Join(empty, "package.json"), `{"name":"bot"}`)
	path, cleanup, err = (Manager{}).SandboxConfig(empty)
	if err != nil {
		t.Fatal(err)
	}
	cleanup()
	if path != "" {
		t.Fatalf("no override expected for missing config, got %q", path)
	}
}
