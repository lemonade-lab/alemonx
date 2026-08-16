package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func embeddedTemplates() fstest.MapFS {
	return fstest.MapFS{
		"bot/package.json":        {Data: []byte(`{"name":"bot"}`)},
		"bot/index.js":            {Data: []byte(`import {start} from 'alemonjs';`)},
		"dev/package.json":        {Data: []byte(`{"name":"dev"}`)},
		"dev/src/index.ts":        {Data: []byte(`console.log('dev')`)},
		"bot/.npmrc":              {Data: []byte(`registry=https://registry.npmjs.org`)},
		"notes.txt":               {Data: []byte(`not a template`)},
		"bot/nested/deep/file.js": {Data: []byte(`deep`)},
	}
}

func TestResolveRootExplicitWins(t *testing.T) {
	explicit := filepath.Join(t.TempDir(), "explicit")
	t.Setenv("ALX_WORKSPACE", filepath.Join(t.TempDir(), "env"))
	t.Setenv("ALEMONJS_SETUP_ROOTS", t.TempDir())
	root, err := ResolveRoot(explicit)
	if err != nil {
		t.Fatal(err)
	}
	if root != explicit {
		t.Fatalf("root = %q, want explicit %q", root, explicit)
	}
}

func TestResolveRootEnvFallsBackToCwdWorkspace(t *testing.T) {
	t.Setenv("ALX_WORKSPACE", "")
	t.Setenv("ALEMONJS_SETUP_ROOTS", "")
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(original)
	current, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := ResolveRoot("")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(current, "workspace")
	if root != want {
		t.Fatalf("root = %q, want %q", root, want)
	}
}

func TestResolveRootUsesWritableSetupRoot(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	readonly := t.TempDir()
	if os.Geteuid() != 0 {
		if err := os.Chmod(readonly, 0o500); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(readonly, 0o700)
	}
	if err := os.Chdir(readonly); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(original)
	writableRoot := t.TempDir()
	t.Setenv("ALX_WORKSPACE", "")
	t.Setenv("ALEMONJS_SETUP_ROOTS", readonly+string(os.PathListSeparator)+writableRoot)
	root, err := ResolveRoot("")
	if err != nil {
		t.Fatal(err)
	}
	if root != writableRoot {
		t.Fatalf("root = %q, want writable setup root %q", root, writableRoot)
	}
}

func TestEnsureMaterializesTemplatesAndCreatesLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	layout, err := Ensure(root, embeddedTemplates())
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{layout.Root, layout.Templates(), layout.Bots()} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("expected directory %s: %v", dir, err)
		}
	}
	for _, file := range []string{
		"bot/package.json",
		"bot/index.js",
		"bot/nested/deep/file.js",
		"dev/package.json",
		"dev/src/index.ts",
		"bot/.npmrc",
	} {
		if _, err := os.Stat(filepath.Join(layout.Templates(), file)); err != nil {
			t.Errorf("template %s not materialized: %v", file, err)
		}
	}
	// Top-level non-directory entries are not templates.
	if _, err := os.Stat(filepath.Join(layout.Templates(), "notes.txt")); !os.IsNotExist(err) {
		t.Error("non-template top-level file should not be copied")
	}
	marker, err := os.ReadFile(filepath.Join(layout.Templates(), templateMarker))
	if err != nil || string(marker) != TemplateVersion {
		t.Fatalf("version marker = %q, %v", marker, err)
	}
}

func TestEnsureNeverOverwritesExistingTemplates(t *testing.T) {
	root := t.TempDir()
	templates := filepath.Join(root, "templates")
	if err := os.MkdirAll(filepath.Join(templates, "bot"), 0o755); err != nil {
		t.Fatal(err)
	}
	custom := `{"name":"customized"}`
	if err := os.WriteFile(filepath.Join(templates, "bot", "package.json"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(root, embeddedTemplates()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(templates, "bot", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != custom {
		t.Fatalf("existing template was overwritten: %q", data)
	}
	// Missing files are still added next to the customized ones.
	if _, err := os.Stat(filepath.Join(templates, "bot", "index.js")); err != nil {
		t.Errorf("missing template file not copied: %v", err)
	}
}

func TestLayoutPaths(t *testing.T) {
	layout := Layout{Root: "/tmp/alx-workspace"}
	if layout.Templates() != filepath.Join("/tmp/alx-workspace", "templates") {
		t.Fatalf("templates = %q", layout.Templates())
	}
	if layout.Bots() != filepath.Join("/tmp/alx-workspace", "bots") {
		t.Fatalf("bots = %q", layout.Bots())
	}
	if layout.Packages() != filepath.Join("/tmp/alx-workspace", "packages") {
		t.Fatalf("packages = %q", layout.Packages())
	}
}

func TestTemplatesOutdatedDetectsVersionDrift(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Ensure(root, embeddedTemplates()); err != nil {
		t.Fatal(err)
	}
	if TemplatesOutdated(root) {
		t.Fatal("fresh materialized templates must not be outdated")
	}
	if err := os.WriteFile(filepath.Join(root, "templates", templateMarker), []byte("0"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !TemplatesOutdated(root) {
		t.Fatal("version drift must be detected")
	}
}
