package catalog

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestOptionsReadsAllPagesAndDetailUsesID(t *testing.T) {
	platformServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/resources/item" {
			json.NewEncoder(w).Encode(map[string]any{"data": Resource{ID: "item", Markdown: "# Help"}})
			return
		}
		if r.URL.Query().Get("type") != "connector,js-plugin" {
			t.Errorf("type lost: %s", r.URL)
		}
		page := 1
		id := "first"
		if r.URL.Query().Get("page") == "2" {
			page = 2
			id = "second"
		}
		json.NewEncoder(w).Encode(ResourcePage{Data: []Resource{{ID: id}}, Total: 2, Page: page, PageSize: 1})
	})
	items, err := Options("connector,js-plugin")
	if err != nil || len(items) != 2 || items[1].ID != "second" {
		t.Fatalf("options: %#v %v", items, err)
	}
	item, err := Detail("item")
	if err != nil || item.Markdown != "# Help" {
		t.Fatalf("detail: %#v %v", item, err)
	}
}
func TestInstallTargetUsesModeAndRejectsUnsafeTargets(t *testing.T) {
	for _, test := range []struct {
		resource Resource
		want     string
	}{
		{Resource{Type: "connector", InstallMode: "npm", PackageName: "@example/connect"}, "@example/connect"},
		{Resource{Type: "js-plugin", InstallMode: "git", RepositoryURL: "https://gitee.com/example/plugin"}, "git+https://gitee.com/example/plugin.git"},
		{Resource{InstallMode: "npm", PackageName: "--flag"}, ""},
		{Resource{InstallMode: "npm", RepositoryURL: "https://github.com/example/plugin"}, ""},
		{Resource{InstallMode: "unknown", PackageName: "valid"}, ""},
		{Resource{RepositoryURL: "https://github.com/example/repo/tree/main"}, ""},
		{Resource{RepositoryURL: "https://user:secret@github.com/example/repo"}, ""},
		{Resource{RepositoryURL: "https://github.com/example/repo#branch"}, ""},
	} {
		target, err := InstallTarget(test.resource)
		if target != test.want || (err != nil) != (test.want == "") {
			t.Fatalf("target=%q err=%v for %#v", target, err, test.resource)
		}
	}
}
