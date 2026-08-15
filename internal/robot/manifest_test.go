package robot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPackageManifestReadsAndWritesWorkspaceAndAlemonjsFields(t *testing.T) {
	root := t.TempDir()
	manifest := `{
  "name": "example-bot",
  "version": "1.2.3",
  "description": "example",
  "private": "true",
  "type": "module",
  "packageManager": "yarn@1.22.22",
  "workspaces": {"packages": ["packages/*"], "nohoist": ["legacy"]},
  "alemonjs": {
    "config": [{"name": "token", "type": "string", "required": true}],
    "config-source": {"readme": "https://docs.example.com"},
    "desktop": {"logo": "old.svg", "sidebars": [{"name": "状态", "command": "status"}]},
    "web": {"root": "old-dist", "serverPort": true}
  }
}`
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	manager := Manager{}
	got, err := manager.PackageManifest(root)
	if err != nil {
		t.Fatalf("PackageManifest: %v", err)
	}
	if !got.Private || got.ModuleType != "module" || got.PackageManager != "yarn@1.22.22" {
		t.Fatalf("basic fields = %#v", got)
	}
	if !got.WorkspacesEnabled || len(got.Workspaces) != 1 || got.Workspaces[0] != "packages/*" {
		t.Fatalf("workspaces = %#v", got)
	}
	if got.AlemonjsWebRoot != "old-dist" || !got.AlemonjsWebServerPort || len(got.AlemonjsConfig) == 0 {
		t.Fatalf("alemonjs fields = %#v", got)
	}

	got.Private = false
	got.Workspaces = []string{"packages/*", "plugins/*"}
	got.AlemonjsConfig = json.RawMessage(`[{"name":"appId","type":"number","description":"应用 ID"}]`)
	got.AlemonjsConfigSourceOfficial = "https://alemonjs.example.com"
	got.AlemonjsDesktopLogo = "new.svg"
	got.AlemonjsWebRoot = "dist"
	got.AlemonjsWebServerPort = false
	if _, err := manager.SavePackageManifest(root, got); err != nil {
		t.Fatalf("SavePackageManifest: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["private"] != false {
		t.Fatalf("private = %#v, want false", saved["private"])
	}
	workspaces, ok := saved["workspaces"].(map[string]any)
	if !ok || len(workspaces["packages"].([]any)) != 2 || len(workspaces["nohoist"].([]any)) != 1 {
		t.Fatalf("workspace object was not preserved: %#v", saved["workspaces"])
	}
	alemonjs := saved["alemonjs"].(map[string]any)
	desktop := alemonjs["desktop"].(map[string]any)
	if desktop["logo"] != "new.svg" || len(desktop["sidebars"].([]any)) != 1 {
		t.Fatalf("desktop declaration was not preserved: %#v", desktop)
	}
	web := alemonjs["web"].(map[string]any)
	if web["root"] != "dist" {
		t.Fatalf("web root = %#v", web)
	}
	if _, hasServerPort := web["serverPort"]; hasServerPort {
		t.Fatalf("serverPort should be removed: %#v", web)
	}
	config := alemonjs["config"].([]any)
	if len(config) != 1 || config[0].(map[string]any)["name"] != "appId" {
		t.Fatalf("config = %#v", config)
	}
}

func TestValidateManifestRejectsUnsafeWorkspaceAndInvalidAlemonjsConfig(t *testing.T) {
	input := PackageManifest{
		Name:              "example-bot",
		Version:           "1.0.0",
		WorkspacesEnabled: true,
		Workspaces:        []string{"../outside"},
	}
	if err := validateManifest(input); err == nil {
		t.Fatal("unsafe workspace should be rejected")
	}
	input.Workspaces = []string{"packages/*"}
	input.AlemonjsConfig = json.RawMessage(`{"name":"not-an-array"}`)
	if err := validateManifest(input); err == nil {
		t.Fatal("non-array AlemonJS config should be rejected")
	}
}
