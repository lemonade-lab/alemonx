package resources

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"alemonx/internal/workspace"
)

func embeddedBundle() fstest.MapFS {
	return fstest.MapFS{
		"templates/bot/package.json": {Data: []byte(`{}`)},
		"packages/yarn/node_modules/yarn/bin/yarn.js": {
			Data: []byte(`#!/usr/bin/env node`),
		},
		"packages/yarn/node_modules/yarn/package.json": {
			Data: []byte(`{"name":"yarn"}`),
		},
		"packages/pm2/package.json": {Data: []byte(`{}`)},
	}
}

func TestToolCommandMaterializesAndResolves(t *testing.T) {
	layout, err := workspace.Ensure(filepath.Join(t.TempDir(), "ws"), nil)
	if err != nil {
		t.Fatal(err)
	}
	Init(embeddedBundle(), layout)
	command, args, ok := ToolCommand("yarn")
	if !ok {
		t.Fatal("bundled yarn should be available")
	}
	if command != "node" {
		t.Fatalf("command = %q, want node", command)
	}
	wantEntry := filepath.Join(layout.Root, "packages", "yarn", "node_modules", "yarn", "bin", "yarn.js")
	if len(args) != 1 || args[0] != wantEntry {
		t.Fatalf("args = %#v, want entry %q", args, wantEntry)
	}
	data, err := os.ReadFile(wantEntry)
	if err != nil || string(data) != `#!/usr/bin/env node` {
		t.Fatalf("materialized entry missing: %v", err)
	}
}

func TestToolCommandProvisionsOnDemandWithYarn(t *testing.T) {
	layout, err := workspace.Ensure(filepath.Join(t.TempDir(), "ws"), nil)
	if err != nil {
		t.Fatal(err)
	}
	Init(embeddedBundle(), layout)
	original := provisionRunner
	defer func() { provisionRunner = original }()
	var ranIn string
	var ranCommand string
	var ranArgs []string
	provisionRunner = func(directory, command string, args ...string) error {
		ranIn = directory
		ranCommand = command
		ranArgs = append([]string(nil), args...)
		entry := filepath.Join(directory, "node_modules", "pm2", "bin", "pm2")
		if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
			return err
		}
		return os.WriteFile(entry, []byte("#!/usr/bin/env node\n"), 0o755)
	}
	command, args, ok := ToolCommand("pm2")
	if !ok {
		t.Fatal("pm2 should be provisioned on demand")
	}
	if command != "node" {
		t.Fatalf("command = %q, want node", command)
	}
	wantDir := filepath.Join(layout.Root, "packages", "pm2")
	if ranIn != wantDir {
		t.Fatalf("provision dir = %q, want %q", ranIn, wantDir)
	}
	if ranCommand != "node" {
		t.Fatalf("provision command = %q, want node", ranCommand)
	}
	joined := strings.Join(ranArgs, " ")
	if !strings.Contains(joined, "yarn.js") || !strings.Contains(joined, "install") {
		t.Fatalf("provision args = %#v, want embedded yarn install", ranArgs)
	}
	if len(args) != 1 || args[0] != filepath.Join(wantDir, "node_modules", "pm2", "bin", "pm2") {
		t.Fatalf("args = %#v", args)
	}
}

func TestToolCommandSkipsProvisionWhenAlreadyInstalled(t *testing.T) {
	layout, err := workspace.Ensure(filepath.Join(t.TempDir(), "ws"), nil)
	if err != nil {
		t.Fatal(err)
	}
	Init(embeddedBundle(), layout)
	entry := filepath.Join(layout.Root, "packages", "pm2", "node_modules", "pm2", "bin", "pm2")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("#!/usr/bin/env node\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	original := provisionRunner
	defer func() { provisionRunner = original }()
	called := false
	provisionRunner = func(directory, command string, args ...string) error {
		called = true
		return nil
	}
	if _, _, ok := ToolCommand("pm2"); !ok {
		t.Fatal("already provisioned pm2 must resolve")
	}
	if called {
		t.Fatal("provision must be skipped when the entry already exists")
	}
	if _, _, ok := ToolCommand("unknown"); ok {
		t.Fatal("unknown tool must not resolve")
	}
}

func TestToolCommandProvisionFailureFallsBack(t *testing.T) {
	layout, err := workspace.Ensure(filepath.Join(t.TempDir(), "ws"), nil)
	if err != nil {
		t.Fatal(err)
	}
	Init(embeddedBundle(), layout)
	original := provisionRunner
	defer func() { provisionRunner = original }()
	provisionRunner = func(directory, command string, args ...string) error {
		return errors.New("offline")
	}
	if _, _, ok := ToolCommand("pm2"); ok {
		t.Fatal("failed provisioning must not resolve pm2")
	}
}

func TestToolCommandWithoutInitFallsBack(t *testing.T) {
	mu.Lock()
	embedded = nil
	workspaceRoot = ""
	materialized = map[string]string{}
	mu.Unlock()
	if _, _, ok := ToolCommand("yarn"); ok {
		t.Fatal("tool must not resolve before Init")
	}
}
