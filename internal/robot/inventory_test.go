package robot

import (
	"path/filepath"
	"testing"
)

func TestPackageInventoryDistinguishesDeclaredInstalledAndBackpack(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot","dependencies":{"missing":"1","module":"2","library":"3"}}`)
	writeAppPageFixture(t, filepath.Join(root, "alemon.config.yaml"), "apps:\n  - module\n  - custom\n")
	writeAppPageFixture(t, filepath.Join(root, "node_modules/module/package.json"), `{"name":"module","version":"2.1","main":"index.js","alemonjs":{},"repository":{"url":"https://github.com/example/module"}}`)
	writeAppPageFixture(t, filepath.Join(root, "node_modules/library/package.json"), `{"name":"library","version":"3.1","main":"index.js"}`)
	local := filepath.Join(root, "packages/directory-name")
	writeAppPageFixture(t, filepath.Join(local, "package.json"), `{"name":"actual-package","version":"4","exports":"./index.js","alemonjs":{}}`)
	if _, err := gitRun(local, "init"); err != nil {
		t.Fatal(err)
	}
	if _, err := gitRun(local, "remote", "add", "origin", "https://github.com/example/backpack.git"); err != nil {
		t.Fatal(err)
	}
	items, err := (Manager{}).PackageInventory(root)
	if err != nil {
		t.Fatal(err)
	}
	facts := map[string]PackageFact{}
	for _, item := range items {
		facts[item.Name] = item
	}
	if len(facts) != 4 {
		t.Fatalf("facts: %#v", facts)
	}
	if facts["missing"].Installed || !facts["missing"].Declared {
		t.Fatalf("missing: %#v", facts["missing"])
	}
	if !facts["module"].Installed || !facts["module"].Enabled || !facts["module"].Loadable || facts["module"].Version != "2.1" {
		t.Fatalf("module: %#v", facts["module"])
	}
	if facts["library"].Loadable {
		t.Fatal("ordinary library must not become an AlemonJS module")
	}
	if facts["actual-package"].Source != "backpack" || facts["actual-package"].Repository != "https://github.com/example/backpack.git" {
		t.Fatalf("backpack: %#v", facts["actual-package"])
	}
}
