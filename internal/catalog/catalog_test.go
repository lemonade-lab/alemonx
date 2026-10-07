package catalog

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

func platformServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	previous := platformURL
	platformURL = server.URL
	t.Cleanup(func() { platformURL = previous; server.Close() })
}

func TestPlatformCatalogAndDocument(t *testing.T) {
	calls := map[string]int{}
	platformServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		if r.URL.Query().Get("type") != "js-plugin" && r.URL.Path != "/resources/npm-plugin" {
			t.Errorf("unexpected type: %s", r.URL.String())
		}
		switch r.URL.Path {
		case "/resource-subtypes":
			json.NewEncoder(w).Encode(map[string]any{"data": []ResourceSubtype{{ID: "js-module-data", Type: "js-plugin", Name: "数据"}}})
		case "/resources":
			if r.URL.Query().Get("page") == "1" {
				json.NewEncoder(w).Encode(ResourcePage{Data: []Resource{{ID: "npm-plugin", Type: "js-plugin", Subtype: "js-module-data", Name: "展示名", InstallMode: "npm", PackageName: "@example/data"}}, Total: 2, Page: 1, PageSize: 1})
			} else {
				json.NewEncoder(w).Encode(ResourcePage{Data: []Resource{{ID: "git-plugin", Type: "js-plugin", Name: "Git", InstallMode: "git", RepositoryURL: "https://github.com/example/plugin"}}, Total: 2, Page: 2, PageSize: 1})
			}
		case "/resources/npm-plugin":
			json.NewEncoder(w).Encode(map[string]any{"data": Resource{ID: "npm-plugin", Markdown: "# 平台正文"}})
		default:
			t.Fatalf("unexpected source: %s", r.URL)
		}
	})
	groups, err := Fetch("modules")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || len(groups[0].Items) != 2 || len(groups[1].Items) != 1 {
		t.Fatalf("groups: %#v", groups)
	}
	if groups[0].Items[0].Install != "@example/data" || groups[0].Items[1].Install != "git+https://github.com/example/plugin" {
		t.Fatal("installation must use platform fields")
	}
	document, err := LoadDocument("npm-plugin")
	if err != nil || document.Markdown != "# 平台正文" {
		t.Fatalf("document: %#v, %v", document, err)
	}
	if calls["/resources"] != 2 {
		t.Fatalf("pagination calls: %#v", calls)
	}
}

func TestPlatformErrorsPreserveStatusAndQuery(t *testing.T) {
	platformServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "中文 & 搜索" {
			t.Errorf("query not preserved: %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"code":"invalid_subtype","message":"分类不存在"}}`))
	})
	var result ResourcePage
	err := PlatformGet("resources", url.Values{"q": {"中文 & 搜索"}}, &result)
	failure, ok := err.(*PlatformError)
	if !ok || failure.Status != 400 || failure.Message != "分类不存在" {
		t.Fatalf("error: %#v", err)
	}
}

func TestLegacyRepositoryInputsAreRejected(t *testing.T) {
	if _, err := LoadDocument("https://github.com/example/plugin"); err == nil {
		t.Fatal("legacy repository document must not be fetched")
	}
	if _, err := LoadPackageVersions("git+https://github.com/example/plugin"); err == nil {
		t.Fatal("GitHub version API must not be fetched")
	}
	if _, err := Categories("unknown"); err == nil {
		t.Fatal("unknown kind must fail")
	}
}

func TestLivePlatformContract(t *testing.T) {
	if os.Getenv("ALEMONX_TEST_PLATFORM_LIVE") != "1" {
		t.Skip("set ALEMONX_TEST_PLATFORM_LIVE=1 to verify public API")
	}
	for _, kind := range []string{"environment", "apps"} {
		groups, err := Fetch(kind)
		if err != nil {
			t.Fatal(err)
		}
		if len(groups) == 0 || len(groups[0].Items) == 0 {
			t.Fatalf("empty %s catalog", kind)
		}
		item := groups[0].Items[0]
		document, err := LoadDocument(item.ID)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d resources, first=%s mode=%s documentBytes=%d", kind, len(groups[0].Items), item.Name, item.InstallMode, len(document.Markdown))
	}
}
