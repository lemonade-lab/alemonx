package project

import (
	"alemonx/internal/catalog"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSelectedResourcesAreRevalidatedAndDeduplicated(t *testing.T) {
	config := Config{ResourceIDs: []string{"one", "one", "two"}}
	calls := 0
	err := resolveResources(&config, func(id string) (catalog.Resource, error) {
		calls++
		return catalog.Resource{ID: id, Type: "connector", InstallMode: "npm", PackageName: "@example/" + id}, nil
	})
	if err != nil || calls != 2 || len(config.resolvedResources) != 2 {
		t.Fatalf("resolve: %#v calls=%d err=%v", config, calls, err)
	}
	for _, resource := range []catalog.Resource{{Type: "skill", InstallMode: "npm", PackageName: "example"}, {Type: "connector", InstallMode: "npm", PackageName: "--bad"}} {
		if err := resolveResources(&config, func(string) (catalog.Resource, error) { return resource, nil }); err == nil {
			t.Fatal("invalid selection accepted")
		}
	}
	if err := resolveResources(&config, func(string) (catalog.Resource, error) { return catalog.Resource{}, errors.New("removed") }); err == nil {
		t.Fatal("unavailable selection accepted")
	}
}
func TestOfficialNpmSelectionIsWrittenToTemplate(t *testing.T) {
	root := t.TempDir()
	if err := copyTemplate(os.DirFS("../../resources/templates"), "bot", root); err != nil {
		t.Fatal(err)
	}
	config := Config{Template: "bot", PackageManager: "yarn", resolvedResources: []catalog.Resource{{Type: "connector", InstallMode: "npm", PackageName: "@example/new"}, {Type: "js-plugin", InstallMode: "git", RepositoryURL: "https://github.com/example/git"}}}
	if err := patchPackage(root, config); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.DevDependencies["@example/new"] != "latest" {
		t.Fatalf("npm target lost: %s", data)
	}
	if _, ok := pkg.DevDependencies["git"]; ok {
		t.Fatal("Git resource must use the Git installer")
	}
}
