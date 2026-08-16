package pm2config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNameIsStableAndSeparatesSameNamedProjects(t *testing.T) {
	first := Name("/robots/one/bot")
	if first != Name("/robots/one/bot") {
		t.Fatal("PM2 name should be stable for one project")
	}
	if first == Name("/robots/two/bot") {
		t.Fatal("same directory name at different paths must not share a PM2 name")
	}
	if !strings.HasPrefix(first, "alemonx-bot-") {
		t.Fatalf("unexpected PM2 name: %q", first)
	}
}

func TestConfigPinsNameNamespaceAndSelfLocatedCWD(t *testing.T) {
	config := Config("/robots/example")
	for _, want := range []string{
		"name: \"alemonx-example-",
		"namespace: \"alemonx\"",
		"cwd: __dirname",
		"script: './index.js'",
		"autorestart: true",
		"min_uptime: '10s'",
		"max_restarts: 10",
		"restart_delay: 3000",
		"exp_backoff_restart_delay: 1000",
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("config missing %q:\n%s", want, config)
		}
	}
	if strings.Contains(config, "/robots/example") {
		t.Fatalf("config must not embed the absolute project path:\n%s", config)
	}
}

func TestEnsureIDIsIdempotent(t *testing.T) {
	root := t.TempDir()
	if err := EnsureID(root); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(root, idFileName))
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.TrimSpace(string(first))) < 16 {
		t.Fatalf("identity too short: %q", first)
	}
	if err := EnsureID(root); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(root, idFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("EnsureID must not overwrite an existing identity")
	}
}

func TestNameSurvivesDirectoryMoveWithStableID(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	writeProject := func(root, id string) {
		manifest := `{"name":"my-robot"}`
		if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, idFileName), []byte(id), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	id := strings.Repeat("a", 32)
	writeProject(first, id)
	// Simulating a move: same identity + same package name at a new path.
	writeProject(second, id)
	if Name(first) != Name(second) {
		t.Fatalf("moving a project must keep its PM2 name: %q != %q", Name(first), Name(second))
	}
	if !strings.HasPrefix(Name(first), "alemonx-my-robot-") {
		t.Fatalf("unexpected stable name: %q", Name(first))
	}
	// A different identity for the same package name must not collide.
	third := t.TempDir()
	writeProject(third, strings.Repeat("b", 32))
	if Name(first) == Name(third) {
		t.Fatal("same package name with different identities must not share a PM2 name")
	}
}

func TestNameFallsBackToPathDigestWithoutID(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"my-robot"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	name := Name(root)
	if !strings.HasPrefix(name, "alemonx-") || name == Name(t.TempDir()) {
		t.Fatalf("unexpected legacy name: %q", name)
	}
	// Legacy names keep the directory base, not the package name.
	if strings.HasPrefix(name, "alemonx-my-robot-") {
		t.Fatalf("legacy fallback must keep the directory base: %q", name)
	}
}
