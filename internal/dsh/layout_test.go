package dsh

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"alemonx/internal/resources"
	"alemonx/internal/workspace"
)

func TestLegacyMigrationPreservesOriginalAndExistingDestination(t *testing.T) {
	old := t.TempDir()
	root := t.TempDir()
	r := NewRegistry(old, nil)
	rt, err := r.Runtime(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.SaveConfig(Config{Provider: "deepseek-official", Model: "test", Root: root, APIKey: "test"}); err != nil {
		t.Fatal(err)
	}
	id, err := rt.CreateSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	next := NewRegistry(t.TempDir(), nil)
	next.legacyDir = old
	migrated, err := next.Runtime(root)
	if err != nil {
		t.Fatal(err)
	}
	if !migrated.HasSession(id) {
		t.Fatal("session lost")
	}
	if _, err := os.Stat(filepath.Join(rt.dir, "sessions.json")); err != nil {
		t.Fatal("source removed", err)
	}
	if err := os.WriteFile(filepath.Join(migrated.dir, "keep"), []byte("user"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := next.MigrateLegacy(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(migrated.dir, "keep")); err != nil {
		t.Fatal("destination overwritten", err)
	}
}

func TestRuntimeHomeLockIsExclusiveAndReusable(t *testing.T) {
	home := t.TempDir()
	one, err := acquireRuntimeLock(home)
	if err != nil {
		t.Fatal(err)
	}
	if two, err := acquireRuntimeLock(home); err == nil {
		unlockRuntime(two)
		t.Fatal("second owner accepted")
	}
	unlockRuntime(one)
	two, err := acquireRuntimeLock(home)
	if err != nil {
		t.Fatal(err)
	}
	unlockRuntime(two)
}

func TestMigrationSkipsGeneratedFallbackButPreservesProfileData(t *testing.T) {
	old, target := t.TempDir(), filepath.Join(t.TempDir(), "migrated")
	for _, rel := range []string{"profiles/node_modules/@agentclientprotocol", "profiles/alemonx/.dsh-module-fallback/node_modules", "profiles/alemonx/node_modules/custom"} {
		if err := os.MkdirAll(filepath.Join(old, rel), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, rel := range []string{"sessions.json", "profiles/alemonx/cordis.yml", "profiles/alemonx/node_modules/custom/index.js"} {
		if err := os.WriteFile(filepath.Join(old, rel), []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	links := map[string]string{
		"profiles/node_modules/@agentclientprotocol/sdk":                filepath.Join(t.TempDir(), "removed-install", "sdk"),
		"profiles/alemonx/.dsh-module-fallback/node_modules/dependency": filepath.Join(t.TempDir(), "removed-install", "dependency"),
		"profiles/alemonx/node_modules/dependency":                      filepath.Join(old, "profiles/alemonx/.dsh-module-fallback/node_modules/dependency"),
	}
	for rel, link := range links {
		if err := os.Symlink(link, filepath.Join(old, rel)); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrateRuntimeHome(old, target); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"sessions.json", "profiles/alemonx/cordis.yml", "profiles/alemonx/node_modules/custom/index.js"} {
		if data, err := os.ReadFile(filepath.Join(target, rel)); err != nil || string(data) != "keep" {
			t.Fatalf("data lost: %s, %v", rel, err)
		}
	}
	for rel := range links {
		if _, err := os.Lstat(filepath.Join(target, rel)); !os.IsNotExist(err) {
			t.Fatalf("stale link copied: %s", rel)
		}
		if _, err := os.Lstat(filepath.Join(old, rel)); err != nil {
			t.Fatalf("original changed: %s", rel)
		}
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(old, "user-link")); err != nil {
		t.Fatal(err)
	}
	if err := migrateRuntimeHome(old, filepath.Join(t.TempDir(), "unsafe")); err == nil {
		t.Fatal("unknown external link accepted")
	}
}

func TestProjectRelocationRetainsStorageAndSessions(t *testing.T) {
	base, oldRoot, newRoot := t.TempDir(), t.TempDir(), t.TempDir()
	r := NewRegistry(base, nil)
	old, err := r.Runtime(oldRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := old.SaveConfig(Config{Provider: "deepseek-official", Model: "test", Root: oldRoot, APIKey: "test"}); err != nil {
		t.Fatal(err)
	}
	id, err := old.CreateSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Relocate(context.Background(), oldRoot, newRoot); err != nil {
		t.Fatal(err)
	}
	restarted := NewRegistry(base, nil)
	next, err := restarted.Runtime(newRoot)
	if err != nil {
		t.Fatal(err)
	}
	if next.dir != old.dir || !next.HasSession(id) {
		t.Fatal("relocation lost runtime identity or sessions")
	}
	cfg, err := next.LoadPersistedConfig()
	if err != nil || cfg.Root != canonicalRoot(newRoot) {
		t.Fatalf("root binding = %#v, %v", cfg, err)
	}
}

func TestBundledWorkspaceDSHInitializesSDK(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "resources"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "dsh", "runtime.zip")); err != nil {
		t.Skip("run make dsh-runtime to prepare bundled SDK")
	}
	t.Setenv("ALX_DSH_BIN", "")
	ws := t.TempDir()
	resources.Init(os.DirFS(root), workspace.Layout{Root: ws})
	defer resources.Init(nil, workspace.Layout{})
	r := New(filepath.Join(ws, "dsh", "runtimes", "smoke"), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := r.Start(ctx, Config{Provider: "deepseek-official", Model: "deepseek-flash", Root: ws, APIKey: "initialization-only"}); err != nil {
		t.Fatal(err)
	}
	defer r.Stop(context.Background())
	if !r.Status().Ready {
		t.Fatal("packaged SDK not ready")
	}
	if err := r.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	movedHome := filepath.Join(ws, "dsh", "runtimes", "migrated-smoke")
	if err := migrateRuntimeHome(r.dir, movedHome); err != nil {
		t.Fatalf("SDK-generated profile failed migration: %v", err)
	}
	moved := New(movedHome, nil)
	// Give the second process its own initialization budget; archive extraction
	// and the first boot (especially under -race) consume the original deadline.
	movedCtx, movedCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer movedCancel()
	if err := moved.Start(movedCtx, Config{Provider: "deepseek-official", Model: "deepseek-flash", Root: ws, APIKey: "initialization-only"}); err != nil {
		t.Fatalf("migrated SDK failed to rebuild dependencies: %v", err)
	}
	defer moved.Stop(context.Background())
	if _, err := os.Stat(filepath.Join(movedHome, "profiles", "node_modules", "@agentclientprotocol", "sdk")); err != nil {
		t.Fatalf("SDK fallback was not rebuilt: %v", err)
	}
}
