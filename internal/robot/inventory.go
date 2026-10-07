package robot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type PackageFact struct {
	Name       string `json:"name"`
	Version    string `json:"version,omitempty"`
	Declared   bool   `json:"declared"`
	Installed  bool   `json:"installed"`
	Enabled    bool   `json:"enabled"`
	Loadable   bool   `json:"loadable"`
	Source     string `json:"source"`
	Repository string `json:"repository,omitempty"`
}

// PackageInventory is entirely local and remains usable without the market.
func (m Manager) PackageInventory(root string) ([]PackageFact, error) {
	project, err := projectPath(root)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(project, "package.json"))
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Dependencies         map[string]string `json:"dependencies"`
		DevDependencies      map[string]string `json:"devDependencies"`
		OptionalDependencies map[string]string `json:"optionalDependencies"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	enabled, enabledErr := m.EnabledApps(project)
	if enabledErr != nil {
		return nil, enabledErr
	}
	active := map[string]bool{}
	for _, name := range enabled {
		active[name] = true
	}
	facts := map[string]PackageFact{}
	add := func(name, source string, declared bool, data []byte) {
		fact := PackageFact{Name: name, Declared: declared, Enabled: active[name], Source: source}
		var pkg struct {
			Name       string          `json:"name"`
			Version    string          `json:"version"`
			Main       string          `json:"main"`
			Exports    json.RawMessage `json:"exports"`
			Alemonjs   json.RawMessage `json:"alemonjs"`
			Repository json.RawMessage `json:"repository"`
		}
		if json.Unmarshal(data, &pkg) == nil && pkg.Name == name {
			fact.Installed = true
			fact.Version = pkg.Version
			// A local package declaration and executable entry are required for the picker.
			fact.Loadable = len(pkg.Alemonjs) > 0 && string(pkg.Alemonjs) != "null" && (pkg.Main != "" || len(pkg.Exports) > 0)
			_ = json.Unmarshal(pkg.Repository, &fact.Repository)
			if fact.Repository == "" {
				var repository struct {
					URL string `json:"url"`
				}
				_ = json.Unmarshal(pkg.Repository, &repository)
				fact.Repository = repository.URL
			}
		}
		facts[name] = fact
	}
	for _, deps := range []map[string]string{manifest.Dependencies, manifest.DevDependencies, manifest.OptionalDependencies} {
		for name, spec := range deps {
			if !packageNamePattern.MatchString(name) {
				continue
			}
			data, _ := os.ReadFile(filepath.Join(project, "node_modules", filepath.FromSlash(name), "package.json"))
			add(name, "dependency", true, data)
			if strings.HasPrefix(spec, "git+https://") {
				fact := facts[name]
				fact.Repository = strings.Split(spec, "#")[0]
				facts[name] = fact
			}
		}
	}
	locals, err := m.LocalPackages(project)
	if err != nil {
		return nil, err
	}
	for _, local := range locals {
		if !local.Valid {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(local.Path, "package.json"))
		add(local.Name, "backpack", true, data)
		if gitRoot, err := gitRun(local.Path, "rev-parse", "--show-toplevel"); err == nil && sameWorkspacePath(local.Path, gitRoot) {
			if repository, err := gitRun(local.Path, "config", "--get", "remote.origin.url"); err == nil {
				fact := facts[local.Name]
				fact.Repository = repository
				facts[local.Name] = fact
			}
		}
	}
	result := []PackageFact{}
	for _, fact := range facts {
		result = append(result, fact)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
