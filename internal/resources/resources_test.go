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
		"nvm/v0.40.7/nvm.sh":        {Data: []byte(`# nvm`)},
		"nvm/v0.40.7/nvm-exec":      {Data: []byte(`#!/bin/sh`)},
		"nvm/v0.40.7/LICENSE.md":    {Data: []byte(`MIT`)},
	}
}

func TestMaterializeNVMUsesEmbeddedBundle(t *testing.T) {
	Init(embeddedBundle(), workspace.Layout{})
	target := filepath.Join(t.TempDir(), "nvm", "v0.40.7")
	created, err := MaterializeNVM(target)
	if err != nil || !created {
		t.Fatalf("MaterializeNVM = %t, %v", created, err)
	}
	for _, name := range []string{"nvm.sh", "nvm-exec", "LICENSE.md", versionMarker} {
		if _, err := os.Stat(filepath.Join(target, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	created, err = MaterializeNVM(target)
	if err != nil || created {
		t.Fatalf("second MaterializeNVM = %t, %v", created, err)
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

func TestVersionMarkerAndOutdated(t *testing.T) {
	layout, err := workspace.Ensure(filepath.Join(t.TempDir(), "ws"), nil)
	if err != nil {
		t.Fatal(err)
	}
	Init(embeddedBundle(), layout)
	if _, _, ok := ToolCommand("yarn"); !ok {
		t.Fatal("yarn should materialize")
	}
	marker := filepath.Join(layout.Root, "packages", "yarn", ".alemonx-version")
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != tools["yarn"].BundleVersion {
		t.Fatalf("marker = %q, want %q", data, tools["yarn"].BundleVersion)
	}
	if outdated, _ := Outdated("yarn"); outdated {
		t.Fatal("fresh materialized copy must not be outdated")
	}
	if err := os.WriteFile(marker, []byte("0"), 0o644); err != nil {
		t.Fatal(err)
	}
	outdated, current := Outdated("yarn")
	if !outdated || current != "0" {
		t.Fatalf("outdated = %t, current = %q", outdated, current)
	}
	names := Names()
	if len(names) != 2 || names[0] != "pm2" || names[1] != "yarn" {
		t.Fatalf("Names = %#v", names)
	}
}

func TestProvisionErrorRecorded(t *testing.T) {
	layout, err := workspace.Ensure(filepath.Join(t.TempDir(), "ws"), nil)
	if err != nil {
		t.Fatal(err)
	}
	Init(embeddedBundle(), layout)
	original := provisionRunner
	defer func() { provisionRunner = original }()
	provisionRunner = func(directory, command string, args ...string) error {
		return errors.New("offline registry")
	}
	if _, _, ok := ToolCommand("pm2"); ok {
		t.Fatal("pm2 provisioning must fail")
	}
	if reason := LastProvisionError("pm2"); !strings.Contains(reason, "offline") {
		t.Fatalf("provision error = %q, want offline reason", reason)
	}
}

func TestInstalledDoesNotTriggerProvisioning(t *testing.T) {
	layout, err := workspace.Ensure(filepath.Join(t.TempDir(), "ws"), nil)
	if err != nil {
		t.Fatal(err)
	}
	Init(embeddedBundle(), layout)
	if Installed("pm2") {
		t.Fatal("pm2 must not be considered installed before provisioning")
	}
	if _, _, ok := ToolCommand("yarn"); !ok {
		t.Fatal("yarn should materialize")
	}
	if !Installed("yarn") {
		t.Fatal("yarn should be installed after materialization")
	}
	if Installed("pm2") {
		t.Fatal("Installed must not trigger on-demand provisioning")
	}
}
